package main

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	clickhouse_loader "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/clickhouse-loader"
	kafka_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/kafka"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.ClickhouseLoaderConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	log.Println(cfg.Clickhouse.Username, cfg.Clickhouse.Password)

	addr := net.JoinHostPort(cfg.Clickhouse.Host, cfg.Clickhouse.Port)

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:     []string{addr},
		Protocol: clickhouse.Native,
		TLS:      &tls.Config{},
		Auth: clickhouse.Auth{
			Username: cfg.Clickhouse.Username,
			Password: cfg.Clickhouse.Password,
			Database: cfg.Clickhouse.DatabaseDefault,
		},
	})
	if err != nil {
		log.Fatalf("❌ ClickHouse Open connection failed: %v", err)
	}
	defer conn.Close()

	if err := conn.Ping(ctx); err != nil {
		log.Fatalf("❌ ClickHouse ping failed: %v", err)
	}
	log.Println("✅ Connected to Default ClickHouse")

	if err := clickhouse_loader.CreateDB(ctx, conn, cfg.Clickhouse.Database); err != nil {
		log.Fatalf("❌ Failed to create table: %v", err)
	}
	log.Printf("✅ Db %s ready", cfg.Clickhouse.Database)

	connProd, err := clickhouse.Open(&clickhouse.Options{
		Addr:     []string{addr},
		Protocol: clickhouse.Native,
		TLS:      &tls.Config{},
		Auth: clickhouse.Auth{
			Username: cfg.Clickhouse.Username,
			Password: cfg.Clickhouse.Password,
			Database: cfg.Clickhouse.Database,
		},
	})
	if err != nil {
		log.Fatalf("❌ Prod ClickHouse Open connection failed: %v", err)
	}
	defer connProd.Close()

	if err := connProd.Ping(ctx); err != nil {
		log.Fatalf("❌ Prod ClickHouse ping failed: %v", err)
	}
	log.Println("✅ Connected to Prod ClickHouse")

	kafkaReaders, err := kafka_service.InitKafkaReaders(cfg.Kafka)
	if err != nil {
		log.Fatalf("Cannot init kafka: %v", err)
	}
	defer kafkaReaders.Ortb.Close()
	defer kafkaReaders.Impressions.Close()
	defer kafkaReaders.Clicks.Close()
	log.Println("✅ Kafka readers initialized")

	log.Println("GROUP_ID", cfg)

	log.Println("🔄 Waiting for Kafka group coordinator to be ready...")
	time.Sleep(10 * time.Second)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(cfg.TimeoutSec) * time.Second)
	defer ticker.Stop()

	log.Printf("🚀 ClickHouse Loader started. Reading from topics: %s, %s, %s", cfg.Kafka.KafkaTopicOrtb, cfg.Kafka.KafkaTopicImpressions, cfg.Kafka.KafkaTopicClicks)

	for {
		select {
		case <-sigChan:
			log.Print("🛑 Shutting down ClickHouse Loader")
			return
		case <-ticker.C:
			err := clickhouse_loader.ProcessKafkaMessages(
				ctx,
				kafkaReaders,
				connProd,
				cfg.Clickhouse,
				cfg.TimeoutSec,
			)
			if err != nil {
				log.Printf("❌ Processing error: %v", err)
				continue
			}
		}
	}
}
