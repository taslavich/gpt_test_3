package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[PERCENTER][FATAL] %v", err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.PercenterConfig](ctx)
	if err != nil {
		return fmt.Errorf("load percenter config: %w", err)
	}
	policy, err := simplePolicyFromConfig(cfg)
	if err != nil {
		return err
	}
	if cfg.RedisDBAdvPercenter != 7 {
		return fmt.Errorf("REDIS_DB_ADV_PERCENTER must be 7, got %d", cfg.RedisDBAdvPercenter)
	}
	if strings.TrimSpace(cfg.RedisADVAddr) == "" {
		return fmt.Errorf("REDIS_ADV_ADDR is required")
	}

	redisClient, err := redisService.NewRedisClient(
		strings.TrimSpace(cfg.RedisADVAddr), cfg.RedisPassword, cfg.RedisDBAdvPercenter, cfg.RedisPoolSize, cfg.RedisMinIdleConns,
	)
	if err != nil {
		return fmt.Errorf("initialize percenter Redis: %w", err)
	}
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("percenter Redis unavailable: %w", err)
	}
	store := percenter.NewSimpleStateStore(redisClient, policy)

	clickhouseConn, err := clickhouse.Open(&clickhouse.Options{
		Addr:     []string{net.JoinHostPort(cfg.Clickhouse.Host, cfg.Clickhouse.Port)},
		Protocol: clickhouse.Native,
		TLS:      &tls.Config{MinVersion: tls.VersionTLS12},
		Auth: clickhouse.Auth{
			Username: cfg.Clickhouse.Username,
			Password: cfg.Clickhouse.Password,
			Database: cfg.Clickhouse.Database,
		},
		MaxOpenConns: 2,
		MaxIdleConns: 2,
	})
	if err != nil {
		return fmt.Errorf("initialize ClickHouse: %w", err)
	}
	defer clickhouseConn.Close()
	if err := clickhouseConn.Ping(ctx); err != nil {
		return fmt.Errorf("ClickHouse unavailable: %w", err)
	}

	log.Printf(
		"[PERCENTER][STARTUP] simple optimizer enabled redis_db=%d interval=%s rebenchmark=%s min_impressions=%d steps_pp=%v max_margin=%.2f",
		cfg.RedisDBAdvPercenter, policy.OptimizeInterval, policy.RebenchmarkInterval, policy.MinImpressions, policy.SearchStepsPP, policy.MaxMargin,
	)

	runTick := func(now time.Time) {
		metrics, loadErr := percenter.LoadSimpleWindowMetrics(
			ctx, clickhouseConn, cfg.Clickhouse.Database, cfg.Clickhouse.TableOrtb, cfg.Clickhouse.TableImpressions, policy.OptimizeInterval,
		)
		if loadErr != nil {
			log.Printf("[PERCENTER][SIMPLE][METRICS_ERROR] %v", loadErr)
			return
		}
		metricIndex := percenter.NewSimpleMetricsIndex(metrics)
		states, stateErr := store.States(ctx)
		if stateErr != nil {
			log.Printf("[PERCENTER][SIMPLE][STATE_LIST_ERROR] %v", stateErr)
			return
		}
		for _, state := range states {
			if state.TypeModel != percenter.TypeModelSimple {
				continue
			}
			metric, ok := metricIndex.ForState(state)
			if !ok {
				continue
			}
			next, changed := percenter.AdvanceSimple(state, metric, policy, now)
			if !changed {
				continue
			}
			saved, saveErr := store.SaveCAS(ctx, next, state.PointVersion)
			if saveErr != nil {
				log.Printf("[PERCENTER][SIMPLE][STATE_SAVE_ERROR] segment_hash=%s error=%v", state.SegmentHash, saveErr)
				continue
			}
			if !saved {
				log.Printf("[PERCENTER][SIMPLE][STATE_RACE_SKIP] segment_hash=%s expected_point_version=%d", state.SegmentHash, state.PointVersion)
				continue
			}
			reason := "state_updated"
			if len(next.DecisionHistory) > 0 {
				reason = next.DecisionHistory[len(next.DecisionHistory)-1].Reason
			}
			log.Printf(
				"[PERCENTER][SIMPLE][STATE_UPDATED] segment_hash=%s campaign_id=%s point_version=%d margin=%.4f ssp_bid=%.9f baseline_winrate=%.6f requests=%d impressions=%d observed_winrate=%.6f profit_per_request=%.9f reason=%s",
				next.SegmentHash, next.CampaignID, next.PointVersion, next.Margin, next.SSPBid, next.BaselineWinRate,
				metric.Requests, metric.Impressions, metric.WinRate(), metric.ProfitPerRequest(), reason,
			)
		}
	}

	// Run once at startup; state cadence still prevents a point from moving more
	// often than every configured optimization interval.
	runTick(time.Now().UTC())
	ticker := time.NewTicker(policy.OptimizeInterval)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-stop:
			return nil
		case now := <-ticker.C:
			runTick(now.UTC())
		}
	}
}

func simplePolicyFromConfig(cfg *config.PercenterConfig) (percenter.SimplePolicy, error) {
	if cfg == nil {
		return percenter.SimplePolicy{}, fmt.Errorf("percenter config is nil")
	}
	steps, err := parseSteps(cfg.SimpleMarginSearchSteps)
	if err != nil {
		return percenter.SimplePolicy{}, err
	}
	policy := percenter.SimplePolicy{
		WinRateRetention:    cfg.SimpleWinRateRetention,
		MinImpressions:      cfg.SimpleMinImpressions,
		OptimizeInterval:    cfg.SimpleOptimizeInterval,
		RebenchmarkInterval: cfg.SimpleRebenchmarkInterval,
		SearchStepsPP:       steps,
		MaxMargin:           cfg.SimpleMaxMargin,
		StateTTL:            cfg.SimpleStateTTL,
	}.Normalize()
	if len(policy.SearchStepsPP) != 3 || policy.SearchStepsPP[0] != 5 || policy.SearchStepsPP[1] != 2 || policy.SearchStepsPP[2] != 1 {
		return percenter.SimplePolicy{}, fmt.Errorf("SIMPLE_MARGIN_SEARCH_STEPS must be exactly 5,2,1")
	}
	if policy.WinRateRetention != 0.50 {
		return percenter.SimplePolicy{}, fmt.Errorf("SIMPLE_WIN_RATE_RETENTION must be 0.5")
	}
	if policy.MinImpressions != 5 {
		return percenter.SimplePolicy{}, fmt.Errorf("SIMPLE_MIN_IMPRESSIONS must be 5")
	}
	if policy.OptimizeInterval != 5*time.Minute {
		return percenter.SimplePolicy{}, fmt.Errorf("SIMPLE_OPTIMIZE_INTERVAL must be 5m")
	}
	if policy.RebenchmarkInterval != 6*time.Hour {
		return percenter.SimplePolicy{}, fmt.Errorf("SIMPLE_REBENCHMARK_INTERVAL must be 6h")
	}
	if policy.MaxMargin != 0.90 {
		return percenter.SimplePolicy{}, fmt.Errorf("SIMPLE_MAX_MARGIN must be 0.9")
	}
	return policy, nil
}

func parseSteps(raw string) ([]float64, error) {
	parts := strings.Split(raw, ",")
	steps := make([]float64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid SIMPLE_MARGIN_SEARCH_STEPS %q", raw)
		}
		steps = append(steps, value)
	}
	return steps, nil
}
