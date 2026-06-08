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
	defer connProd.Close()

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
	batchRatioManager.StartClickHouseTicker(ctx, connProd, cfg.BatchRatioConfig)
	batchRatioManager.StartHTTPServer(ctx, cfg.BatchRatioConfig)

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

	loaderCtx, stopLoaders := context.WithCancel(ctx)
	defer stopLoaders()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("🚀 ClickHouse Loader started. Reading from topics: %s, %s, %s", cfg.Kafka.KafkaTopicOrtb, cfg.Kafka.KafkaTopicImpressions, cfg.Kafka.KafkaTopicClicks)

	impressionClickInterval := time.Duration(cfg.ImpressionClickFlushIntervalSec) * time.Second
	if impressionClickInterval <= 0 {
		impressionClickInterval = time.Minute
	}
	loaderErrCh := make(chan error, 3)
	var loaderWG sync.WaitGroup
	emptyPause := time.Duration(cfg.EmptyLoopPauseMS) * time.Millisecond
	if emptyPause <= 0 {
		emptyPause = 200 * time.Millisecond
	}

	loaderWG.Add(1)
	go func() {
		defer loaderWG.Done()
		for {
			select {
			case <-loaderCtx.Done():
				return
			default:
			}

			inserted, err := clickhouse_loader.ProcessKafkaMessagesOrtb(
				loaderCtx,
				kafkaReaders.Ortb,
				connProd,
				cfg.Clickhouse.TableOrtb,
				cfg.Clickhouse.BatchSizeOrtb,
				cfg.TimeoutSec,
				cfg.Clickhouse.BatchTimeoutMS,
			)
			if err != nil {
				loaderErrCh <- err
				stopLoaders()
				return
			}
			if inserted == 0 {
				select {
				case <-loaderCtx.Done():
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
			batchSizeImpressions, _ := batchRatioManager.BatchSizes(cfg.Clickhouse.BatchSizeOrtb)
			err := clickhouse_loader.ProcessKafkaMessagesImpressions(
				loaderCtx,
				kafkaReaders.Impressions,
				connProd,
				cfg.Clickhouse.TableImpressions,
				batchSizeImpressions,
				cfg.TimeoutSec,
				cfg.Clickhouse.BatchTimeoutMS,
			)
			if err != nil {
				loaderErrCh <- err
				stopLoaders()
				return
			}

			select {
			case <-loaderCtx.Done():
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
			_, batchSizeClicks := batchRatioManager.BatchSizes(cfg.Clickhouse.BatchSizeOrtb)
			err := clickhouse_loader.ProcessKafkaMessagesClicks(
				loaderCtx,
				kafkaReaders.Clicks,
				connProd,
				cfg.Clickhouse.TableClicks,
				batchSizeClicks,
				cfg.TimeoutSec,
				cfg.Clickhouse.BatchTimeoutMS,
			)
			if err != nil {
				loaderErrCh <- err
				stopLoaders()
				return
			}

			select {
			case <-loaderCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	select {
	case <-sigChan:
		log.Print("🛑 Shutting down ClickHouse Loader")
		stopLoaders()
	case err := <-loaderErrCh:
		log.Printf("❌ ClickHouse Loader stream error, stopping all streams: %v", err)
		stopLoaders()
	case <-loaderCtx.Done():
	}
	loaderWG.Wait()
}
