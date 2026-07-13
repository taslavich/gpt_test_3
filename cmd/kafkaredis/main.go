package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	kafkaService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/kafka"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg, err := config.LoadConfig[config.KafkaRedisConfig](ctx)
	if err != nil {
		log.Fatal(err)
	}
	addr := ""
	if len(cfg.RedisShardAddrs) > 0 {
		addr = cfg.RedisShardAddrs[0]
	}
	rdb, err := redisService.NewRedisClient(addr, cfg.RedisPassword, cfg.RedisDBAdvRuntime, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		log.Fatal(err)
	}
	defer rdb.Close()
	writers, err := kafkaService.CreateAdvKafkaWriters(cfg.KafkaConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer writers.Close()
	readers, err := kafkaService.CreateAdvKafkaReaders(cfg.KafkaConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer readers.Close()
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	var work atomic.Bool
	work.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("/work_status/postgres_sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Query().Get("work") {
		case "true":
			work.Store(true)
			w.WriteHeader(http.StatusOK)
		case "false":
			work.Store(false)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	go func() { _ = http.ListenAndServe(fmt.Sprintf("%s:%d", cfg.HTTPHost, cfg.HTTPPort), mux) }()
	go func() {
		t := time.NewTicker(cfg.ExporterInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				err := kafkaService.ScanSpentTotals(ctx, rdb, cfg.RedisScanCount, func(c context.Context, m kafkaService.SpentTotalMessage) error {
					pc, cancel := context.WithTimeout(c, 5*time.Second)
					defer cancel()
					return kafkaService.PublishSpentTotal(pc, writers.SpentTotals, m)
				})
				if err != nil {
					log.Printf("⚠️ spent_totals export failed: %v", err)
				}
			}
		}
	}()
	for ctx.Err() == nil {
		if !work.Load() {
			time.Sleep(time.Second)
			continue
		}
		msg, err := readers.SpentTotals.FetchMessage(ctx)
		if err != nil {
			continue
		}
		var st kafkaService.SpentTotalMessage
		if err := json.Unmarshal(msg.Value, &st); err == nil {
			err = kafkaService.ApplySpentTotal(ctx, db, st)
		}
		if err != nil {
			work.Store(false)
			log.Printf("⚠️ spent_totals import failed topic=%s partition=%d offset=%d: %v", msg.Topic, msg.Partition, msg.Offset, err)
			continue
		}
		_ = readers.SpentTotals.CommitMessages(ctx, msg)
	}
}
