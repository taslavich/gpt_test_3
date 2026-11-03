package main

import (
	"context"
	"fmt"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	p "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/pub"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	godotenv.Load("pub.env")
	botURL := strings.TrimSpace(os.Getenv("BOT_BASE_URL"))
	botSecret := strings.TrimSpace(os.Getenv("BOT_INTERNAL_SECRET"))
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	var send func(context.Context, string) error
	if botURL != "" && botSecret != "" {
		send = utils.NewBotMessageWithTimeout(botURL, botSecret, 2*time.Second).SendTextMessageToBot
	} else {
		log.Print("pub system notifications disabled: set BOT_BASE_URL and BOT_INTERNAL_SECRET")
	}
	reportLifecycle := func(stage string) {
		if send == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := send(ctx, fmt.Sprintf("[pub] host=%s %s; see service logs", host, stage)); err != nil {
			log.Printf("pub system notification failed: %v", err)
		}
	}
	fatal := func(stage string, err error) {
		log.Printf("pub %s: %v", stage, err)
		reportLifecycle(stage)
		os.Exit(1)
	}
	alerts := p.NewSystemAlerts(host, send)
	var cfg p.Config
	if e := cleanenv.ReadEnv(&cfg); e != nil {
		fatal("env_load_failed", e)
	}
	if e := cfg.Validate(); e != nil {
		fatal("config_invalid", e)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	redis := cfg.Redis()
	defer redis.Close()
	s, e := p.NewServer(cfg, redis, nil)
	if e != nil {
		fatal("server_init_failed", e)
	}
	defer s.CloseORTB()
	telemetry, e := p.NewTelemetry(cfg.TelemetryDir, s.ORTB, cfg.ORTBReadySet, cfg.TelemetryMaxBytes)
	if e != nil {
		fatal("telemetry_init_failed", e)
	}
	defer telemetry.Close()
	s.Telemetry = telemetry
	s.Alerts = alerts
	go alerts.Run(ctx)
	go s.RunInfrastructure(ctx)
	telemetryDone := make(chan struct{})
	go func() { defer close(telemetryDone); telemetry.Run(ctx, alerts) }()
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
						alerts.Record(p.AlertRedisSafety)
						log.Printf("publisher Redis safety: %v", e)
					}
					cancel()
				}
				flush, cancel := context.WithTimeout(ctx, 2*time.Second)
				if e := s.FlushLast(flush); e != nil {
					alerts.Record(p.AlertLastRequest)
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
			reportLifecycle("http_listener_failed")
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
