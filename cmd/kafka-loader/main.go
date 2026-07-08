package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
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
		cfg.RedisDBConversions,
		cfg.RedisUseTLS,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init redis shards: %v", err)
	}
	defer func() {
		if err := redisClients.Close(); err != nil {
			log.Printf("⚠️ failed to close Redis clients: %v", err)
		}
	}()

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

	if len(redisAddrs) == 0 {
		log.Fatal("Cannot init ADV balance ticker: redis addrs list is empty")
	}
	if err := kafka_service.StartAdvBalanceRedisTicker(
		ctx,
		cfg.KafkaConfig,
		redisAddrs[0],
		cfg.RedisPassword,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	); err != nil {
		log.Fatalf("Cannot init ADV balance ticker: %v", err)
	}
	log.Println("✅ ADV balance Kafka to Redis ticker initialized")

	kafkaWriter, err := kafka_service.CreateKafkaWriters(cfg.KafkaConfig)
	if err != nil {
		log.Fatalf("Cannot init kafka: %v", err)
	}

	defer func() {
		if err := kafkaWriter.Clicks.Close(); err != nil {
			log.Printf("⚠️ failed to close Clicks Kafka writer: %v", err)
		}
	}()

	defer func() {
		if err := kafkaWriter.Impressions.Close(); err != nil {
			log.Printf("⚠️ failed to close Impressions Kafka writer: %v", err)
		}
	}()

	defer func() {
		if err := kafkaWriter.Ortb.Close(); err != nil {
			log.Printf("⚠️ failed to close ORTB Kafka writer: %v", err)
		}
	}()

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
	defer func() {
		if err := connProd.Close(); err != nil {
			log.Printf("⚠️ failed to close ClickHouse batch-ratio connection: %v", err)
		}
	}()

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
	stopSspOnce := services.NewResettableOnce()
	loaderControl.AddOnStart(stopSspOnce.Reset)

	batchRatioManager.StartClickHouseTicker(ctx, connProd, cfg.BatchRatioConfig)
	batchRatioManager.StartHTTPServer(ctx, cfg.BatchRatioConfig, loaderControl)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	impressionClickInterval := time.Duration(cfg.ImpressionClickFlushIntervalSec) * time.Second
	if impressionClickInterval <= 0 {
		impressionClickInterval = time.Minute
	}

	impressionClickIntervalSec := int64(impressionClickInterval / time.Second)
	if impressionClickIntervalSec <= 0 {
		impressionClickIntervalSec = 1
	}

	var loaderWG sync.WaitGroup
	botNotifier := utils.NewBotMessage(cfg.BotBaseURL, cfg.BotInternalSecret)
	handleStreamError := func(err error) {
		message := fmt.Sprintf("❌ service=Kafka Loader stream error, stopping batch processing and SSP adapter ORTB streams: %v", err)
		log.Print(message)
		if err := botNotifier.SendTextMessageToBot(ctx, message); err != nil {
			log.Printf("❌ failed to send bot notification: %v", err)
		}
		loaderControl.Stop()
		stopSspOnce.Do(func() {
			services.StopAllSspAdapterOrtbStreams(ctx, cfg.SspAdapterWorkStatusURLs)
		})
	}
	emptyPause := time.Duration(cfg.EmptyLoopPauseMS) * time.Millisecond
	if emptyPause <= 0 {
		emptyPause = 200 * time.Millisecond
	}

	log.Printf(
		"🚀 Kafka Loader initialized. Batch processing is stopped until POST /loader/start. Topics: %s, %s, %s",
		cfg.KafkaConfig.KafkaTopicOrtb,
		cfg.KafkaConfig.KafkaTopicImpressions,
		cfg.KafkaConfig.KafkaTopicClicks,
	)

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
				handleStreamError(err)
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

		for {
			if err := loaderControl.WaitIntervalAfterStart(ctx, impressionClickInterval); err != nil {
				return
			}

			batchSizeImpressions, _ := batchRatioManager.BatchSizesInt64(
				cfg.RedisConfig.BatchSizeOrtb * impressionClickIntervalSec,
			)
			err := kafka_loader.ProcessBatchImpressions(
				ctx,
				redisClients.Impressions,
				kafkaWriter.Impressions,
				batchSizeImpressions,
				cfg.RedisSetImpressions,
			)
			if err != nil {
				handleStreamError(err)
				continue
			}
		}
	}()

	loaderWG.Add(1)
	go func() {
		defer loaderWG.Done()

		for {
			if err := loaderControl.WaitIntervalAfterStart(ctx, impressionClickInterval); err != nil {
				return
			}

			_, batchSizeClicks := batchRatioManager.BatchSizesInt64(
				cfg.RedisConfig.BatchSizeOrtb * impressionClickIntervalSec,
			)
			err := kafka_loader.ProcessBatchClicks(
				ctx,
				redisClients.Clicks,
				kafkaWriter.Clicks,
				batchSizeClicks,
				cfg.RedisSetClicks,
			)
			if err != nil {
				handleStreamError(err)
				continue
			}
		}
	}()

	loaderWG.Add(1)
	go func() {
		defer loaderWG.Done()

		for {
			if err := loaderControl.WaitIntervalAfterStart(ctx, 1*time.Minute); err != nil {
				return
			}

			err := kafka_loader.ProcessBatchConversions(
				ctx,
				redisClients.Conversions,
				kafkaWriter.Conversions,
				cfg.RedisConfig.BatchSizeConversions*60,
				cfg.RedisSetConversions,
			)
			if err != nil {
				handleStreamError(err)
				continue
			}
		}
	}()

	<-sigChan
	log.Print("🛑 Graceful shutdown requested")

	loaderControl.Stop()

	done := make(chan struct{})
	go func() {
		loaderWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Print("✅ Current batches finished")
	case <-time.After(5 * time.Second):
		log.Print("⚠️ Graceful shutdown timeout, forcing cancel")
	}

	cancel()
}
