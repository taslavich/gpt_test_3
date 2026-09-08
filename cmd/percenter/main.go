package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-chi/chi/v5"
	goredis "github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
	percenterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter/web"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

const startupCheckTimeout = 10 * time.Second

var requiredPercenterEnv = []string{
	"CLICKHOUSE_USERNAME",
	"CLICKHOUSE_PASSWORD",
	"CLICKHOUSE_DB",
	"CLICKHOUSE_HOST",
	"CLICKHOUSE_PORT",
	"CLICKHOUSE_TABLE_ORTB",
	"CLICKHOUSE_TABLE_IMPRESSIONS",
	"CLICKHOUSE_TABLE_CLICKS",
	"REDIS_ADV_ADDR",
	"REDIS_PASSWORD",
	"REDIS_DB_ADV_PERCENTER",
	"REDIS_PERCENTER_HISTORY_READY_KEY",
	"REDIS_POOL_SIZE",
	"REDIS_MIN_IDLE_CONNS",
	"SSP_REOPTIMIZE_INTERVAL",
	"SIMPLE_BASELINE_REOPTIMIZE_INTERVAL",
	"MARGIN_OPTIMIZE_INTERVAL",
	"BUYOUT_RETENTION",
	"EFFICIENCY_RETENTION",
	"SIMPLE_WIN_RATE_RETENTION",
	"DEFAULT_MIN_MARGIN",
	"PROMO_MIN_MARGIN",
	"MAX_MARGIN",
	"SSP_SEARCH_PRECISION",
	"MARGIN_SEARCH_STEPS",
	"PERCENTER_SEGMENT_STATE_TTL",
	"PERCENTER_ADV_CACHE_TTL",
	"BOT_BASE_URL",
	"BOT_INTERNAL_SECRET",
	"HTTP_HOSTNAME",
	"HTTP_PORT",
}

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
		return fmt.Errorf("cannot load percenter config: %w", err)
	}
	if err := validateRequiredEnvironment(); err != nil {
		return err
	}
	if err := validatePercenterConfig(cfg); err != nil {
		return fmt.Errorf("invalid percenter config: %w", err)
	}

	policy := policyFromConfig(cfg.PercenterAlgorithmConfig)

	redisClient, err := redisService.NewRedisClient(
		strings.TrimSpace(cfg.RedisADVAddr),
		cfg.RedisPassword,
		cfg.RedisDBAdvPercenter,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		return fmt.Errorf("cannot initialize percenter Redis: %w", err)
	}
	defer redisClient.Close()

	startupCtx, startupCancel := context.WithTimeout(ctx, startupCheckTimeout)
	if err := verifyRedis(startupCtx, redisClient); err != nil {
		startupCancel()
		return fmt.Errorf("Redis startup check failed: %w", err)
	}
	startupCancel()
	log.Printf("[PERCENTER][STARTUP] Redis OK addr=%s db=%d", cfg.RedisADVAddr, cfg.RedisDBAdvPercenter)

	store := percenter.NewStateStore(redisClient, policy)
	store.ConfigureHistoryQueue(cfg.RedisPercenterHistoryReadyKey)

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
		return fmt.Errorf("cannot initialize percenter ClickHouse client: %w", err)
	}
	defer clickhouseConn.Close()

	startupCtx, startupCancel = context.WithTimeout(ctx, startupCheckTimeout)
	if err := verifyClickHouse(startupCtx, clickhouseConn, cfg); err != nil {
		startupCancel()
		return fmt.Errorf("ClickHouse startup check failed: %w", err)
	}
	startupCancel()
	log.Printf(
		"[PERCENTER][STARTUP] ClickHouse OK addr=%s database=%s tables=%s,%s,%s",
		net.JoinHostPort(cfg.Host, cfg.Port),
		cfg.Database,
		cfg.TableOrtb,
		cfg.TableImpressions,
		cfg.TableClicks,
	)

	bot := utils.NewBotMessage(cfg.BotBaseURL, cfg.BotInternalSecret)
	startupCtx, startupCancel = context.WithTimeout(ctx, startupCheckTimeout)
	if err := bot.SendTextMessageToBot(startupCtx, "[PERCENTER][STARTUP] dependency check OK"); err != nil {
		startupCancel()
		return fmt.Errorf("bot startup check failed: %w", err)
	}
	startupCancel()
	log.Printf("[PERCENTER][STARTUP] Bot OK base_url=%s", cfg.BotBaseURL)

	runtimeTelemetry := &percenterRuntimeTelemetry{}
	startPercenterTelemetry(ctx, redisClient, bot, runtimeTelemetry)

	dirtyNotifyAlert := services.NewRecoveryNotifier(bot, 5*time.Minute)
	pendingDeliveryAlert := services.NewRecoveryNotifier(bot, 5*time.Minute)
	stateHistoryAlert := services.NewRecoveryNotifier(bot, 5*time.Minute)
	store.StartHistoryDirtyNotifier(ctx, func(err error) {
		runtimeTelemetry.historyDirtyNotifyErrors.Add(1)
		msg := fmt.Sprintf("[PERCENTER][HISTORY_DIRTY_NOTIFY_ERROR] %v", err)
		log.Print(msg)
		dirtyNotifyAlert.Failure(ctx, msg)
	}, func() {
		dirtyNotifyAlert.Recovered(ctx, "[PERCENTER][HISTORY_DIRTY_NOTIFY_RECOVERED] dirty-segment notifications are healthy")
	})
	go runPendingHistoryFlusher(ctx, store, cfg.RedisPercenterHistoryReadyKey, pendingDeliveryAlert, runtimeTelemetry)
	log.Printf("[PERCENTER][STARTUP] async pending history delivery enabled redis_queue=%s", cfg.RedisPercenterHistoryReadyKey)

	log.Printf(
		"[PERCENTER][STARTUP] validation complete: margin_interval=%s ssp_reoptimize=%s simple_baseline_reoptimize=%s",
		policy.MarginOptimizeInterval,
		policy.SSPReoptimizeInterval,
		policy.SimpleBaselineReoptimizeInterval,
	)

	diagnosticsServer := percenterWeb.NewServer(redisClient, store, clickhouseConn, cfg, policy)
	diagnosticsServer.SetStateHistoryHealthReporter(
		func(callCtx context.Context, message string) { stateHistoryAlert.Failure(callCtx, message) },
		func(callCtx context.Context, message string) { stateHistoryAlert.Recovered(callCtx, message) },
	)
	router := httpServer.InitHttpRouter(chi.NewRouter())
	percenterWeb.InitHttpRoutes(router, diagnosticsServer)
	log.Println("HTTP routes initialized")
	go httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)

	runTick := func() {
		startedAt := time.Now().UTC()
		diagnosticsServer.RecordTickStart(startedAt)
		log.Printf(
			"[PERCENTER][TICK] started_at=%s interval=%s metrics_window=%s",
			startedAt.Format(time.RFC3339),
			policy.MarginOptimizeInterval,
			3*policy.MarginOptimizeInterval,
		)

		stats, err := processTick(ctx, clickhouseConn, store, cfg, policy, stateHistoryAlert, runtimeTelemetry)
		finishedAt := time.Now().UTC()
		diagnosticsServer.RecordTickFinish(finishedAt, err)
		if err != nil {
			runtimeTelemetry.tickErrors.Add(1)
			log.Printf("[PERCENTER][TICK_ERROR] duration=%s error=%v", finishedAt.Sub(startedAt), err)
			if sendErr := bot.SendTextMessageToBot(ctx, fmt.Sprintf("[PERCENTER][CLICKHOUSE_DOWN] %v", err)); sendErr != nil {
				log.Printf("[PERCENTER][TELEGRAM_ERROR] %v", sendErr)
			}
			return
		}
		log.Printf(
			"[PERCENTER][TICK] completed_at=%s duration=%s metrics=%d states_updated=%d rebenchmarks_started=%d fallback_decisions=%d fallback_routes_changed=%d fallback_no_eligible=%d",
			finishedAt.Format(time.RFC3339),
			finishedAt.Sub(startedAt),
			stats.MetricsLoaded,
			stats.StatesUpdated,
			stats.RebenchmarksStarted,
			stats.FallbackDecisions,
			stats.FallbackRoutesChanged,
			stats.FallbackNoEligible,
		)
	}

	// Decisions are made only on the configured cadence. A ClickHouse outage after
	// a successful startup is non-fatal: ADV keeps serving the last Redis state and
	// this worker alerts on every tick while ClickHouse remains unavailable.
	ticker := time.NewTicker(policy.MarginOptimizeInterval)
	defer ticker.Stop()

	rebenchmarkLogTicker := time.NewTicker(policy.SSPReoptimizeInterval)
	defer rebenchmarkLogTicker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-stop:
			return nil
		case now := <-rebenchmarkLogTicker.C:
			log.Printf(
				"[PERCENTER][REBENCHMARK_TIMER] at=%s smart_interval=%s simple_interval=%s note=per-segment_rebenchmark_runs_when_due_and_metrics_exist",
				now.UTC().Format(time.RFC3339),
				policy.SSPReoptimizeInterval,
				policy.SimpleBaselineReoptimizeInterval,
			)
		case <-ticker.C:
			runTick()
		}
	}
}

