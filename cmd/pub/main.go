package main

import (
	"context"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
	p "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/pub"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	godotenv.Load("pub.env")
	var cfg p.Config
	if e := cleanenv.ReadEnv(&cfg); e != nil {
		log.Fatal(e)
	}
	if e := cfg.Validate(); e != nil {
		log.Fatal(e)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	redis := cfg.Redis()
	defer redis.Close()
	s, e := p.NewServer(cfg, redis, nil)
	if e != nil {
		log.Fatal(e)
	}
	defer s.CloseORTB()
	telemetry, e := p.NewTelemetry(cfg.TelemetryDir, s.ORTB, cfg.ORTBReadySet, cfg.TelemetryMaxBytes)
	if e != nil {
		log.Fatal(e)
	}
	defer telemetry.Close()
	s.Telemetry = telemetry
	go s.RunInfrastructure(ctx)
	telemetryDone := make(chan struct{})
	go func() { defer close(telemetryDone); telemetry.Run(ctx) }()
	go func() {
		lastSafety := time.Now()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if time.Since(lastSafety) >= 20*time.Second {
					lastSafety = time.Now()
					check, cancel := context.WithTimeout(ctx, 2*time.Second)
					if e := s.RefreshSafety(check); e != nil {
						log.Printf("publisher Redis safety: %v", e)
					}
					cancel()
				}
				flush, cancel := context.WithTimeout(ctx, 2*time.Second)
				if e := s.FlushLast(flush); e != nil {
					log.Printf("publisher last_request batch: %v", e)
				}
				cancel()
			}
		}
	}()
	server := &http.Server{Addr: cfg.Listen, Handler: s.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("pub listening %s", cfg.Listen)
		if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Printf("pub HTTP: %v", e)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdown)
	<-telemetryDone
	s.FlushLast(shutdown)
	if e := telemetry.Flush(shutdown); e != nil {
		log.Printf("pub telemetry remains in durable spool: %v", e)
	}
}
