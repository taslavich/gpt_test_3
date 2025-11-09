package httpServer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func WorkSspAdapterMiddleware(work *bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == sppAdapterWeb.GetWorkStatusUrl {
				next.ServeHTTP(w, r)
				return
			}

			if r.URL.Path == sppAdapterWeb.PutWorkStatusUrl {
				next.ServeHTTP(w, r)
				return
			}

			if !*work {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func InitHttpRouter(httpRouter *chi.Mux) *chi.Mux {
	httpRouter.Use(middleware.Logger)
	httpRouter.Use(middleware.Recoverer)
	httpRouter.Use(middleware.Timeout(60 * time.Second))
	httpRouter.Mount("/debug", middleware.Profiler())

	httpRouter.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	return httpRouter
}

func RunHttpServer(ctx context.Context, router *chi.Mux, host string, port uint16) {
	httpServerAddr := fmt.Sprintf("%s:%d", host, port)
	httpServer := &http.Server{
		Addr:         httpServerAddr,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,

		// Увеличить лимиты соединений
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	errChan := make(chan error)
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		select {
		case <-stop:
			log.Println("Shutting down gracefully...")
			httpServer.Shutdown(ctx)
		case err := <-errChan:
			log.Fatalf("Server crashed: %v", err)
		}
	}()

	log.Printf("Start listening to http://%s/", httpServerAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errChan <- err
		log.Fatalf("Can't start server: %v", err)
	}
}

func RunHttpsServerOptimized(
	ctx context.Context,
	router *chi.Mux,
	host string,
	port uint16,
	e_certFile, e_keyFile string,
	r_certFile, r_keyFile string,
) {
	ecdsaCert, err := tls.LoadX509KeyPair(e_certFile, e_keyFile)
	if err != nil {
		log.Fatal("ECDSA cert error:", err)
	}

	rsaCert, err := tls.LoadX509KeyPair(r_certFile, r_keyFile)
	if err != nil {
		log.Fatal("RSA cert error:", err)
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS10,
		CurvePreferences: []tls.CurveID{
			tls.X25519, tls.CurveP256, tls.CurveP384,
		},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,

			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,

			tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		},
		Certificates: []tls.Certificate{ecdsaCert, rsaCert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      router,
		TLSConfig:    tlsConfig,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errChan := make(chan error, 1) // буферизованный канал

	// Graceful shutdown горутина
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		select {
		case <-stop:
			log.Println("Shutting down gracefully...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			server.Shutdown(shutdownCtx)
		case err := <-errChan:
			log.Printf("Server error: %v", err)
		case <-ctx.Done():
			server.Shutdown(context.Background())
		}
	}()

	log.Printf("Start listening to https://%s/", server.Addr)

	// ✅ Используем ListenAndServeTLS с пустыми путями
	if err := server.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// ✅ Неблокирующая отправка ошибки
		select {
		case errChan <- err:
		default:
			log.Fatalf("Can't start server: %v", err)
		}
	}

	log.Println("Server stopped")
}
