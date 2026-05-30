package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

<<<<<<< HEAD
	log.Printf("🚀 Kafka Loader started. Continuous Redis -> Kafka processing")
=======
	ortbTicker := time.NewTicker(time.Duration(cfg.FlushIntervalSec) * time.Second)
	defer ortbTicker.Stop()

	impressionClickInterval := time.Duration(cfg.ImpressionClickFlushIntervalSec) * time.Second
	lastImpressionClickRun := time.Now()

	log.Printf(
		"🚀 Kafka Loader started. ORTB every %d sec, impressions/clicks every %d sec",
		cfg.FlushIntervalSec,
		cfg.ImpressionClickFlushIntervalSec,
	)
>>>>>>> opt

	for {
		select {
		case <-sigChan:
<<<<<<< HEAD
			log.Print("🛑 Shutting down Kafka Loader")
			return
		default:
			err := kafka_loader.ProcessKafkaMessages(
				ctx,
				redisClient.Ortb,
				redisClient.Impressions,
				redisClient.Clicks,
				kafkaWriter.Ortb,
				kafkaWriter.Impressions,
				kafkaWriter.Clicks,
				cfg.BatchSizeOrtb,
				cfg.BatchSizeImpressions,
				cfg.BatchSizeClicks,
				cfg.RedisSetOrtb,
				cfg.RedisSetImpressions,
				cfg.RedisSetClicks,
			)
			if err != nil {
				log.Printf("❌ Batch processing error: %v", err)
=======
			log.Println("🛑 Shutting down Kafka Loader")
			return

		case <-ortbTicker.C:
			runImpressionsClicks := time.Since(lastImpressionClickRun) >= impressionClickInterval

			err := kafka_loader.ProcessKafkaMessages(
				context.Background(),

				redisClients.Ortb,
				redisClients.Impressions,
				redisClients.Clicks,

				kafkaWriter.Ortb,
				kafkaWriter.Impressions,
				kafkaWriter.Clicks,

				cfg.BatchSizeOrtb,
				cfg.BatchSizeImpressions,
				cfg.BatchSizeClicks,

				cfg.RedisSetOrtb,
				cfg.RedisSetImpressions,
				cfg.RedisSetClicks,
				runImpressionsClicks,
			)
			if err != nil {
				log.Printf("❌ Batch processing error: %v", err)
			}

			if runImpressionsClicks {
				lastImpressionClickRun = time.Now()
>>>>>>> opt
			}
		}
	}
}
