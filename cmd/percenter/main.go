package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.PercenterConfig](ctx)
	if err != nil {
		log.Fatalf("cannot load percenter config: %v", err)
	}
	policy := policyFromConfig(cfg.PercenterAlgorithmConfig)
	percenterRedisAddr := strings.TrimSpace(cfg.RedisADVAddr)
	if percenterRedisAddr == "" {
		log.Fatal("REDIS_ADV_ADDR is required for percenter")
	}
	if cfg.RedisDBAdvPercenter != 7 {
		log.Fatalf("percenter requires REDIS_DB_ADV_PERCENTER=7, got %d", cfg.RedisDBAdvPercenter)
	}

	redisClient, err := redisService.NewRedisClient(
		percenterRedisAddr,
		cfg.RedisPassword,
		cfg.RedisDBAdvPercenter,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("cannot initialize percenter Redis: %v", err)
	}
	defer redisClient.Close()
	store := percenter.NewStateStore(redisClient, policy)

	clickhouseConn, err := clickhouse.Open(&clickhouse.Options{
		Addr:     []string{net.JoinHostPort(cfg.Host, cfg.Port)},
		Protocol: clickhouse.Native,
		TLS:      &tls.Config{MinVersion: tls.VersionTLS12},
		Auth: clickhouse.Auth{
			Username: cfg.Username,
			Password: cfg.Password,
			Database: cfg.Database,
		},
		MaxOpenConns: 2,
		MaxIdleConns: 2,
	})
	if err != nil {
		log.Fatalf("cannot initialize percenter ClickHouse client: %v", err)
	}
	defer clickhouseConn.Close()

	bot := utils.NewBotMessage(cfg.BotBaseURL, cfg.BotInternalSecret)
	run := func() {
		if err := processTick(ctx, clickhouseConn, store, cfg, policy); err != nil {
			log.Printf("[PERCENTER][TICK_ERROR] %v", err)
			if sendErr := bot.SendTextMessageToBot(ctx, fmt.Sprintf("[PERCENTER][CLICKHOUSE_DOWN] %v", err)); sendErr != nil {
				log.Printf("[PERCENTER][TELEGRAM_ERROR] %v", sendErr)
			}
		}
	}

	// Decisions are made only on the configured cadence. A ClickHouse outage is
	// non-fatal: ADV keeps serving the last Redis state and this worker alerts on
	// every tick while ClickHouse remains unavailable.
	ticker := time.NewTicker(policy.MarginOptimizeInterval)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			run()
		}
	}
}

func processTick(ctx context.Context, conn clickhouse.Conn, store *percenter.StateStore, cfg *config.PercenterConfig, policy percenter.Policy) error {
	metrics, err := percenter.LoadWindowMetrics(
		ctx,
		conn,
		cfg.Database,
		cfg.TableOrtb,
		cfg.TableImpressions,
		3*policy.MarginOptimizeInterval,
	)
	if err != nil {
		return fmt.Errorf("load ClickHouse window metrics: %w", err)
	}

	now := time.Now().UTC()
	for _, metric := range metrics {
		state, err := store.Load(ctx, metric.SegmentHash)
		if err != nil {
			// A state can expire between the auction and the worker tick. The next
			// ADV request recreates it from the current campaign snapshot.
			log.Printf("[PERCENTER][STATE_LOAD_SKIP] segment_hash=%s error=%v", metric.SegmentHash, err)
			continue
		}
		if metric.PointVersion != state.PointVersion {
			continue
		}
		updated, changed := percenter.Advance(state, metric, policy, now)
		if !changed {
			continue
		}
		saved, err := store.SaveIfCurrent(ctx, state, updated)
		if err != nil {
			log.Printf("[PERCENTER][STATE_SAVE_ERROR] segment_hash=%s error=%v", metric.SegmentHash, err)
			continue
		}
		if !saved {
			log.Printf("[PERCENTER][STATE_RACE_SKIP] segment_hash=%s", metric.SegmentHash)
			continue
		}
		log.Printf(
			"[PERCENTER][STATE_UPDATED] segment_hash=%s point_version=%d phase=%s advertiser_price=%.6f ssp_bid=%.6f margin=%.4f requests=%d wins=%d profit_per_request=%.9f",
			updated.SegmentHash,
			updated.PointVersion,
			updated.Phase,
			updated.AdvertiserPrice,
			updated.SSPBid,
			updated.Margin,
			metric.Requests,
			metric.Wins,
			metric.ProfitPerRequest(),
		)
	}
	return nil
}

func policyFromConfig(cfg config.PercenterAlgorithmConfig) percenter.Policy {
	return percenter.Policy{
		BuyoutRetention:        cfg.BuyoutRetention,
		EfficiencyRetention:    cfg.EfficiencyRetention,
		DefaultMinMargin:       cfg.DefaultMinMargin,
		PromoMinMargin:         cfg.PromoMinMargin,
		MaxMargin:              cfg.MaxMargin,
		SSPSearchPrecision:     cfg.SSPSearchPrecision,
		MarginSearchStepsPP:    append([]float64(nil), cfg.MarginSearchSteps...),
		SSPReoptimizeInterval:  cfg.SSPReoptimizeInterval,
		MarginOptimizeInterval: cfg.MarginOptimizeInterval,
		SegmentStateTTL:        cfg.PercenterSegmentStateTTL,
		ADVCacheTTL:            cfg.PercenterADVCacheTTL,
	}.Normalize()
}
