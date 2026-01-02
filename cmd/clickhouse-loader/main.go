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

	addr := net.JoinHostPort(cfg.Clickhouse.Host, cfg.Clickhouse.Port)

	conn, err := clickhouse.Open(&clickhouse.Options{
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
		log.Fatalf("❌ ClickHouse Open connection failed: %v", err)
	}
	defer conn.Close()

	if err := conn.Ping(ctx); err != nil {
		log.Fatalf("❌ ClickHouse ping failed: %v", err)
	}
	log.Println("✅ Connected to ClickHouse")

	if err := clickhouse_loader.CreateTable(ctx, conn, cfg.Clickhouse.ClickHouseTable); err != nil {
		log.Fatalf("❌ Failed to create table: %v", err)
	}
	log.Printf("✅ Table %s ready", cfg.Clickhouse.ClickHouseTable)

	kafkaReader, err := kafka_service.InitKafkaReader(cfg.Kafka)
	if err != nil {
		log.Fatalf("Cannot init kafka: %v", err)
	}
	defer kafkaReader.Close()
	log.Println("✅ Kafka reader initialized")

	log.Println("GROUP_ID", cfg.Kafka.KafkaGroupID)

	log.Println("🔄 Waiting for Kafka group coordinator to be ready...")
	time.Sleep(10 * time.Second)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("🚀 ClickHouse Loader started. Reading from topic: %s", cfg.Kafka.KafkaTopic)

	for {
		select {
		case <-sigChan:
			log.Print("🛑 Shutting down ClickHouse Loader")
			return
		default:
			err := clickhouse_loader.ProcessKafkaMessages(
				ctx,
				kafkaReader,
				conn,
				cfg.Clickhouse.ClickHouseTable,
				cfg.Clickhouse.BatchSize,
				cfg.TimeoutSec,
			)
			if err != nil {
				log.Printf("❌ Processing error: %v", err)
				continue
			}
		}
	}
}
