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

	conn := clickhouse.OpenDB(&clickhouse.Options{
		Addr:     []string{addr},
		Protocol: clickhouse.Native,
		TLS:      &tls.Config{},
		Auth: clickhouse.Auth{
			Username: cfg.Clickhouse.Username,
			Password: cfg.Clickhouse.Password,
			Database: cfg.Clickhouse.Database,
		},
	})
	defer conn.Close()

	if err := conn.PingContext(ctx); err != nil {
		log.Fatalf("❌ ClickHouse ping failed: %v", err)
	}
	log.Println("✅ Connected to ClickHouse")

	if err := clickhouse_loader.CreateTable(conn, cfg.Clickhouse.ClickHouseTable); err != nil {
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

	var totalProcessed int64
	startTime := time.Now()

	log.Printf("🚀 ClickHouse Loader started. Reading from topic: %s", cfg.Kafka.KafkaTopic)

	for {
		select {
		case <-sigChan:
			log.Printf("🛑 Shutting down ClickHouse Loader. Total processed: %d records", totalProcessed)
			return
		default:
			processed, err := clickhouse_loader.ProcessKafkaMessages(
				ctx,
				cfg.Kafka.KafkaBrokers[0],
				cfg.Kafka.KafkaTopic,
				kafkaReader,
				conn,
				cfg.Clickhouse.ClickHouseTable,
				cfg.Clickhouse.BatchSize,
				cfg.TimeoutSec,
			)
			if err != nil {
				log.Printf("❌ Processing error: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			totalProcessed += int64(processed)
			if processed > 0 {
				log.Printf("💾 Inserted %d records to ClickHouse (total: %d)", processed, totalProcessed)
			}

			if time.Since(startTime) > time.Minute {
				rate := float64(totalProcessed) / time.Since(startTime).Minutes()
				log.Printf("📊 Stats: %d records processed, %.2f records/min", totalProcessed, rate)
				startTime = time.Now()
			}
		}
	}
}
