package httpServer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/activation"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ocsp"
)

/*func runServer(cfg *config.BiddingEngineConfig) {

	lis, err := net.Listen(
		"tcp",
		fmt.Sprintf(
			"%s:%d",
			cfg.HTTPServer.Host,
			cfg.HTTPServer.Port,
		),
	)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterBiddingEngineServiceServer(
		s,
		&biddingEngineWeb.Server{
			ProfitPercent:        cfg.ProfitPercent,
			GetWinnerBidInternal: biddingEngine.GetWinnerBid,
		},
	)

	log.Printf("Server started on %s:%d", cfg.HTTPServer.Host, cfg.HTTPServer.Port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

	// Канал для ошибок
	errChan := make(chan error)

	// Запуск сервера в горутине
	go func() {
		if err := s.Serve(lis); err != nil {
			errChan <- err
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		log.Println("Shutting down gracefully...")
		srv.GracefulStop() // Плавная остановка gRPC
	case err := <-errChan:
		log.Fatalf("Server crashed: %v", err)
	}
}*/

func InitHttpRouter() *chi.Mux {
	httpRouter := chi.NewRouter()
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

// ===== OCSP инфраструктура (обновление stapling вне хендшейка) =====

type servedCert struct {
	mu   sync.RWMutex
	cert tls.Certificate   // актуальная копия (включая OCSPStaple)
	leaf *x509.Certificate // parsed leaf
	iss  *x509.Certificate // parsed issuer (первый intermediate из fullchain)
}

func loadFullchain(pair tls.Certificate) (leaf, issuer *x509.Certificate) {
	leaf, _ = x509.ParseCertificate(pair.Certificate[0])
	if len(pair.Certificate) > 1 {
		issuer, _ = x509.ParseCertificate(pair.Certificate[1])
	}
	return
}

// Периодически обновляет OCSP-staple для pair (если доступен OCSP сервер)
func refreshOCSP(ctx context.Context, sc *servedCert, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	client := &http.Client{Timeout: 5 * time.Second}

	for {
		select {
		case <-ticker.C:
			sc.mu.RLock()
			leaf, iss := sc.leaf, sc.iss
			sc.mu.RUnlock()

			if leaf == nil || iss == nil || len(leaf.OCSPServer) == 0 {
				continue
			}
			reqBytes, err := ocsp.CreateRequest(leaf, iss, &ocsp.RequestOptions{})
			if err != nil {
				continue
			}
			req, err := http.NewRequestWithContext(ctx, "POST", leaf.OCSPServer[0], bytes.NewReader(reqBytes))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/ocsp-request")

			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// Обновляем staple вкопии сертификата
			sc.mu.Lock()
			c := sc.cert
			c.OCSPStaple = body
			sc.cert = c
			sc.mu.Unlock()

		case <-ctx.Done():
			return
		}
	}
}

// ===== Выбор сертификата: RSA-first, ECDSA — fallback =====

func selectCertificateRSAFirst(hello *tls.ClientHelloInfo, ecdsaCert, rsaCert *tls.Certificate) *tls.Certificate {
	// Без SNI — максимально совместимый RSA
	if hello == nil || hello.ServerName == "" {
		return rsaCert
	}
	// Если клиент поддерживает RSA-схемы подписи — отдадим RSA (шире совместимость)
	for _, s := range hello.SignatureSchemes {
		switch s {
		case tls.PKCS1WithSHA256, tls.PKCS1WithSHA384, tls.PKCS1WithSHA512,
			tls.PSSWithSHA256, tls.PSSWithSHA384, tls.PSSWithSHA512,
			tls.PKCS1WithSHA1:
			return rsaCert
		}
	}
	// Иначе пробуем ECDSA
	return ecdsaCert
}

// ===== Session ticket key rotation =====

func generateSessionTicketKey() [32]byte {
	var key [32]byte
	_, err := rand.Read(key[:])
	if err != nil {
		log.Printf("Failed to generate session ticket key: %v", err)
	}
	return key
}

// ===== HTTPS сервер с поправками совместимости TLS =====

func RunHttpsServerOptimized(
	ctx context.Context,
	router *chi.Mux,
	host string,
	port uint16,
	ecdsaCertFile, ecdsaKeyFile string,
	rsaCertFile, rsaKeyFile string,
) {
	go func() {
		redirectAddr := fmt.Sprintf("%s:80", host)
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := *r.URL
			u.Scheme = "https"
			u.Host = r.Host
			http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
		})

		srv := &http.Server{
			Addr:              redirectAddr,
			Handler:           h,
			ReadHeaderTimeout: 30 * time.Second,
		}

		log.Printf("Starting HTTP→HTTPS redirect on http://%s/", redirectAddr)

		// Graceful shutdown для редиректа
		go func() {
			<-ctx.Done() // Ждем когда основной контекст отменится
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv.Shutdown(shutdownCtx)
		}()

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP redirect server error: %v", err)
		}
	}()

	// GC/Memory — по желанию, оставим как было
	debug.SetGCPercent(10)
	debug.SetMemoryLimit(256 * 1024 * 1024)
	runtime.GOMAXPROCS(runtime.NumCPU())

	// systemd socket activation (если есть)
	if listeners, err := activation.Listeners(); err == nil && len(listeners) > 0 {
		log.Println("Using systemd socket activation (will wrap in TLS below)")
	}

	// 1) Грузим ОТДЕЛЬНО два fullchain-серта (leaf+intermediate)
	ecdsaPair, err := tls.LoadX509KeyPair(ecdsaCertFile, ecdsaKeyFile) // должен быть fullchain
	if err != nil {
		log.Fatalf("Failed to load ECDSA certificate: %v", err)
	}
	rsaPair, err := tls.LoadX509KeyPair(rsaCertFile, rsaKeyFile) // должен быть fullchain
	if err != nil {
		log.Fatalf("Failed to load RSA certificate: %v", err)
	}

	// 2) Разбираем leaf/issuer и готовим «живые» копии
	ecdsaServ := &servedCert{cert: ecdsaPair}
	ecdsaServ.leaf, ecdsaServ.iss = loadFullchain(ecdsaPair)

	rsaServ := &servedCert{cert: rsaPair}
	rsaServ.leaf, rsaServ.iss = loadFullchain(rsaPair)

	// 3) Обновление OCSP stapling вне хендшейка (уменьшает timeouts/EOF)
	go refreshOCSP(ctx, ecdsaServ, 12*time.Hour)
	go refreshOCSP(ctx, rsaServ, 12*time.Hour)

	// 4) Ротация session ticket key (для 0-RTT/резюмпшена TLS, снижает латентность)
	sessionTicketKey := generateSessionTicketKey()
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				sessionTicketKey = generateSessionTicketKey()
				log.Println("Rotated TLS session ticket key")
			case <-ctx.Done():
				return
			}
		}
	}()

	// 5) TLS-конфиг: только TLS1.2/1.3, современные шифры, ALPN h2+h1.1
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,

		// Современные кривые с максимальной совместимостью
		CurvePreferences: []tls.CurveID{
			tls.X25519, tls.CurveP256,
		},

		// Набор шифров без CBC/3DES (они не помогают совместимости, но ухудшают безопасность)
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,

			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},

		// Сертификаты будут отдаваться через GetCertificate (ниже), чтобы выбрать RSA/ECDSA
		// Certificates здесь можно не указывать либо указать как fallback
		// Certificates: []tls.Certificate{rsaPair, ecdsaPair},

		NextProtos: []string{"h2", "http/1.1"},

		// Резюмпшен: включены session tickets, ключ — наш
		SessionTicketsDisabled: false,
		SessionTicketKey:       sessionTicketKey,

		// Порядок шифров сервера применим только к TLS1.2 (для 1.3 игнорируется)
		PreferServerCipherSuites: true,

		// Выбор сертификата: RSA-first
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			ecdsaServ.mu.RLock()
			ecdsa := ecdsaServ.cert
			ecdsaServ.mu.RUnlock()

			rsaServ.mu.RLock()
			rsa := rsaServ.cert
			rsaServ.mu.RUnlock()

			return selectCertificateRSAFirst(hello, &ecdsa, &rsa), nil
		},
	}

	httpServerAddr := fmt.Sprintf("%s:%d", host, port)

	// 6) Оптимизированный net.Listen с аккуратными сокет-опциями
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// Буферы (512KB)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 512*1024)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, 512*1024)
				// Reuse addr
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				// Nagle off
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_NODELAY, 1)
				// Keepalive
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPIDLE, 30)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPINTVL, 10)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPCNT, 6)
				// TCP Fast Open (сервер): TCP_FASTOPEN = 23 (0x17) — значение зависит от ядра
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, 0x17, 1)
				// ВАЖНО: SO_ACCEPTCONN/TCP_MAXSEG руками не настраиваем — убрано.
			})
		},
	}

	errorLog := log.New(os.Stderr, "HTTP: ", log.LstdFlags)
	server := &http.Server{
		Addr:    httpServerAddr,
		Handler: router,

		// Важные таймауты (с запасом для медленного TLS/мобилок)
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       45 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       300 * time.Second,

		MaxHeaderBytes: 1 << 20, // 1MB
		TLSConfig:      tlsConfig,
		ErrorLog:       errorLog,
	}

	errChan := make(chan error, 1)
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		select {
		case <-stop:
			log.Println("Shutting down gracefully...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf("Graceful shutdown failed: %v", err)
			}
		case err := <-errChan:
			log.Printf("Server error: %v", err)
		case <-ctx.Done():
			_ = server.Shutdown(context.Background())
		}
	}()

	log.Printf("Starting optimized HTTPS server on https://%s/", httpServerAddr)
	log.Printf("TLS features: RSA-first + ECDSA, TLS1.2/1.3, OCSP stapling (async), HTTP/2, TFO")

	// systemd socket activation
	if listeners, err := activation.Listeners(); err == nil && len(listeners) > 0 {
		log.Println("Using systemd socket activation")
		tlsListener := tls.NewListener(listeners[0], server.TLSConfig)
		if err := server.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
		return
	}

	ln, err := lc.Listen(ctx, "tcp", httpServerAddr)
	if err != nil {
		log.Fatalf("Failed to create optimized listener: %v", err)
	}
	defer ln.Close()

	tlsListener := tls.NewListener(ln, server.TLSConfig)
	if err := server.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		select {
		case errChan <- err:
		default:
			log.Fatalf("Can't start optimized HTTPS server: %v", err)
		}
	}

	log.Println("Server stopped")
}
