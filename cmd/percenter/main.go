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
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[PERCENTER][FATAL] %v", err)
	}
}

func run() error {
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	cfg, err := config.LoadConfig[config.PercenterConfig](ctx)
	if err != nil {
		return fmt.Errorf("load percenter config: %w", err)
	}
	simplePolicy, complexPolicy, err := percenter.PoliciesFromConfig(cfg.PercenterPolicyConfig)
	if err != nil {
		return fmt.Errorf("invalid percenter policy config: %w", err)
	}
	policyFingerprint := percenter.PolicyFingerprint(simplePolicy, complexPolicy)
	log.Printf("[PERCENTER][POLICY] fingerprint=%s history_pending_ttl=%s simple_ttl=%s complex_ttl=%s simple_steps=%v complex_ssp_steps=%v complex_margin_steps=%v", policyFingerprint, simplePolicy.PendingHistoryTTL, simplePolicy.StateTTL, complexPolicy.StateTTL, simplePolicy.SearchStepsPP, complexPolicy.SSPSearchStepsPercent, complexPolicy.MarginSearchStepsPP)
	if cfg.RedisDBAdvPercenter != 7 {
		return fmt.Errorf("REDIS_DB_ADV_PERCENTER=%d is invalid: production invariant requires 7", cfg.RedisDBAdvPercenter)
	}
	if strings.TrimSpace(cfg.RedisADVAddr) == "" {
		return fmt.Errorf("REDIS_ADV_ADDR is required")
	}

	degradedDependencies := make([]string, 0, 3)
	redisState := "enabled"
	clickhouseState := "enabled"
	kafkaState := "enabled"
	redisClient, err := redisService.NewRedisClient(
		strings.TrimSpace(cfg.RedisADVAddr), cfg.RedisPassword, cfg.RedisDBAdvPercenter, cfg.RedisPoolSize, cfg.RedisMinIdleConns,
	)
	if err != nil {
		return fmt.Errorf("initialize percenter Redis: %w", err)
	}
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		redisState = "degraded"
		degradedDependencies = append(degradedDependencies, "redis_db7")
		log.Printf("[PERCENTER][REDIS_DEGRADED] redis_db=%d error=%v", cfg.RedisDBAdvPercenter, err)
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
		clickhouseState = "degraded"
		degradedDependencies = append(degradedDependencies, "clickhouse")
		log.Printf("[PERCENTER][CLICKHOUSE_DEGRADED] error=%v", err)
	}

	observabilityOutbox, err := percenter.OpenObservabilityOutbox(cfg.PercenterOutboxPath)
	if err != nil {
		return fmt.Errorf("open percenter observability outbox: %w", err)
	}
	defer observabilityOutbox.Close()
	if len(cfg.KafkaBrokers) == 0 {
		return fmt.Errorf("KAFKA_BROKERS is invalid: at least one broker is required for durable history delivery")
	}
	if strings.TrimSpace(cfg.KafkaTopicPercenter) == "" {
		return fmt.Errorf("KAFKA_TOPIC_PERCENTER=%q is invalid: a non-empty topic is required", cfg.KafkaTopicPercenter)
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
	if err := probeKafkaBrokers(probeCtx, cfg.KafkaBrokers); err != nil {
		kafkaState = "degraded"
		// A configured but temporarily unavailable broker must not prevent the
		// optimizer process from starting. Redis/bbolt retain pending delivery.
		degradedDependencies = append(degradedDependencies, "kafka")
		log.Printf("[PERCENTER][KAFKA_DEGRADED] error=%v", err)
	}
	probeCancel()
	kafkaWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.KafkaBrokers...),
		Topic:        strings.TrimSpace(cfg.KafkaTopicPercenter),
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}
	defer kafkaWriter.Close()
	sink := &percenter.ObservabilitySink{
		Redis: redisClient, Kafka: kafkaWriter, ClickHouse: clickhouseConn,
		HistoryTable: cfg.PercenterHistoryTable, TelemetryTable: cfg.PercenterTelemetryTable,
	}
	transitions := percenter.NewDependencyTransitions()
	var simpleUpdates atomic.Uint64
	var complexUpdates atomic.Uint64
	var optimizerAnomalies atomic.Uint64
	var botNotifier *utils.BotMessage
	botURL := strings.TrimSpace(cfg.BotBaseURL)
	botSecret := strings.TrimSpace(cfg.BotInternalSecret)
	if botURL != "" && botSecret != "" {
		botNotifier = utils.NewBotMessage(botURL, botSecret)
	} else if botURL != "" || botSecret != "" {
		log.Printf("[PERCENTER][TELEGRAM_DISABLED] BOT_BASE_URL and BOT_INTERNAL_SECRET must both be set; continuing without Telegram")
	} else {
		log.Printf("[PERCENTER][TELEGRAM_DISABLED] credentials are not configured")
	}
	telegramState := "disabled"
	if botNotifier != nil {
		telegramState = "enabled"
	}
	log.Printf(
		"[PERCENTER][COMPONENTS] redis_state=%s redis_db=%d clickhouse=%s kafka=%s telegram=%s local_outbox=enabled",
		redisState, cfg.RedisDBAdvPercenter, clickhouseState, kafkaState, telegramState,
	)
	sendBotAsync := func(logPrefix, text string) {
		if botNotifier == nil {
			return
		}
		go func() {
			notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer notifyCancel()
			if err := botNotifier.SendTextMessageToBot(notifyCtx, text); err != nil {
				log.Printf("%s %v", logPrefix, err)
			}
		}()
	}
	notifyTransition := func(name string, failed bool, detail error) {
		transition := transitions.Update(name, failed)
		if transition == "" {
			return
		}
		message := fmt.Sprintf("[PERCENTER][%s] dependency=%s", transition, name)
		if detail != nil {
			message += fmt.Sprintf(" error=%v", detail)
		}
		log.Print(message)
		sendBotAsync("[PERCENTER][TELEGRAM_ERROR]", message)
	}
	relayInterval := cfg.PercenterRelayInterval
	if relayInterval <= 0 {
		relayInterval = time.Minute
	}
	var backgroundWG sync.WaitGroup
	backgroundWG.Add(1)
	go func() {
		defer backgroundWG.Done()
		ticker := time.NewTicker(relayInterval)
		defer ticker.Stop()
		runDelivery := func(deliveryCtx context.Context) {
			_, pendingErr := percenter.RecoverPendingHistory(deliveryCtx, redisClient, observabilityOutbox, 1000)
			notifyTransition("redis_db7_pending_history", pendingErr != nil, pendingErr)
			if pendingErr != nil {
				return
			}
			relayErr := percenter.RelayObservabilityOutbox(deliveryCtx, observabilityOutbox, redisClient)
			notifyTransition("redis_db7_delivery", relayErr != nil, relayErr)
			if relayErr != nil {
				return
			}
			_, sinkErr := sink.DrainOnce(deliveryCtx, 1000)
			notifyTransition("kafka_clickhouse_delivery", sinkErr != nil, sinkErr)
			if sinkErr != nil {
			}
		}
		runDelivery(ctx)
		for {
			select {
			case <-ctx.Done():
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
				runDelivery(shutdownCtx)
				shutdownCancel()
				return
			case <-ticker.C:
				runDelivery(ctx)
			}
		}
	}()
	digestInterval := cfg.PercenterDigestInterval
	if digestInterval <= 0 {
		digestInterval = 30 * time.Minute
	}
	backgroundWG.Add(1)
	go func() {
		defer backgroundWG.Done()
		ticker := time.NewTicker(digestInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				localBacklog, _ := observabilityOutbox.Count()
				redisBacklog := percenter.RedisObservabilityBacklog(ctx, redisClient)
				message := fmt.Sprintf(
					"[PERCENTER][30M_HEALTH] active_errors=%v local_outbox=%d redis_ready=%d simple_updates=%d complex_updates=%d anomalies=%d",
					transitions.Active(), localBacklog, redisBacklog, simpleUpdates.Swap(0), complexUpdates.Swap(0), optimizerAnomalies.Swap(0),
				)
				sendBotAsync("[PERCENTER][TELEGRAM_DIGEST_ERROR]", message)
			}
		}
	}()

	degradedState := "none"
	if len(degradedDependencies) > 0 {
		degradedState = strings.Join(degradedDependencies, ",")
	}
	log.Printf(
		"[PERCENTER][STARTUP] fingerprint=%s simple_state_ttl=%s complex_state_ttl=%s history_pending_ttl=%s redis_db=%d local_outbox=enabled redis_relay=enabled kafka=%s clickhouse=%s telegram=%s degraded=%s kafka_topic=%q simple_interval=%s simple_rebenchmark=%s simple_min_impressions=%d simple_retention=%.2f simple_steps=%v complex_interval=%s complex_rebenchmark=%s complex_min_impressions=%d complex_buyout_retention=%.2f complex_efficiency_retention=%.2f complex_ssp_steps=%v complex_margin_steps=%v max_margin=%.2f",
		policyFingerprint, simplePolicy.StateTTL, complexPolicy.StateTTL, simplePolicy.PendingHistoryTTL, cfg.RedisDBAdvPercenter, kafkaState, clickhouseState, telegramState, degradedState, strings.TrimSpace(cfg.KafkaTopicPercenter),
		simplePolicy.OptimizeInterval, simplePolicy.RebenchmarkInterval, simplePolicy.MinImpressions, simplePolicy.WinRateRetention, simplePolicy.SearchStepsPP,
		complexPolicy.OptimizeInterval, complexPolicy.RebenchmarkInterval, complexPolicy.MinImpressions, complexPolicy.BuyoutRetention, complexPolicy.EfficiencyRetention, complexPolicy.SSPSearchStepsPercent, complexPolicy.MarginSearchStepsPP, complexPolicy.MaxMargin,
	)

	runSimpleTick := func(tickCtx context.Context, now time.Time) {
		metrics, loadErr := percenter.LoadSimpleWindowMetrics(
			tickCtx, clickhouseConn, cfg.Clickhouse.Database, cfg.Clickhouse.TableOrtb, cfg.Clickhouse.TableImpressions, cfg.Clickhouse.TableClicks, simplePolicy.OptimizeInterval,
		)
		if loadErr != nil {
			notifyTransition("clickhouse_simple_metrics", true, loadErr)
			return
		}
		notifyTransition("clickhouse_simple_metrics", false, nil)
		metricIndex := percenter.NewSimpleMetricsIndex(metrics)
		states, stateErr := simpleStore.States(tickCtx)
		if stateErr != nil {
			notifyTransition("redis_db7_simple_state", true, stateErr)
			return
		}
		notifyTransition("redis_db7_simple_state", false, nil)
		for _, state := range states {
			if state.TypeModel != percenter.TypeModelSimple {
				continue
			}
			if state.PendingHistory != nil {
				if err := percenter.PersistSimplePendingHistory(tickCtx, simpleStore, observabilityOutbox, state); err != nil {
					notifyTransition("simple_pending_history", true, err)
					optimizerAnomalies.Add(1)
					continue
				}
				notifyTransition("simple_pending_history", false, nil)
				state.PendingHistory = nil
			}
			metric, ok := metricIndex.ForState(state)
			if !ok {
				continue
			}
			next, changed := percenter.AdvanceSimple(state, metric, simplePolicy, now)
			if !changed {
				continue
			}
			event, ok := percenter.BuildSimpleHistoryEvent(state, next, metric)
			if !ok {
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][SIMPLE][HISTORY_BUILD_ERROR] segment_hash=%s point_version=%d", state.SegmentHash, state.PointVersion)
				continue
			}
			next.PendingHistory = &event
			saved, saveErr := simpleStore.SaveCAS(tickCtx, next, state.PointVersion)
			if saveErr != nil {
				notifyTransition("redis_db7_simple_state", true, saveErr)
				optimizerAnomalies.Add(1)
				continue
			}
			notifyTransition("redis_db7_simple_state", false, nil)
			if !saved {
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][SIMPLE][STATE_RACE_SKIP] segment_hash=%s expected_point_version=%d", state.SegmentHash, state.PointVersion)
				continue
			}
			simpleUpdates.Add(1)
			if err := percenter.PersistSimplePendingHistory(tickCtx, simpleStore, observabilityOutbox, next); err != nil {
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][SIMPLE][HISTORY_OUTBOX_ERROR] segment_hash=%s point_version=%d event_id=%s error=%v", next.SegmentHash, next.PointVersion, event.EventID, err)
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

	runComplexTick := func(tickCtx context.Context, now time.Time) {
		metrics, loadErr := percenter.LoadComplexWindowMetrics(
			tickCtx, clickhouseConn, cfg.Clickhouse.Database, cfg.Clickhouse.TableOrtb, cfg.Clickhouse.TableImpressions, cfg.Clickhouse.TableClicks, complexPolicy.OptimizeInterval,
		)
		if loadErr != nil {
			notifyTransition("clickhouse_complex_metrics", true, loadErr)
			return
		}
		notifyTransition("clickhouse_complex_metrics", false, nil)
		metricIndex := percenter.NewComplexMetricsIndex(metrics)
		states, stateErr := complexStore.States(tickCtx)
		if stateErr != nil {
			notifyTransition("redis_db7_complex_state", true, stateErr)
			return
		}
		notifyTransition("redis_db7_complex_state", false, nil)
		for _, state := range states {
			if state.TypeModel != percenter.TypeModelComplex {
				continue
			}
			if state.PendingHistory != nil {
				if err := percenter.PersistComplexPendingHistory(tickCtx, complexStore, observabilityOutbox, state); err != nil {
					notifyTransition("complex_pending_history", true, err)
					optimizerAnomalies.Add(1)
					continue
				}
				notifyTransition("complex_pending_history", false, nil)
				state.PendingHistory = nil
			}
			metric, ok := metricIndex.ForState(state)
			if !ok {
				continue
			}
			next, changed := percenter.AdvanceComplex(state, metric, complexPolicy, now)
			if !changed {
				continue
			}
			event, ok := percenter.BuildComplexHistoryEvent(state, next, metric)
			if !ok {
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][COMPLEX][HISTORY_BUILD_ERROR] segment_hash=%s point_version=%d", state.SegmentHash, state.PointVersion)
				continue
			}
			next.PendingHistory = &event
			saved, saveErr := complexStore.SaveCAS(tickCtx, next, state.PointVersion)
			if saveErr != nil {
				notifyTransition("redis_db7_complex_state", true, saveErr)
				optimizerAnomalies.Add(1)
				continue
			}
			notifyTransition("redis_db7_complex_state", false, nil)
			if !saved {
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][COMPLEX][STATE_RACE_SKIP] segment_hash=%s expected_point_version=%d", state.SegmentHash, state.PointVersion)
				continue
			}
			complexUpdates.Add(1)
			if err := percenter.PersistComplexPendingHistory(tickCtx, complexStore, observabilityOutbox, next); err != nil {
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][COMPLEX][HISTORY_OUTBOX_ERROR] segment_hash=%s point_version=%d event_id=%s error=%v", next.SegmentHash, next.PointVersion, event.EventID, err)
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
		runBoundedOptimizerTick(ctx, simplePolicy.OptimizeInterval, now, runSimpleTick, runComplexTick)
	}

	// Both policies are fixed to 5m by validation below, so one ticker services
	// both optimizers without changing Simple cadence. The tick context is a
	// child of the signal-aware process context, therefore SIGTERM cancels a
	// blocking ClickHouse/Redis operation instead of waiting for runTick.
	if ctx.Err() == nil {
		runTick(time.Now().UTC())
	}
	ticker := time.NewTicker(simplePolicy.OptimizeInterval)
	defer ticker.Stop()

	backgroundShutdownAttempted := false
	shutdownBackground := func() error {
		stopSignals()
		backgroundShutdownAttempted = true
		if !waitForBackground(&backgroundWG, 6*time.Second) {
			return fmt.Errorf("percenter background shutdown timed out after %s", 6*time.Second)
		}
		return nil
	}
	// Keep cleanup ahead of the resource-closing defers even if a future runtime
	// error adds another return path after workers have started. main() calls
	// log.Fatalf only after run() returns, so this bounded cleanup runs first.
	defer func() {
		if backgroundShutdownAttempted {
			return
		}
		stopSignals()
		_ = waitForBackground(&backgroundWG, 6*time.Second)
	}()

	for {
		select {
		case <-ctx.Done():
			return shutdownBackground()
		case now := <-ticker.C:
			if ctx.Err() != nil {
				return shutdownBackground()
			}
			runTick(now.UTC())
		}
	}
}

func runBoundedOptimizerTick(
	parent context.Context,
	timeout time.Duration,
	now time.Time,
	runSimple func(context.Context, time.Time),
	runComplex func(context.Context, time.Time),
) {
	if parent == nil || parent.Err() != nil {
		return
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	tickCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if runSimple != nil {
		runSimple(tickCtx, now)
	}
	if tickCtx.Err() != nil {
		return
	}
	if runComplex != nil {
		runComplex(tickCtx, now)
	}
}

func probeKafkaBrokers(ctx context.Context, brokers []string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("Kafka brokers list is empty")
	}
	var lastErr error
	for _, broker := range brokers {
		broker = strings.TrimSpace(broker)
		if broker == "" {
			continue
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", broker)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Kafka brokers list contains no usable address")
	}
	return lastErr
}

func waitForBackground(wg *sync.WaitGroup, timeout time.Duration) bool {
	if wg == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	if timeout <= 0 {
		<-done
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		log.Printf("[PERCENTER][SHUTDOWN_TIMEOUT] background workers did not stop within %s", timeout)
		return false
	}
}