func validateRequiredEnvironment() error {
	missing := make([]string, 0)
	for _, name := range requiredPercenterEnv {
		value, ok := os.LookupEnv(name)
		if !ok || strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required environment variables are missing or empty: %s", strings.Join(missing, ", "))
	}
	return nil
}

func validatePercenterConfig(cfg *config.PercenterConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	if strings.TrimSpace(cfg.RedisADVAddr) == "" {
		return fmt.Errorf("REDIS_ADV_ADDR is empty")
	}
	if cfg.RedisDBAdvPercenter != 7 {
		return fmt.Errorf("REDIS_DB_ADV_PERCENTER must be 7, got %d", cfg.RedisDBAdvPercenter)
	}
	if strings.TrimSpace(cfg.RedisPercenterHistoryReadyKey) == "" {
		return fmt.Errorf("REDIS_PERCENTER_HISTORY_READY_KEY is empty")
	}
	if cfg.RedisPoolSize <= 0 {
		return fmt.Errorf("REDIS_POOL_SIZE must be > 0, got %d", cfg.RedisPoolSize)
	}
	if cfg.RedisMinIdleConns < 0 || cfg.RedisMinIdleConns > cfg.RedisPoolSize {
		return fmt.Errorf("REDIS_MIN_IDLE_CONNS must be between 0 and REDIS_POOL_SIZE, got %d (pool=%d)", cfg.RedisMinIdleConns, cfg.RedisPoolSize)
	}

	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("CLICKHOUSE_HOST is empty")
	}
	port, err := strconv.Atoi(strings.TrimSpace(cfg.Port))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("CLICKHOUSE_PORT must be a valid TCP port, got %q", cfg.Port)
	}
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("CLICKHOUSE_DB is empty")
	}
	if strings.TrimSpace(cfg.TableOrtb) == "" || strings.TrimSpace(cfg.TableImpressions) == "" || strings.TrimSpace(cfg.TableClicks) == "" {
		return fmt.Errorf("ClickHouse table names must not be empty")
	}

	if cfg.SSPReoptimizeInterval <= 0 {
		return fmt.Errorf("SSP_REOPTIMIZE_INTERVAL must be > 0")
	}
	if cfg.SimpleBaselineReoptimizeInterval <= 0 {
		return fmt.Errorf("SIMPLE_BASELINE_REOPTIMIZE_INTERVAL must be > 0")
	}
	if cfg.MarginOptimizeInterval <= 0 {
		return fmt.Errorf("MARGIN_OPTIMIZE_INTERVAL must be > 0")
	}
	if cfg.PercenterSegmentStateTTL <= 0 {
		return fmt.Errorf("PERCENTER_SEGMENT_STATE_TTL must be > 0")
	}
	if cfg.PercenterADVCacheTTL <= 0 {
		return fmt.Errorf("PERCENTER_ADV_CACHE_TTL must be > 0")
	}

	if err := validateUnitInterval("BUYOUT_RETENTION", cfg.BuyoutRetention); err != nil {
		return err
	}
	if err := validateUnitInterval("EFFICIENCY_RETENTION", cfg.EfficiencyRetention); err != nil {
		return err
	}
	if err := validateUnitInterval("SIMPLE_WIN_RATE_RETENTION", cfg.SimpleWinRateRetention); err != nil {
		return err
	}
	if err := validateMargin("DEFAULT_MIN_MARGIN", cfg.DefaultMinMargin); err != nil {
		return err
	}
	if err := validateMargin("PROMO_MIN_MARGIN", cfg.PromoMinMargin); err != nil {
		return err
	}
	if err := validateMargin("MAX_MARGIN", cfg.MaxMargin); err != nil {
		return err
	}
	if cfg.DefaultMinMargin > cfg.PromoMinMargin {
		return fmt.Errorf("DEFAULT_MIN_MARGIN %.4f must not exceed PROMO_MIN_MARGIN %.4f", cfg.DefaultMinMargin, cfg.PromoMinMargin)
	}
	if cfg.PromoMinMargin > cfg.MaxMargin {
		return fmt.Errorf("PROMO_MIN_MARGIN %.4f must not exceed MAX_MARGIN %.4f", cfg.PromoMinMargin, cfg.MaxMargin)
	}
	if cfg.SSPSearchPrecision <= 0 || cfg.SSPSearchPrecision >= 1 || math.IsNaN(cfg.SSPSearchPrecision) || math.IsInf(cfg.SSPSearchPrecision, 0) {
		return fmt.Errorf("SSP_SEARCH_PRECISION must be > 0 and < 1, got %v", cfg.SSPSearchPrecision)
	}
	if len(cfg.MarginSearchSteps) == 0 {
		return fmt.Errorf("MARGIN_SEARCH_STEPS must contain at least one positive step")
	}
	previous := math.Inf(1)
	for i, step := range cfg.MarginSearchSteps {
		if step <= 0 || step > 100 || math.IsNaN(step) || math.IsInf(step, 0) {
			return fmt.Errorf("MARGIN_SEARCH_STEPS[%d] must be > 0 and <= 100, got %v", i, step)
		}
		if step >= previous {
			return fmt.Errorf("MARGIN_SEARCH_STEPS must be strictly descending, got %v", []float64(cfg.MarginSearchSteps))
		}
		previous = step
	}

	if strings.TrimSpace(cfg.BotBaseURL) == "" {
		return fmt.Errorf("BOT_BASE_URL is empty")
	}
	if strings.TrimSpace(cfg.BotInternalSecret) == "" {
		return fmt.Errorf("BOT_INTERNAL_SECRET is empty")
	}
	if strings.TrimSpace(cfg.HttpServer.Host) == "" {
		return fmt.Errorf("HTTP_HOSTNAME is empty")
	}
	if cfg.HttpServer.Port == 0 {
		return fmt.Errorf("HTTP_PORT must be > 0")
	}

	return nil
}

