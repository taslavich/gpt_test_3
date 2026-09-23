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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.PercenterConfig](ctx)
	if err != nil {
		return fmt.Errorf("load percenter config: %w", err)
	}
	simplePolicy, complexPolicy, err := percenter.PoliciesFromConfig(cfg.PercenterPolicyConfig)
	if err != nil {
		return fmt.Errorf("invalid percenter policy config: %w", err)
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
		log.Printf("[PERCENTER][CLICKHOUSE_DEGRADED] error=%v", err)
	}

	observabilityOutbox, err := percenter.OpenObservabilityOutbox(cfg.PercenterOutboxPath)
	if err != nil {
		return fmt.Errorf("open percenter observability outbox: %w", err)
	}
	defer observabilityOutbox.Close()
	if len(cfg.KafkaBrokers) == 0 {
		return fmt.Errorf("KAFKA_BROKERS is required for durable history delivery")
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
	if err := probeKafkaBrokers(probeCtx, cfg.KafkaBrokers); err != nil {
		// A configured but temporarily unavailable broker must not prevent the
		// optimizer process from starting. Redis/bbolt retain pending delivery.
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
				log.Printf("[PERCENTER][PENDING_HISTORY_RECOVERY_ERROR] %v", pendingErr)
				return
			}
			relayErr := percenter.RelayObservabilityOutbox(deliveryCtx, observabilityOutbox, redisClient)
			notifyTransition("redis_db7_delivery", relayErr != nil, relayErr)
			if relayErr != nil {
				log.Printf("[PERCENTER][OBSERVABILITY_RELAY_ERROR] %v", relayErr)
				return
			}
			_, sinkErr := sink.DrainOnce(deliveryCtx, 1000)
			notifyTransition("kafka_clickhouse_delivery", sinkErr != nil, sinkErr)
			if sinkErr != nil {
				log.Printf("[PERCENTER][OBSERVABILITY_SINK_ERROR] %v", sinkErr)
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

	log.Printf(
		"[PERCENTER][STARTUP] redis_db=%d simple_interval=%s simple_rebenchmark=%s simple_ttl=%s simple_min_impressions=%d simple_retention=%.2f simple_steps=%v complex_interval=%s complex_rebenchmark=%s complex_ttl=%s complex_min_impressions=%d complex_buyout_retention=%.2f complex_efficiency_retention=%.2f complex_ssp_steps=%v complex_margin_steps=%v max_margin=%.2f",
		cfg.RedisDBAdvPercenter, simplePolicy.OptimizeInterval, simplePolicy.RebenchmarkInterval, simplePolicy.StateTTL, simplePolicy.MinImpressions, simplePolicy.WinRateRetention, simplePolicy.SearchStepsPP,
		complexPolicy.OptimizeInterval, complexPolicy.RebenchmarkInterval, complexPolicy.StateTTL, complexPolicy.MinImpressions, complexPolicy.BuyoutRetention, complexPolicy.EfficiencyRetention, complexPolicy.SSPSearchStepsPercent, complexPolicy.MarginSearchStepsPP, complexPolicy.MaxMargin,
	)

	runSimpleTick := func(now time.Time) {
		metrics, loadErr := percenter.LoadSimpleWindowMetrics(
			ctx, clickhouseConn, cfg.Clickhouse.Database, cfg.Clickhouse.TableOrtb, cfg.Clickhouse.TableImpressions, cfg.Clickhouse.TableClicks, simplePolicy.OptimizeInterval,
		)
		if loadErr != nil {
			notifyTransition("clickhouse_simple_metrics", true, loadErr)
			log.Printf("[PERCENTER][SIMPLE][METRICS_ERROR] %v", loadErr)
			return
		}
		notifyTransition("clickhouse_simple_metrics", false, nil)
		metricIndex := percenter.NewSimpleMetricsIndex(metrics)
		states, stateErr := simpleStore.States(ctx)
		if stateErr != nil {
			notifyTransition("redis_db7_simple_state", true, stateErr)
			log.Printf("[PERCENTER][SIMPLE][STATE_LIST_ERROR] %v", stateErr)
			return
		}
		notifyTransition("redis_db7_simple_state", false, nil)
		for _, state := range states {
			if state.TypeModel != percenter.TypeModelSimple {
				continue
			}
			if state.PendingHistory != nil {
				if err := percenter.PersistSimplePendingHistory(ctx, simpleStore, observabilityOutbox, state); err != nil {
					optimizerAnomalies.Add(1)
					log.Printf("[PERCENTER][SIMPLE][PENDING_HISTORY_RECOVERY_ERROR] segment_hash=%s point_version=%d event_id=%s error=%v", state.SegmentHash, state.PointVersion, state.PendingHistory.EventID, err)
					continue
				}
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
			saved, saveErr := simpleStore.SaveCAS(ctx, next, state.PointVersion)
			if saveErr != nil {
				notifyTransition("redis_db7_simple_state", true, saveErr)
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][SIMPLE][STATE_SAVE_ERROR] segment_hash=%s error=%v", state.SegmentHash, saveErr)
				continue
			}
			notifyTransition("redis_db7_simple_state", false, nil)
			if !saved {
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][SIMPLE][STATE_RACE_SKIP] segment_hash=%s expected_point_version=%d", state.SegmentHash, state.PointVersion)
				continue
			}
			simpleUpdates.Add(1)
			if err := percenter.PersistSimplePendingHistory(ctx, simpleStore, observabilityOutbox, next); err != nil {
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

	runComplexTick := func(now time.Time) {
		metrics, loadErr := percenter.LoadComplexWindowMetrics(
			ctx, clickhouseConn, cfg.Clickhouse.Database, cfg.Clickhouse.TableOrtb, cfg.Clickhouse.TableImpressions, cfg.Clickhouse.TableClicks, complexPolicy.OptimizeInterval,
		)
		if loadErr != nil {
			notifyTransition("clickhouse_complex_metrics", true, loadErr)
			log.Printf("[PERCENTER][COMPLEX][METRICS_ERROR] %v", loadErr)
			return
		}
		notifyTransition("clickhouse_complex_metrics", false, nil)
		metricIndex := percenter.NewComplexMetricsIndex(metrics)
		states, stateErr := complexStore.States(ctx)
		if stateErr != nil {
			notifyTransition("redis_db7_complex_state", true, stateErr)
			log.Printf("[PERCENTER][COMPLEX][STATE_LIST_ERROR] %v", stateErr)
			return
		}
		notifyTransition("redis_db7_complex_state", false, nil)
		for _, state := range states {
			if state.TypeModel != percenter.TypeModelComplex {
				continue
			}
			if state.PendingHistory != nil {
				if err := percenter.PersistComplexPendingHistory(ctx, complexStore, observabilityOutbox, state); err != nil {
					optimizerAnomalies.Add(1)
					log.Printf("[PERCENTER][COMPLEX][PENDING_HISTORY_RECOVERY_ERROR] segment_hash=%s point_version=%d event_id=%s error=%v", state.SegmentHash, state.PointVersion, state.PendingHistory.EventID, err)
					continue
				}
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
			saved, saveErr := complexStore.SaveCAS(ctx, next, state.PointVersion)
			if saveErr != nil {
				notifyTransition("redis_db7_complex_state", true, saveErr)
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][COMPLEX][STATE_SAVE_ERROR] segment_hash=%s error=%v", state.SegmentHash, saveErr)
				continue
			}
			notifyTransition("redis_db7_complex_state", false, nil)
			if !saved {
				optimizerAnomalies.Add(1)
				log.Printf("[PERCENTER][COMPLEX][STATE_RACE_SKIP] segment_hash=%s expected_point_version=%d", state.SegmentHash, state.PointVersion)
				continue
			}
			complexUpdates.Add(1)
			if err := percenter.PersistComplexPendingHistory(ctx, complexStore, observabilityOutbox, next); err != nil {
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
			waitForBackground(&backgroundWG, 6*time.Second)
			return nil
		case <-stop:
			cancel()
			waitForBackground(&backgroundWG, 6*time.Second)
			return nil
		case now := <-ticker.C:
			runTick(now.UTC())
		}
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
