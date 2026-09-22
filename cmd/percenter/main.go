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
	simplePolicy, err := simplePolicyFromConfig(cfg)
	if err != nil {
		return err
	}
	complexPolicy, err := complexPolicyFromConfig(cfg)
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
	simpleStore := percenter.NewSimpleStateStore(redisClient, simplePolicy)
	complexStore := percenter.NewComplexStateStore(redisClient, complexPolicy)

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
		"[PERCENTER][STARTUP] simple+complex enabled redis_db=%d simple_interval=%s complex_interval=%s rebenchmark=%s min_impressions=%d simple_steps=%v complex_ssp_steps=%v complex_margin_steps=%v max_margin=%.2f",
		cfg.RedisDBAdvPercenter, simplePolicy.OptimizeInterval, complexPolicy.OptimizeInterval, complexPolicy.RebenchmarkInterval, complexPolicy.MinImpressions,
		simplePolicy.SearchStepsPP, complexPolicy.SSPSearchStepsPercent, complexPolicy.MarginSearchStepsPP, complexPolicy.MaxMargin,
	)

	runSimpleTick := func(now time.Time) {
		metrics, loadErr := percenter.LoadSimpleWindowMetrics(
			ctx, clickhouseConn, cfg.Clickhouse.Database, cfg.Clickhouse.TableOrtb, cfg.Clickhouse.TableImpressions, simplePolicy.OptimizeInterval,
		)
		if loadErr != nil {
			log.Printf("[PERCENTER][SIMPLE][METRICS_ERROR] %v", loadErr)
			return
		}
		metricIndex := percenter.NewSimpleMetricsIndex(metrics)
		states, stateErr := simpleStore.States(ctx)
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
			next, changed := percenter.AdvanceSimple(state, metric, simplePolicy, now)
			if !changed {
				continue
			}
			saved, saveErr := simpleStore.SaveCAS(ctx, next, state.PointVersion)
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

	runComplexTick := func(now time.Time) {
		metrics, loadErr := percenter.LoadComplexWindowMetrics(
			ctx, clickhouseConn, cfg.Clickhouse.Database, cfg.Clickhouse.TableOrtb, cfg.Clickhouse.TableImpressions, complexPolicy.OptimizeInterval,
		)
		if loadErr != nil {
			log.Printf("[PERCENTER][COMPLEX][METRICS_ERROR] %v", loadErr)
			return
		}
		metricIndex := percenter.NewComplexMetricsIndex(metrics)
		states, stateErr := complexStore.States(ctx)
		if stateErr != nil {
			log.Printf("[PERCENTER][COMPLEX][STATE_LIST_ERROR] %v", stateErr)
			return
		}
		for _, state := range states {
			if state.TypeModel != percenter.TypeModelComplex {
				continue
			}
			metric, ok := metricIndex.ForState(state)
			if !ok {
				continue
			}
			next, changed := percenter.AdvanceComplex(state, metric, complexPolicy, now)
			if !changed {
				continue
			}
			saved, saveErr := complexStore.SaveCAS(ctx, next, state.PointVersion)
			if saveErr != nil {
				log.Printf("[PERCENTER][COMPLEX][STATE_SAVE_ERROR] segment_hash=%s error=%v", state.SegmentHash, saveErr)
				continue
			}
			if !saved {
				log.Printf("[PERCENTER][COMPLEX][STATE_RACE_SKIP] segment_hash=%s expected_point_version=%d", state.SegmentHash, state.PointVersion)
				continue
			}
			reason := "state_updated"
			if len(next.DecisionHistory) > 0 {
				reason = next.DecisionHistory[len(next.DecisionHistory)-1].Reason
			}
			log.Printf(
				"[PERCENTER][COMPLEX][STATE_UPDATED] segment_hash=%s campaign_id=%s phase=%s point_version=%d advertiser_price=%.9f margin=%.4f ssp_bid=%.9f baseline_buyout=%.6f baseline_efficiency=%.6f requests=%d impressions=%d buyout=%.6f efficiency=%.6f profit_per_opportunity=%.9f reason=%s",
				next.SegmentHash, next.CampaignID, next.Phase, next.PointVersion, next.AdvertiserPrice, next.Margin, next.SSPBid, next.BaselineBuyout, next.BaselineEfficiency,
				metric.Requests, metric.Impressions, metric.Buyout(), metric.Efficiency(), metric.ProfitPerRelevantOpportunity(), reason,
			)
		}
	}

	runTick := func(now time.Time) {
		runSimpleTick(now)
		runComplexTick(now)
	}

	// Both policies are fixed to 5m by validation below, so one ticker services
	// both optimizers without changing Simple cadence.
	runTick(time.Now().UTC())
	ticker := time.NewTicker(simplePolicy.OptimizeInterval)
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

func complexPolicyFromConfig(cfg *config.PercenterConfig) (percenter.ComplexPolicy, error) {
	if cfg == nil {
		return percenter.ComplexPolicy{}, fmt.Errorf("percenter config is nil")
	}
	sspSteps, err := parseSteps(cfg.ComplexSSPSearchSteps)
	if err != nil {
		return percenter.ComplexPolicy{}, err
	}
	marginSteps, err := parseSteps(cfg.ComplexMarginSearchSteps)
	if err != nil {
		return percenter.ComplexPolicy{}, err
	}
	policy := percenter.ComplexPolicy{
		BuyoutRetention:       cfg.ComplexBuyoutRetention,
		EfficiencyRetention:   cfg.ComplexEfficiencyRetention,
		MinImpressions:        cfg.ComplexMinImpressions,
		OptimizeInterval:      cfg.ComplexOptimizeInterval,
		RebenchmarkInterval:   cfg.ComplexRebenchmarkInterval,
		SSPSearchStepsPercent: sspSteps,
		MarginSearchStepsPP:   marginSteps,
		MaxMargin:             cfg.ComplexMaxMargin,
		StateTTL:              cfg.ComplexStateTTL,
	}.Normalize()
	if !stepsEqual(policy.SSPSearchStepsPercent, []float64{10, 5, 2, 1}) {
		return percenter.ComplexPolicy{}, fmt.Errorf("COMPLEX_SSP_SEARCH_STEPS must be exactly 10,5,2,1")
	}
	if !stepsEqual(policy.MarginSearchStepsPP, []float64{10, 5, 2, 1}) {
		return percenter.ComplexPolicy{}, fmt.Errorf("COMPLEX_MARGIN_SEARCH_STEPS must be exactly 10,5,2,1")
	}
	if policy.BuyoutRetention != 0.80 {
		return percenter.ComplexPolicy{}, fmt.Errorf("COMPLEX_BUYOUT_RETENTION must be 0.8")
	}
	if policy.EfficiencyRetention != 0.80 {
		return percenter.ComplexPolicy{}, fmt.Errorf("COMPLEX_EFFICIENCY_RETENTION must be 0.8")
	}
	if policy.MinImpressions != 5 {
		return percenter.ComplexPolicy{}, fmt.Errorf("COMPLEX_MIN_IMPRESSIONS must be 5")
	}
	if policy.OptimizeInterval != 5*time.Minute {
		return percenter.ComplexPolicy{}, fmt.Errorf("COMPLEX_OPTIMIZE_INTERVAL must be 5m")
	}
	if policy.RebenchmarkInterval != 6*time.Hour {
		return percenter.ComplexPolicy{}, fmt.Errorf("COMPLEX_REBENCHMARK_INTERVAL must be 6h")
	}
	if policy.MaxMargin != 0.90 {
		return percenter.ComplexPolicy{}, fmt.Errorf("COMPLEX_MAX_MARGIN must be 0.9")
	}
	return policy, nil
}

func stepsEqual(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func parseSteps(raw string) ([]float64, error) {
	parts := strings.Split(raw, ",")
	steps := make([]float64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid percenter search steps %q", raw)
		}
		steps = append(steps, value)
	}
	return steps, nil
}