func validateUnitInterval(name string, value float64) error {
	if value <= 0 || value > 1 || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be > 0 and <= 1, got %v", name, value)
	}
	return nil
}

func validateMargin(name string, value float64) error {
	if value <= 0 || value >= 1 || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be > 0 and < 1, got %v", name, value)
	}
	return nil
}

func verifyRedis(ctx context.Context, client *goredis.Client) error {
	if client == nil {
		return fmt.Errorf("client is nil")
	}
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("PING failed: %w", err)
	}

	key := fmt.Sprintf("percenter:startup-check:%d:%d", os.Getpid(), time.Now().UnixNano())
	const expected = "ok"
	if err := client.Set(ctx, key, expected, 30*time.Second).Err(); err != nil {
		return fmt.Errorf("write test failed: %w", err)
	}
	defer client.Del(context.Background(), key)

	actual, err := client.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("read test failed: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("read/write test returned unexpected value %q", actual)
	}
	if err := client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete test failed: %w", err)
	}
	return nil
}

func verifyClickHouse(ctx context.Context, conn clickhouse.Conn, cfg *config.PercenterConfig) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("PING failed: %w", err)
	}

	// Execute the real percenter metrics query over a tiny window. This validates
	// the configured database/table names as well as every column/join the worker
	// requires, without waiting five minutes for the first production tick.
	if _, err := percenter.LoadWindowMetrics(
		ctx,
		conn,
		cfg.Database,
		cfg.TableOrtb,
		cfg.TableImpressions,
		cfg.TableClicks,
		time.Second,
	); err != nil {
		return fmt.Errorf("metrics schema/query smoke test failed: %w", err)
	}
	if _, err := percenter.LoadFallbackTraffic(
		ctx,
		conn,
		cfg.Database,
		cfg.TableOrtb,
		cfg.TableImpressions,
		time.Second,
	); err != nil {
		return fmt.Errorf("fallback schema/query smoke test failed: %w", err)
	}
	return nil
}

