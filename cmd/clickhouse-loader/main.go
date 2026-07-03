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
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("⚠️ failed to close Default ClickHouse connection: %v", err)
		}
	}()

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
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		MaxOpenConns: 4,
		MaxIdleConns: 4,
		Settings: clickhouse.Settings{
			"max_insert_threads": 8,
		},
	})
	if err != nil {
		log.Fatalf("❌ Prod ClickHouse Open connection failed: %v", err)
	}

	defer func() {
		if err := connProd.Close(); err != nil {
			log.Printf("⚠️ failed to close ClickHouse batch-ratio connection: %v", err)
		}
	}()

	if err := connProd.Ping(ctx); err != nil {
		log.Fatalf("❌ Prod ClickHouse ping failed: %v", err)
	}
	log.Println("✅ Connected to Prod ClickHouse")

	impressionsPercent := cfg.Clickhouse.BatchSizeImpressionsPercent
	if impressionsPercent == 0 && cfg.Clickhouse.BatchSizeOrtb > 0 {
		impressionsPercent = float64(cfg.Clickhouse.BatchSizeImpressions) / float64(cfg.Clickhouse.BatchSizeOrtb)
	}
	clicksPercent := cfg.Clickhouse.BatchSizeClicksPercent
	if clicksPercent == 0 && cfg.Clickhouse.BatchSizeOrtb > 0 {
		clicksPercent = float64(cfg.Clickhouse.BatchSizeClicks) / float64(cfg.Clickhouse.BatchSizeOrtb)
	}
	batchRatioManager := services.NewBatchRatioManager(impressionsPercent, clicksPercent, cfg.BatchRatioConfig.TickerEnabled)
	loaderControl := services.NewLoaderControl(false)

	batchRatioManager.StartClickHouseTicker(ctx, connProd, cfg.BatchRatioConfig)
	batchRatioManager.StartHTTPServer(ctx, cfg.BatchRatioConfig, loaderControl)

	kafkaReaders, err := kafka_service.InitKafkaReaders(cfg.Kafka)
	if err != nil {
		log.Fatalf("Cannot init kafka: %v", err)
	}
	defer func() {
		if err := kafkaReaders.Clicks.Close(); err != nil {
			log.Printf("⚠️ failed to close Clicks Kafka reader: %v", err)
		}
	}()

	defer func() {
		if err := kafkaReaders.Impressions.Close(); err != nil {
			log.Printf("⚠️ failed to close Impressions Kafka reader: %v", err)
		}
	}()

	defer func() {
		if err := kafkaReaders.Ortb.Close(); err != nil {
			log.Printf("⚠️ failed to close ORTB Kafka reader: %v", err)
		}
	}()
	log.Println("✅ Kafka readers initialized")

	log.Println("GROUP_ID", cfg)

	log.Println("🔄 Waiting for Kafka group coordinator to be ready...")
	time.Sleep(10 * time.Second)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("🚀 ClickHouse Loader initialized. Batch processing is stopped until POST /loader/start. Topics: %s, %s, %s", cfg.Kafka.KafkaTopicOrtb, cfg.Kafka.KafkaTopicImpressions, cfg.Kafka.KafkaTopicClicks)

	impressionClickInterval := time.Duration(cfg.ImpressionClickFlushIntervalSec) * time.Second
	if impressionClickInterval <= 0 {
		impressionClickInterval = time.Minute
	}

	impressionClickIntervalSec := int(impressionClickInterval / time.Second)
	if impressionClickIntervalSec <= 0 {
		impressionClickIntervalSec = 1
	}

	var loaderWG sync.WaitGroup
	botNotifier := utils.NewBotMessage(cfg.BotBaseURL, cfg.BotInternalSecret)
	handleStreamError := func(err error) {
		message := fmt.Sprintf("❌ service=ClickHouse Loader stream error, stopping batch processing: %v", err)
		log.Print(message)
		if err := botNotifier.SendTextMessageToBot(ctx, message); err != nil {
			log.Printf("❌ failed to send bot notification: %v", err)
		}
		loaderControl.Stop()
	}
	emptyPause := time.Duration(cfg.EmptyLoopPauseMS) * time.Millisecond
	if emptyPause <= 0 {
		emptyPause = 200 * time.Millisecond
	}

	loaderWG.Add(1)
	go func() {
		defer loaderWG.Done()
		for {
			if err := loaderControl.Wait(ctx); err != nil {
				return
			}

			inserted, err := clickhouse_loader.ProcessKafkaMessagesOrtb(
				ctx,
				kafkaReaders.Ortb,
				connProd,
				cfg.Clickhouse.TableOrtb,
				cfg.Clickhouse.BatchSizeOrtb,
				cfg.TimeoutSec,
				cfg.Clickhouse.BatchTimeoutMS,
			)
			if err != nil {
				handleStreamError(err)
				continue
			}
			if inserted == 0 {
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

			batchSizeImpressions, _ := batchRatioManager.BatchSizes(
				cfg.Clickhouse.BatchSizeOrtb * impressionClickIntervalSec,
			)
			err := clickhouse_loader.ProcessKafkaMessagesImpressions(
				ctx,
				kafkaReaders.Impressions,
				connProd,
				cfg.Clickhouse.TableImpressions,
				batchSizeImpressions,
				cfg.TimeoutSec,
				cfg.Clickhouse.BatchTimeoutMS,
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

			_, batchSizeClicks := batchRatioManager.BatchSizes(
				cfg.Clickhouse.BatchSizeOrtb * impressionClickIntervalSec,
			)
			err := clickhouse_loader.ProcessKafkaMessagesClicks(
				ctx,
				kafkaReaders.Clicks,
				connProd,
				cfg.Clickhouse.TableClicks,
				batchSizeClicks,
				cfg.TimeoutSec,
				cfg.Clickhouse.BatchTimeoutMS,
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
			if err := loaderControl.WaitIntervalAfterStart(ctx, time.Minute*10); err != nil {
				return
			}

			err := clickhouse_loader.ProcessKafkaMessagesConversions(
				ctx,
				kafkaReaders.Conversions,
				connProd,
				cfg.Clickhouse.TableConversions,
				cfg.Clickhouse.BatchSizeConversions*600,
				cfg.TimeoutSec,
				cfg.Clickhouse.BatchTimeoutMS,
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
