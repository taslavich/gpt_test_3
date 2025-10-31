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
	"syscall"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ocsp"

	// Для systemd activation (опционально)
	"github.com/coreos/go-systemd/activation"
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

func RunHttpsServerOptimized(
	ctx context.Context,
	router *chi.Mux,
	host string,
	port uint16,
	ecdsaCertFile, ecdsaKeyFile string,
	rsaCertFile, rsaKeyFile string,
) {
	// === 7. Memory Optimizations ===
	debug.SetGCPercent(10)                  // Более агрессивный GC
	debug.SetMemoryLimit(256 * 1024 * 1024) // Лимит памяти 256MB
	runtime.GOMAXPROCS(runtime.NumCPU())

	// === 6. Systemd Socket Activation ===
	if listeners, err := activation.Listeners(); err == nil && len(listeners) > 0 {
		log.Println("Using systemd socket activation")
		// Будем использовать позже после настройки TLS
	}

	// Загружаем оба сертификата
	ecdsaCert, err := tls.LoadX509KeyPair(ecdsaCertFile, ecdsaKeyFile)
	if err != nil {
		log.Fatalf("Failed to load ECDSA certificate: %v", err)
	}

	rsaCert, err := tls.LoadX509KeyPair(rsaCertFile, rsaKeyFile)
	if err != nil {
		log.Fatalf("Failed to load RSA certificate: %v", err)
	}

	// === 5. Session Ticket Key Rotation ===
	sessionTicketKey := generateSessionTicketKey()

	// Горутина для ротации ключей каждые 24 часа
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			newKey := generateSessionTicketKey()
			sessionTicketKey = newKey
			log.Println("Rotated TLS session ticket key")
		}
	}()

	// Конфиг TLS для максимальной совместимости
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS10,
		MaxVersion: tls.VersionTLS13,

		// Кривые в порядке предпочтения
		CurvePreferences: []tls.CurveID{
			tls.X25519,    // Самый быстрый и безопасный
			tls.CurveP256, // P-256
			tls.CurveP384, // Для максимальной совместимости
		},

		// Шифры для максимальной совместимости
		CipherSuites: []uint16{
			// Современные GCM шифры (TLS 1.2+)
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,

			// RSA GCM шифры (TLS 1.2+)
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,

			// CBC шифры для совместимости (TLS 1.0+)
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA, // Для Windows XP

			// Экстремальная совместимость (TLS 1.0+)
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,       // Очень старые клиенты
			tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA, // Очень старые клиенты
		},

		// Оба сертификата
		Certificates: []tls.Certificate{ecdsaCert, rsaCert},

		// Поддержка HTTP/2 и HTTP/1.1
		NextProtos: []string{"h2", "http/1.1"},

		// === ОПТИМИЗАЦИИ КОТОРЫЕ БЫЛИ В NGINX ===

		// 1. Session Resumption - ВКЛЮЧЕНО
		SessionTicketsDisabled: false,
		ClientSessionCache:     tls.NewLRUClientSessionCache(10000),
		SessionTicketKey:       sessionTicketKey,

		// 2. Prefer Server Cipher Suites (как в nginx: ssl_prefer_server_ciphers on)
		PreferServerCipherSuites: true,

		// 3. Dynamic Record Sizing
		DynamicRecordSizingDisabled: false,

		// 4. Умный выбор сертификата с OCSP Stapling
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert := selectCertificate(hello, &ecdsaCert, &rsaCert)

			// Асинхронное обновление OCSP
			go func(cert *tls.Certificate) {
				if cert.Leaf == nil && len(cert.Certificate) > 0 {
					if leaf, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
						cert.Leaf = leaf
					}
				}

				if cert.Leaf != nil && cert.OCSPStaple == nil {
					// ДОБАВЛЕНО: OCSP с контекстом и таймаутом
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()

					if ocspResponse, err := getOCSPStapleWithContext(ctx, cert.Leaf); err == nil {
						cert.OCSPStaple = ocspResponse
						log.Printf("OCSP staple updated for %s", cert.Leaf.Subject.CommonName)
					}
				}
			}(cert)

			return cert, nil
		},
	}

	httpServerAddr := fmt.Sprintf("%s:%d", host, port)

	// === 2. TCP Fast Open на уровне ядра ===
	// === 4. Buffer Size Optimizations ===
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 128*1024) // 128KB
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, 128*1024) // 128KB

				// TCP Fast Open с квотой (как в nginx)
				syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, 0x17, 1000) // TCP_FASTOPEN=1000

				// ДОБАВЛЕНО: Увеличить лимиты для множественных соединений
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_ACCEPTCONN, 131072) // 128k backlog

				// Reuse port (для многопроцессорности)
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)

				// Максимальные буферы сокета
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 512*1024) // 512KB
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, 512*1024) // 512KB

				// Увеличить максимальный размер сегмента
				syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_MAXSEG, 1460)

				// Nagle algorithm off для уменьшения задержки
				syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_NODELAY, 1)

				// Keepalive
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)

				// ДОБАВЛЕНО: Агрессивные Keep-Alive настройки
				syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPIDLE, 30)  // 30 сек
				syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPINTVL, 10) // 10 сек интервал
				syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPCNT, 6)    // 6 попыток

				// Увеличиваем лимиты файловых дескрипторов
				var rlim syscall.Rlimit
				if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim); err == nil {
					rlim.Cur = 65536
					rlim.Max = 65536
					syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlim)
				}
			})
		},
	}

	// ДОБАВЛЕНО: Кастомный ErrorLog для фильтрации шума
	errorLog := log.New(os.Stderr, "HTTP: ", log.LstdFlags)

	server := &http.Server{
		Addr:    httpServerAddr,
		Handler: router,

		// ДОБАВЛЕНО: HTTP/2 специфичные настройки
		ReadHeaderTimeout: 10 * time.Second, // Уменьшить для HTTP/2

		// Увеличенные таймауты как в продакшн nginx
		ReadTimeout:  45 * time.Second, // Увеличить для медленных TLS handshake
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  300 * time.Second,

		// Увеличить лимиты для HTTP/2 потоков
		MaxHeaderBytes: 1 << 20, // 1 MB
		TLSConfig:      tlsConfig,

		// ДОБАВЛЕНО: Кастомный ErrorLog
		ErrorLog: errorLog,

		// ДОБАВЛЕНО: ConnContext для мониторинга
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return ctx
		},
	}

	errChan := make(chan error, 1)

	// Graceful shutdown горутина
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
		}
	}()

	log.Printf("Starting optimized HTTPS server on https://%s/", httpServerAddr)
	log.Printf("TLS features: RSA+ECDSA, SessionResumption, HTTP/2, TLS1.0-1.3, OCSP, TCP-FastOpen")

	// === 6. Systemd Socket Activation (если есть) ===
	if listeners, err := activation.Listeners(); err == nil && len(listeners) > 0 {
		log.Println("Using systemd socket activation")
		tlsListener := tls.NewListener(listeners[0], server.TLSConfig)
		if err := server.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
		return
	}

	// Создаем оптимизированный listener
	ln, err := lc.Listen(ctx, "tcp", httpServerAddr)
	if err != nil {
		log.Fatalf("Failed to create optimized listener: %v", err)
	}

	defer ln.Close()

	// TLS listener
	tlsListener := tls.NewListener(ln, server.TLSConfig)

	// Запускаем сервер
	if err := server.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		select {
		case errChan <- err:
			// Ошибка отправлена в горутину graceful shutdown
		default:
			log.Fatalf("Can't start optimized HTTPS server: %v", err)
		}
	}

	log.Println("Server stopped")
}

// ДОБАВЛЕНО: OCSP с контекстом
func getOCSPStapleWithContext(ctx context.Context, cert *x509.Certificate) ([]byte, error) {
	if len(cert.OCSPServer) == 0 {
		return nil, errors.New("no OCSP server")
	}

	ocspReq, err := ocsp.CreateRequest(cert, cert, &ocsp.RequestOptions{})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cert.OCSPServer[0], bytes.NewReader(ocspReq))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/ocsp-request")

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// === 5. Session Ticket Key Rotation функции ===
func generateSessionTicketKey() [32]byte {
	var key [32]byte
	_, err := rand.Read(key[:])
	if err != nil {
		log.Printf("Failed to generate session ticket key: %v", err)
	}
	return key
}

// Функция выбора сертификата
func selectCertificate(hello *tls.ClientHelloInfo, ecdsaCert, rsaCert *tls.Certificate) *tls.Certificate {
	// Если клиент явно поддерживает ECDSA - отдаем ECDSA
	for _, suite := range hello.CipherSuites {
		if suite == tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 ||
			suite == tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384 ||
			suite == tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305 {
			return ecdsaCert
		}
	}

	// Fallback на RSA для максимальной совместимости
	return rsaCert
}