type tickStats struct {
	MetricsLoaded         int
	StatesUpdated         int
	RebenchmarksStarted   int
	FallbackDecisions     int
	FallbackRoutesChanged int
	FallbackNoEligible    int
}

type stateHistoryTickHealth struct {
	notifier  *services.RecoveryNotifier
	failed    bool
	succeeded bool
}

func (h *stateHistoryTickHealth) failure(ctx context.Context, message string) {
	if h == nil {
		return
	}
	h.failed = true
	if h.notifier != nil {
		h.notifier.Failure(ctx, message)
	}
}

func (h *stateHistoryTickHealth) success() {
	if h != nil {
		h.succeeded = true
	}
}

func (h *stateHistoryTickHealth) finish(ctx context.Context) {
	if h == nil || h.notifier == nil || h.failed || !h.succeeded {
		return
	}
	h.notifier.Recovered(ctx, "[PERCENTER][STATE_HISTORY_RECOVERED] state/history Redis path is healthy")
}

func shouldReportStateHistoryError(err error) bool {
	return err != nil && !errors.Is(err, goredis.Nil)
}

func scheduledRebenchmarkDue(state percenter.State, policy percenter.Policy, now time.Time) (bool, time.Duration, time.Time) {
	policy = policy.Normalize()
	if state.TypeModel == percenter.TypeModelSimple {
		if state.LastSimpleBaselineAt.IsZero() {
			return false, policy.SimpleBaselineReoptimizeInterval, time.Time{}
		}
		return !now.Before(state.LastSimpleBaselineAt.Add(policy.SimpleBaselineReoptimizeInterval)), policy.SimpleBaselineReoptimizeInterval, state.LastSimpleBaselineAt
	}
	if state.LastSSPReoptimizeAt.IsZero() {
		return false, policy.SSPReoptimizeInterval, time.Time{}
	}
	return !now.Before(state.LastSSPReoptimizeAt.Add(policy.SSPReoptimizeInterval)), policy.SSPReoptimizeInterval, state.LastSSPReoptimizeAt
}

