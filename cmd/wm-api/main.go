package main

import (
	"context"
	"crypto/tls"
	"log"
	"net"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-chi/chi/v5"

	"wm-api/internal/config"
	httpServer "wm-api/internal/http"
	"wm-api/internal/service"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig(ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	addr := net.JoinHostPort(cfg.ClickhouseConfig.Host, cfg.ClickhouseConfig.Port)
	clickhouseConn, err := clickhouse.Open(&clickhouse.Options{
		Addr:     []string{addr},
		Protocol: clickhouse.Native,
		TLS:      &tls.Config{},
		Auth: clickhouse.Auth{
			Username: cfg.ClickhouseConfig.Username,
			Password: cfg.ClickhouseConfig.Password,
			Database: cfg.ClickhouseConfig.Database,
		},
	})
	if err != nil {
		log.Fatalf("❌ ClickHouse Open connection failed: %v", err)
	}
	defer clickhouseConn.Close()

	if err := clickhouseConn.Ping(ctx); err != nil {
		log.Fatalf("❌ ClickHouse ping failed: %v", err)
	}
	log.Println("✅ Connected to ClickHouse")

	feedResolver := service.NewFeedResolver(cfg)
	reportService := service.NewReportService(
		clickhouseConn,
		feedResolver,
		cfg.FactClicksTable,
		cfg.FactImpressionsTable,
	)
	handler := service.NewHandler(reportService)

	router := chi.NewRouter()
	router = httpServer.InitHttpRouter(router)
	router.Get("/wm_api/", handler.WmAPI)
	log.Println("HTTP routes initialized")

	httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)
}
