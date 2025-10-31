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

/***************  OCSP INFRA (VALIDATED STAPLING)  ***************/

type servedCert struct {
	mu   sync.RWMutex
	cert tls.Certificate   // актуальная копия (может иметь OCSPStaple)
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

// Обновляем OCSP stapling в фоне. Штаплим ТОЛЬКО валидный (Status=Good, не просрочен).
func refreshOCSP(ctx context.Context, sc *servedCert, every time.Duration) {
	tk := time.NewTicker(every)
	defer tk.Stop()
	client := &http.Client{Timeout: 5 * time.Second}

	for {
		select {
		case <-tk.C:
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

			parsed, err := ocsp.ParseResponseForCert(body, leaf, iss)
			if err != nil || parsed == nil || parsed.Status != ocsp.Good || time.Now().After(parsed.NextUpdate) {
				// не штаплим некорректный/просроченный ответ
				continue
			}

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

/***************  TLS HELPERS  ***************/

func generateSessionTicketKey() [32]byte {
	var k [32]byte
	_, _ = rand.Read(k[:])
	return k
}

// RSA по умолчанию, ECDSA — fallback. Даёт максимум совместимости.
func selectCertificateRSAFirst(hello *tls.ClientHelloInfo, ecdsaCert, rsaCert *tls.Certificate) *tls.Certificate {
	if hello == nil || hello.ServerName == "" {
		return rsaCert
	}
	for _, s := range hello.SignatureSchemes {
		switch s {
		case tls.PKCS1WithSHA256, tls.PKCS1WithSHA384, tls.PKCS1WithSHA512,
			tls.PSSWithSHA256, tls.PSSWithSHA384, tls.PSSWithSHA512,
			tls.PKCS1WithSHA1:
			return rsaCert
		}
	}
	return ecdsaCert
}

// Грубая эвристика "нужен ли режим legacy":
// 1) клиент явно объявил TLS1.0/1.1; или
// 2) клиент не объявляет современные сигнатуры (есть только SHA1).
func needLegacyTLS(hello *tls.ClientHelloInfo) bool {
	for _, v := range hello.SupportedVersions {
		if v == tls.VersionTLS10 || v == tls.VersionTLS11 {
			return true
		}
	}
	hasModern := false
	for _, s := range hello.SignatureSchemes {
		if s == tls.PKCS1WithSHA256 || s == tls.PSSWithSHA256 || s == tls.ECDSAWithP256AndSHA256 ||
			s == tls.PKCS1WithSHA384 || s == tls.PSSWithSHA384 || s == tls.ECDSAWithP384AndSHA384 ||
			s == tls.PKCS1WithSHA512 || s == tls.PSSWithSHA512 || s == tls.ECDSAWithP521AndSHA512 {
			hasModern = true
			break
		}
	}
	return !hasModern
}

/***************  HTTPS SERVER (BASE + LEGACY PROFILES)  ***************/

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
			ReadHeaderTimeout: 5 * time.Second,
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

	// Стабильные настройки GC (избегаем лишних пауз во время хендшейков)
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(0)
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Загружаем ОБА fullchain сертификата
	ecdsaPair, err := tls.LoadX509KeyPair(ecdsaCertFile, ecdsaKeyFile)
	if err != nil {
		log.Fatalf("Failed to load ECDSA certificate: %v", err)
	}
	rsaPair, err := tls.LoadX509KeyPair(rsaCertFile, rsaKeyFile)
	if err != nil {
		log.Fatalf("Failed to load RSA certificate: %v", err)
	}

	ecdsaServ := &servedCert{cert: ecdsaPair}
	ecdsaServ.leaf, ecdsaServ.iss = loadFullchain(ecdsaPair)
	rsaServ := &servedCert{cert: rsaPair}
	rsaServ.leaf, rsaServ.iss = loadFullchain(rsaPair)

	// Валидированный OCSP stapling — в фоне
	go refreshOCSP(ctx, ecdsaServ, 12*time.Hour)
	go refreshOCSP(ctx, rsaServ, 12*time.Hour)

	// Ticket key rotation (для резюмпшена)
	ticketKey := generateSessionTicketKey()
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				ticketKey = generateSessionTicketKey()
				log.Println("Rotated TLS session ticket key")
			case <-ctx.Done():
				return
			}
		}
	}()

	// Базовый профиль: TLS1.2/1.3, современные шифры, h2+h1.1
	baseGetCert := func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		ecdsaServ.mu.RLock()
		ecdsa := ecdsaServ.cert
		ecdsaServ.mu.RUnlock()
		rsaServ.mu.RLock()
		rsa := rsaServ.cert
		rsaServ.mu.RUnlock()
		return selectCertificateRSAFirst(hello, &ecdsa, &rsa), nil
	}

	baseCfg := &tls.Config{
		MinVersion:       tls.VersionTLS12,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
		CipherSuites: []uint16{
			// RSA
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			// ECDSA
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
		NextProtos:               []string{"h2", "http/1.1"},
		SessionTicketsDisabled:   false,
		SessionTicketKey:         ticketKey,
		PreferServerCipherSuites: true,                       // влияет только на TLS1.2
		Certificates:             []tls.Certificate{rsaPair}, // fallback, если GetCertificate не сработает
		GetCertificate:           baseGetCert,
	}

	// Legacy-профиль: включаем TLS1.0/1.1 + CBC/3DES, только http/1.1
	legacyCfg := baseCfg.Clone()
	legacyCfg.MinVersion = tls.VersionTLS10
	legacyCfg.CipherSuites = append(legacyCfg.CipherSuites,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA, // последний шанс для очень старых клиентов
	)
	legacyCfg.NextProtos = []string{"http/1.1"} // HTTP/2 требует TLS1.2+

	// Автовыбор профиля: если клиент "пахнет" старым — отдаём legacy, иначе базовый
	baseCfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		if needLegacyTLS(hello) {
			return legacyCfg, nil
		}
		return nil, nil // остаёмся на baseCfg
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	// Минимально-инвазивные сокет-настройки (без TFO и экзотики)
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPIDLE, 30)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPINTVL, 10)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_KEEPCNT, 6)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, syscall.TCP_NODELAY, 1)
				// TFO отключён: часто даёт редкие EOF из-за middlebox’ов
			})
		},
	}

	errorLog := log.New(os.Stderr, "HTTP: ", log.LstdFlags)
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       45 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       300 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLSConfig:         baseCfg, // legacy будет выбран через GetConfigForClient
		ErrorLog:          errorLog,
	}

	errCh := make(chan error, 1)
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		select {
		case <-stop:
			log.Println("Shutting down gracefully...")
			ctxSh, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctxSh)
		case err := <-errCh:
			log.Printf("Server error: %v", err)
		case <-ctx.Done():
			_ = srv.Shutdown(context.Background())
		}
	}()

	log.Printf("HTTPS listening on https://%s/ | base TLS1.2/1.3 + legacy TLS1.0/1.1 (auto), RSA-first, OCSP(valid)", addr)

	// systemd socket activation (если юзается)
	if listeners, err := activation.Listeners(); err == nil && len(listeners) > 0 {
		log.Println("Using systemd socket activation")
		tlsL := tls.NewListener(listeners[0], srv.TLSConfig)
		if err := srv.Serve(tlsL); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		return
	}

	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer ln.Close()

	tlsLn := tls.NewListener(ln, srv.TLSConfig)
	if err := srv.Serve(tlsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
		select {
		case errCh <- err:
		default:
			log.Fatalf("Can't start HTTPS server: %v", err)
		}
	}
	log.Println("Server stopped")
}