func applyFallbackRoutes(ctx context.Context, store *percenter.StateStore, decisions []percenter.FallbackDecision, now time.Time, health *stateHistoryTickHealth, telemetry *percenterRuntimeTelemetry) (map[string]struct{}, int, int) {
	stableEligible := make(map[string]struct{})
	routesChanged := 0
	noEligible := 0

	for _, decision := range decisions {
		if !decision.HasSelection {
			// Analyst rule: if even campaign has < 5 impressions in the last
			// five minutes, keep the current route/pricing unchanged.
			noEligible++
			continue
		}

		exactState, err := store.Load(ctx, decision.ExactHash)
		if err != nil {
			if errors.Is(err, goredis.Nil) {
				telemetry.fallbackRedisNil.Add(1)
			} else {
				telemetry.fallbackStateLoadErrors.Add(1)
			}
			message := fmt.Sprintf("[PERCENTER][FALLBACK_STATE_LOAD_SKIP] exact_hash=%s selected_hash=%s level=%s error=%v", decision.ExactHash, decision.SelectedHash, decision.SelectedLevel, err)
			log.Print(message)
			// redis.Nil only means this exact segment has no persisted state yet.
			// That is a normal baseline/fallback condition, not a Redis outage.
			if shouldReportStateHistoryError(err) {
				health.failure(ctx, message)
			}
			continue
		}
		health.success()

		currentHash := percenter.EffectiveStateHash(exactState)
		if currentHash == decision.SelectedHash {
			stableEligible[decision.SelectedHash] = struct{}{}
			continue
		}

		if decision.SelectedHash != decision.ExactHash {
			// Create/validate the parent state before routing production traffic to it.
			if _, err := store.GetOrInitPricingForCampaign(
				ctx,
				decision.SelectedHash,
				exactState.CampaignID,
				exactState.OriginalBid,
				exactState.MinMargin,
				exactState.CampaignVersion,
				exactState.TypeModel,
				exactState.ProfitModel,
				now,
			); err != nil {
				telemetry.fallbackParentInitErrors.Add(1)
				message := fmt.Sprintf("[PERCENTER][FALLBACK_PARENT_INIT_SKIP] exact_hash=%s selected_hash=%s level=%s error=%v", decision.ExactHash, decision.SelectedHash, decision.SelectedLevel, err)
				log.Print(message)
				health.failure(ctx, message)
				continue
			}
		}

		updated, changed := percenter.SetFallbackTarget(exactState, decision.SelectedHash, now)
		if !changed {
			stableEligible[decision.SelectedHash] = struct{}{}
			continue
		}
		event := percenter.FallbackHistoryEvent(exactState, updated, decision, currentHash, now)
		saved, err := store.SaveIfCurrentWithHistory(ctx, exactState, updated, event)
		if err != nil {
			telemetry.fallbackRouteSaveErrors.Add(1)
			message := fmt.Sprintf("[PERCENTER][FALLBACK_ROUTE_SAVE_ERROR] exact_hash=%s selected_hash=%s level=%s error=%v", decision.ExactHash, decision.SelectedHash, decision.SelectedLevel, err)
			log.Print(message)
			health.failure(ctx, message)
			continue
		}
		if !saved {
			log.Printf("[PERCENTER][FALLBACK_ROUTE_RACE_SKIP] exact_hash=%s selected_hash=%s level=%s", decision.ExactHash, decision.SelectedHash, decision.SelectedLevel)
			continue
		}
		health.success()
		routesChanged++
		log.Printf(
			"[PERCENTER][FALLBACK_ROUTE_CHANGED] exact_hash=%s from_hash=%s to_hash=%s level=%s impressions=%d window=%s",
			decision.ExactHash,
			currentHash,
			decision.SelectedHash,
			decision.SelectedLevel,
			decision.Impressions,
			percenter.FallbackWindow,
		)
		// Do not optimize a state in the same tick in which traffic is routed to
		// it. The next five-minute window gives that state fresh measurements.
	}
	return stableEligible, routesChanged, noEligible
}

