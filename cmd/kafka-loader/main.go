package main

import (
	"context"
	"fmt"
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

	redisClient := redis_service.NewRedisClients(
		fmt.Sprintf(
			"%s:%s",
			cfg.RedisHost,
			cfg.RedisPort,
		),
		cfg.RedisPassword,
		cfg.RedisDBOrtb,
		cfg.RedisDBImpressions,
		cfg.RedisDBClicks,
	)
	defer redisClient.Ortb.Close()
	defer redisClient.Impressions.Close()
	defer redisClient.Clicks.Close()

	log.Printf("Redis config: Host=%s, Port=%s, Password=%s, DB_ORTB=%d, DB_IMPRESSIONS=%d, DB_CLICKS=%d",
		cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword,
		cfg.RedisDBOrtb, cfg.RedisDBImpressions, cfg.RedisDBClicks)

	if err := redisClient.Ortb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Ortb Redis: %v", err)
	}
	log.Println("✅ Connected to Ortb Redis")

	if err := redisClient.Impressions.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Impressions Redis: %v", err)
	}
	log.Println("✅ Connected to Impressions Redis")

	if err := redisClient.Clicks.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Clicks Redis: %v", err)
	}
	log.Println("✅ Connected to Clicks Redis")

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

	log.Printf("🚀 Kafka Loader started. Continuous Redis -> Kafka processing")

	for {
		select {
		case <-sigChan:
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
			}
		}
	}
}
