package main

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	kafka_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/kafka"
	kafka_loader "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/kafka-loader"
	redis_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.KafkaLoaderConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	redisAddrs := cfg.RedisShardAddrs
	if cfg.RedisUseTLS {
		redisAddrs = cfg.RedisShardTLSAddrs
	}

	redisClients, err := redis_service.NewRedisShardedClients(
		redisAddrs,
		cfg.RedisPassword,
		cfg.RedisDBOrtb,
		cfg.RedisDBImpressions,
		cfg.RedisDBClicks,
		cfg.RedisUseTLS,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init redis shards: %v", err)
	}
	defer redisClients.Close()

	if err := redis_service.PingClients(ctx, "ORTB", redisClients.Ortb); err != nil {
		log.Fatalf("Failed to connect to ORTB redis shards: %v", err)
	}
	log.Println("✅ Connected to ORTB Redis shards")

	if err := redis_service.PingClients(ctx, "Impressions", redisClients.Impressions); err != nil {
		log.Fatalf("Failed to connect to Impressions redis shards: %v", err)
	}
	log.Println("✅ Connected to Impressions Redis shards")

	if err := redis_service.PingClients(ctx, "Clicks", redisClients.Clicks); err != nil {
		log.Fatalf("Failed to connect to Clicks redis shards: %v", err)
	}
	log.Println("✅ Connected to Clicks Redis shards")

	kafkaWriter, err := kafka_service.CreateKafkaWriters(cfg.KafkaConfig)
	if err != nil {
		log.Fatalf("Cannot init kafka: %v", err)
	}
	defer kafkaWriter.Clicks.Close()
	defer kafkaWriter.Impressions.Close()
	defer kafkaWriter.Ortb.Close()

	log.Println("✅ Kafka writer initialized")

	addr := net.JoinHostPort(cfg.ClickhouseConfig.Host, cfg.ClickhouseConfig.Port)
	connProd, err := clickhouse.Open(&clickhouse.Options{
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
	defer connProd.Close()

	if err := connProd.Ping(ctx); err != nil {
		log.Fatalf("❌ ClickHouse ping failed: %v", err)
	}
	log.Println("✅ Connected to ClickHouse for batch ratio")

	impressionsPercent := cfg.RedisConfig.BatchSizeImpressionsPercent
	if impressionsPercent == 0 && cfg.RedisConfig.BatchSizeOrtb > 0 {
		impressionsPercent = float64(cfg.RedisConfig.BatchSizeImpressions) / float64(cfg.RedisConfig.BatchSizeOrtb)
	}
	clicksPercent := cfg.RedisConfig.BatchSizeClicksPercent
	if clicksPercent == 0 && cfg.RedisConfig.BatchSizeOrtb > 0 {
		clicksPercent = float64(cfg.RedisConfig.BatchSizeClicks) / float64(cfg.RedisConfig.BatchSizeOrtb)
	}
	batchRatioManager := services.NewBatchRatioManager(impressionsPercent, clicksPercent, cfg.BatchRatioConfig.TickerEnabled)
	loaderControl := services.NewLoaderControl(false)

	batchRatioManager.StartClickHouseTicker(ctx, connProd, cfg.BatchRatioConfig)
	batchRatioManager.StartHTTPServer(ctx, cfg.BatchRatioConfig, loaderControl)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	impressionClickInterval := time.Duration(cfg.ImpressionClickFlushIntervalSec) * time.Second
	if impressionClickInterval <= 0 {
		impressionClickInterval = time.Minute
	}

	var loaderWG sync.WaitGroup
	emptyPause := time.Duration(cfg.EmptyLoopPauseMS) * time.Millisecond
	if emptyPause <= 0 {
		emptyPause = 200 * time.Millisecond
	}

	log.Printf("🚀 Kafka Loader initialized. Batch processing is stopped until POST /loader/start")

	loaderWG.Add(1)
	go func() {
		defer loaderWG.Done()
		for {
			if err := loaderControl.Wait(ctx); err != nil {
				return
			}

			processed, err := kafka_loader.ProcessBatchOrtb(
				ctx,
				redisClients.Ortb,
				kafkaWriter.Ortb,
				cfg.RedisConfig.BatchSizeOrtb,
				cfg.RedisSetOrtb,
			)
			if err != nil {
				log.Printf("❌ Kafka Loader stream error, stopping batch processing: %v", err)
				loaderControl.Stop()
				continue
			}
			if processed == 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(emptyPause):
				}
			}
		}
	}()

	loaderWG.Add(1)
	go func() {
		defer loaderWG.Done()
		ticker := time.NewTicker(impressionClickInterval)
		defer ticker.Stop()

		for {
			if err := loaderControl.Wait(ctx); err != nil {
				return
			}

			batchSizeImpressions, _ := batchRatioManager.BatchSizesInt64(cfg.RedisConfig.BatchSizeOrtb)
			err := kafka_loader.ProcessBatchImpressions(
				ctx,
				redisClients.Impressions,
				kafkaWriter.Impressions,
				batchSizeImpressions,
				cfg.RedisSetImpressions,
			)
			if err != nil {
				log.Printf("❌ Kafka Loader stream error, stopping batch processing: %v", err)
				loaderControl.Stop()
				continue
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	loaderWG.Add(1)
	go func() {
		defer loaderWG.Done()
		ticker := time.NewTicker(impressionClickInterval)
		defer ticker.Stop()

		for {
			if err := loaderControl.Wait(ctx); err != nil {
				return
			}

			_, batchSizeClicks := batchRatioManager.BatchSizesInt64(cfg.RedisConfig.BatchSizeOrtb)
			err := kafka_loader.ProcessBatchClicks(
				ctx,
				redisClients.Clicks,
				kafkaWriter.Clicks,
				batchSizeClicks,
				cfg.RedisSetClicks,
			)
			if err != nil {
				log.Printf("❌ Kafka Loader stream error, stopping batch processing: %v", err)
				loaderControl.Stop()
				continue
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	<-sigChan
	log.Print("🛑 Shutting down Kafka Loader")
	cancel()
	loaderWG.Wait()
}