func processTick(ctx context.Context, conn clickhouse.Conn, store *percenter.StateStore, cfg *config.PercenterConfig, policy percenter.Policy, stateHistoryAlert *services.RecoveryNotifier, telemetry *percenterRuntimeTelemetry) (tickStats, error) {
	traffic, err := percenter.LoadFallbackTraffic(
		ctx,
		conn,
		cfg.Database,
		cfg.TableOrtb,
		cfg.TableImpressions,
		percenter.FallbackWindow,
	)
	if err != nil {
		return tickStats{}, fmt.Errorf("load ClickHouse fallback traffic: %w", err)
	}
	decisions := percenter.BuildFallbackDecisions(traffic, percenter.FallbackMinImpressions)
	now := time.Now().UTC()
	health := &stateHistoryTickHealth{notifier: stateHistoryAlert}
	defer health.finish(ctx)
	stableEligible, routesChanged, noEligible := applyFallbackRoutes(ctx, store, decisions, now, health, telemetry)

	metrics, err := percenter.LoadWindowMetrics(
		ctx,
		conn,
		cfg.Database,
		cfg.TableOrtb,
		cfg.TableImpressions,
		cfg.TableClicks,
		3*policy.MarginOptimizeInterval,
	)
	if err != nil {
		return tickStats{}, fmt.Errorf("load ClickHouse window metrics: %w", err)
	}

	stats := tickStats{
		MetricsLoaded:         len(metrics),
		FallbackDecisions:     len(decisions),
		FallbackRoutesChanged: routesChanged,
		FallbackNoEligible:    noEligible,
	}
	for _, metric := range metrics {
		if _, eligible := stableEligible[metric.SegmentHash]; !eligible {
			continue
		}
		// A routing level may be eligible because its whole hierarchy has >= 5
		// impressions, while only a subset currently uses this particular state.
		// Never move the state itself until its current point has at least five
		// actual impressions too.
		if metric.Wins < percenter.FallbackMinImpressions {
			continue
		}

		state, err := store.Load(ctx, metric.SegmentHash)
		if err != nil {
			if errors.Is(err, goredis.Nil) {
				telemetry.stateRedisNil.Add(1)
			} else {
				telemetry.stateLoadErrors.Add(1)
			}
			// A state can expire between the auction and the worker tick. The next
			// ADV request recreates it from the current campaign snapshot.
			message := fmt.Sprintf("[PERCENTER][STATE_LOAD_SKIP] segment_hash=%s error=%v", metric.SegmentHash, err)
			log.Print(message)
			if shouldReportStateHistoryError(err) {
				health.failure(ctx, message)
			}
			continue
		}
		health.success()
		if metric.PointVersion != state.PointVersion {
			continue
		}
		rebenchmarkDue, rebenchmarkInterval, lastRebenchmarkAt := scheduledRebenchmarkDue(state, policy, now)
		updated, changed := percenter.Advance(state, metric, policy, now)
		if !changed {
			continue
		}
		eventType := "state_updated"
		if rebenchmarkDue {
			eventType = "rebenchmark"
		}
		event := percenter.StateUpdateHistoryEvent(state, updated, metric, eventType, now)
		saved, err := store.SaveIfCurrentWithHistory(ctx, state, updated, event)
		if err != nil {
			telemetry.stateSaveErrors.Add(1)
			message := fmt.Sprintf("[PERCENTER][STATE_SAVE_ERROR] segment_hash=%s error=%v", metric.SegmentHash, err)
			log.Print(message)
			health.failure(ctx, message)
			continue
		}
		if !saved {
			log.Printf("[PERCENTER][STATE_RACE_SKIP] segment_hash=%s", metric.SegmentHash)
			continue
		}
		health.success()
		stats.StatesUpdated++
		if rebenchmarkDue {
			stats.RebenchmarksStarted++
			log.Printf(
				"[PERCENTER][REBENCHMARK] segment_hash=%s type_model=%d interval=%s last_completed_at=%s from_phase=%s to_phase=%s old_point_version=%d new_point_version=%d",
				updated.SegmentHash,
				updated.TypeModel,
				rebenchmarkInterval,
				lastRebenchmarkAt.Format(time.RFC3339),
				state.Phase,
				updated.Phase,
				state.PointVersion,
				updated.PointVersion,
			)
		}
		log.Printf(
			"[PERCENTER][STATE_UPDATED] segment_hash=%s type_model=%d point_version=%d phase=%s advertiser_price=%.6f ssp_bid=%.6f margin=%.4f requests=%d wins=%d clicks=%d profit_per_request=%.9f",
			updated.SegmentHash,
			updated.TypeModel,
			updated.PointVersion,
			updated.Phase,
			updated.AdvertiserPrice,
			updated.SSPBid,
			updated.Margin,
			metric.Requests,
			metric.Wins,
			metric.Clicks,
			metric.ProfitPerRequestFor(updated.ProfitModel),
		)
	}
	return stats, nil
}

func policyFromConfig(cfg config.PercenterAlgorithmConfig) percenter.Policy {
	return percenter.Policy{
		BuyoutRetention:                  cfg.BuyoutRetention,
		EfficiencyRetention:              cfg.EfficiencyRetention,
		SimpleWinRateRetention:           cfg.SimpleWinRateRetention,
		DefaultMinMargin:                 cfg.DefaultMinMargin,
		PromoMinMargin:                   cfg.PromoMinMargin,
		MaxMargin:                        cfg.MaxMargin,
		SSPSearchPrecision:               cfg.SSPSearchPrecision,
		MarginSearchStepsPP:              append([]float64(nil), cfg.MarginSearchSteps...),
		SSPReoptimizeInterval:            cfg.SSPReoptimizeInterval,
		SimpleBaselineReoptimizeInterval: cfg.SimpleBaselineReoptimizeInterval,
		MarginOptimizeInterval:           cfg.MarginOptimizeInterval,
		SegmentStateTTL:                  cfg.PercenterSegmentStateTTL,
		ADVCacheTTL:                      cfg.PercenterADVCacheTTL,
	}.Normalize()
}
