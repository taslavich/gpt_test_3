package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

type percenterRuntimeTelemetry struct {
	fallbackRedisNil          atomic.Uint64
	fallbackStateLoadErrors   atomic.Uint64
	fallbackParentInitErrors  atomic.Uint64
	fallbackRouteSaveErrors   atomic.Uint64
	stateRedisNil             atomic.Uint64
	stateLoadErrors           atomic.Uint64
	stateSaveErrors           atomic.Uint64
	historyDirtyNotifyErrors  atomic.Uint64
	historyPendingErrors      atomic.Uint64
	tickErrors                atomic.Uint64
	telemetryPublishFailures  atomic.Uint64
	telemetryPublishRecovered atomic.Uint64
}

func (t *percenterRuntimeTelemetry) totals() percenter.TelemetryTotals {
	if t == nil {
		return percenter.TelemetryTotals{}
	}
	return percenter.TelemetryTotals{
		FallbackRedisNil:          t.fallbackRedisNil.Load(),
		FallbackStateLoadErrors:   t.fallbackStateLoadErrors.Load(),
		FallbackParentInitErrors:  t.fallbackParentInitErrors.Load(),
		FallbackRouteSaveErrors:   t.fallbackRouteSaveErrors.Load(),
		StateRedisNil:             t.stateRedisNil.Load(),
		StateLoadErrors:           t.stateLoadErrors.Load(),
		StateSaveErrors:           t.stateSaveErrors.Load(),
		HistoryDirtyNotifyErrors:  t.historyDirtyNotifyErrors.Load(),
		HistoryPendingErrors:      t.historyPendingErrors.Load(),
		TickErrors:                t.tickErrors.Load(),
		TelemetryPublishFailures:  t.telemetryPublishFailures.Load(),
		TelemetryPublishRecovered: t.telemetryPublishRecovered.Load(),
	}
}

func startPercenterTelemetry(
	ctx context.Context,
	client *redis.Client,
	bot *utils.BotMessage,
	counters *percenterRuntimeTelemetry,
) {
	if client == nil || counters == nil {
		return
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	startedAt := time.Now().UTC()
	instanceID := fmt.Sprintf("%d-%d", os.Getpid(), startedAt.UnixNano())

	go percenter.RunTelemetryPublisher(
		ctx,
		client,
		func(now time.Time) percenter.TelemetrySnapshot {
			return percenter.TelemetrySnapshot{
				Version:    1,
				Source:     "percenter",
				Hostname:   hostname,
				InstanceID: instanceID,
				StartedAt:  startedAt,
				UpdatedAt:  now,
				Totals:     counters.totals(),
			}
		},
		func(attempt uint64, publishErr error) {
			counters.telemetryPublishFailures.Add(1)
			log.Printf(
				"[PERCENTER][TELEMETRY_UPLOAD_RETRY] host=%s attempt=%d next_retry=%s error=%v",
				hostname,
				attempt,
				percenter.TelemetryPublishInterval,
				publishErr,
			)
		},
		func(attempt uint64, publishErr error) {
			message := fmt.Sprintf(
				"[PERCENTER][TELEMETRY_REDIS_UNAVAILABLE] host=%s failed_attempts=%d retry_interval=%s error=%v",
				hostname,
				attempt,
				percenter.TelemetryPublishInterval,
				publishErr,
			)
			log.Print(message)
			if bot != nil {
				if sendErr := bot.SendTextMessageToBot(ctx, message); sendErr != nil {
					log.Printf("[PERCENTER][TELEMETRY_TELEGRAM_ERROR] %v", sendErr)
				}
			}
		},
		func(failures uint64) {
			counters.telemetryPublishRecovered.Add(1)
			log.Printf("[PERCENTER][TELEMETRY_UPLOAD_RECOVERED] host=%s failed_attempts=%d", hostname, failures)
			if failures < percenter.TelemetryFailureAlertAfter || bot == nil {
				return
			}
			message := fmt.Sprintf(
				"[PERCENTER][TELEMETRY_REDIS_RECOVERED] host=%s after_failed_attempts=%d telemetry uploads are healthy",
				hostname,
				failures,
			)
			if sendErr := bot.SendTextMessageToBot(ctx, message); sendErr != nil {
				log.Printf("[PERCENTER][TELEMETRY_TELEGRAM_ERROR] %v", sendErr)
			}
		},
		nil,
	)
	log.Printf(
		"[PERCENTER][TELEMETRY] publisher_started host=%s interval=%s alert_after_failures=%d digest_interval=%s",
		hostname,
		percenter.TelemetryPublishInterval,
		percenter.TelemetryFailureAlertAfter,
		percenter.TelemetryDigestInterval,
	)

	go runPercenterTelemetryDigest(ctx, client, bot)
}

func runPercenterTelemetryDigest(ctx context.Context, client *redis.Client, bot *utils.BotMessage) {
	if client == nil || bot == nil {
		return
	}
	ticker := time.NewTicker(percenter.TelemetryDigestInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			sendPercenterTelemetryDigest(ctx, client, bot, now.UTC())
		}
	}
}

func sendPercenterTelemetryDigest(ctx context.Context, client *redis.Client, bot *utils.BotMessage, now time.Time) {
	snapshots, err := percenter.LoadTelemetrySnapshots(ctx, client, now)
	if err != nil {
		log.Printf("[PERCENTER][30M_HEALTH_DIGEST_SKIP] load_snapshots_error=%v", err)
		return
	}
	checkpoints, lastAt, err := percenter.LoadTelemetryDigestState(ctx, client)
	if err != nil {
		log.Printf("[PERCENTER][30M_HEALTH_DIGEST_SKIP] load_checkpoint_error=%v", err)
		return
	}

	var advTotal percenter.TelemetryTotals
	var percenterTotal percenter.TelemetryTotals
	advByHost := make(map[string]percenter.TelemetryTotals)
	newInstances := 0
	advInstances := 0
	percenterInstances := 0
	for _, snapshot := range snapshots {
		previous, exists := checkpoints[snapshot.Key()]
		if !exists {
			newInstances++
		}
		delta := snapshot.Totals.Delta(previous)
		switch snapshot.Source {
		case "adv":
			advInstances++
			advTotal = advTotal.Add(delta)
			advByHost[snapshot.Hostname] = advByHost[snapshot.Hostname].Add(delta)
		case "percenter":
			percenterInstances++
			percenterTotal = percenterTotal.Add(delta)
		}
	}

	status := telemetryDigestStatus(advTotal, percenterTotal)
	periodStart := lastAt
	periodLabel := "first digest; new instance totals are since process start"
	if !periodStart.IsZero() {
		periodLabel = fmt.Sprintf("%s -> %s", periodStart.Format(time.RFC3339), now.Format(time.RFC3339))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[PERCENTER][30M_HEALTH]\n")
	fmt.Fprintf(&b, "period: %s\n", periodLabel)
	fmt.Fprintf(&b, "status: %s redis_snapshot_path=HEALTHY snapshots=%d new_instances=%d\n\n", status, len(snapshots), newInstances)
	fmt.Fprintf(&b, "ADV aggregate (instances=%d):\n", advInstances)
	fmt.Fprintf(&b, "  state_get_or_init: calls=%d success=%d other_errors=%d\n", advTotal.StateGetOrInitCalls, advTotal.StateGetOrInitSuccess, advTotal.StateGetOrInitOtherErrors)
	fmt.Fprintf(&b, "  request_context: canceled=%d deadline=%d\n", advTotal.RequestContextCanceled, advTotal.RequestDeadlineExceeded)
	fmt.Fprintf(&b, "  background_retry: started=%d success=%d failed=%d deduplicated=%d pool_full=%d\n", advTotal.BackgroundRetryStarted, advTotal.BackgroundRetrySuccess, advTotal.BackgroundRetryFailed, advTotal.BackgroundRetryDeduplicated, advTotal.BackgroundRetryPoolFull)
	fmt.Fprintf(&b, "  telemetry_upload: failures=%d recovered=%d\n", advTotal.TelemetryPublishFailures, advTotal.TelemetryPublishRecovered)

	hosts := make([]string, 0, len(advByHost))
	for host := range advByHost {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	if len(hosts) > 0 {
		fmt.Fprintf(&b, "  by_host:\n")
		for _, host := range hosts {
			h := advByHost[host]
			fmt.Fprintf(&b, "    %s: canceled=%d deadline=%d retry_failed=%d upload_fail=%d\n", host, h.RequestContextCanceled, h.RequestDeadlineExceeded, h.BackgroundRetryFailed, h.TelemetryPublishFailures)
		}
	}

	fmt.Fprintf(&b, "\nPERCENTER (instances=%d):\n", percenterInstances)
	fmt.Fprintf(&b, "  missing_state: fallback_redis_nil=%d state_redis_nil=%d\n", percenterTotal.FallbackRedisNil, percenterTotal.StateRedisNil)
	fmt.Fprintf(&b, "  real_state_errors: fallback_load=%d parent_init=%d route_save=%d state_load=%d state_save=%d\n", percenterTotal.FallbackStateLoadErrors, percenterTotal.FallbackParentInitErrors, percenterTotal.FallbackRouteSaveErrors, percenterTotal.StateLoadErrors, percenterTotal.StateSaveErrors)
	fmt.Fprintf(&b, "  history_errors: dirty_notify=%d pending=%d\n", percenterTotal.HistoryDirtyNotifyErrors, percenterTotal.HistoryPendingErrors)
	fmt.Fprintf(&b, "  tick_errors=%d telemetry_upload_failures=%d recovered=%d", percenterTotal.TickErrors, percenterTotal.TelemetryPublishFailures, percenterTotal.TelemetryPublishRecovered)

	message := b.String()
	if err := bot.SendTextMessageToBot(ctx, message); err != nil {
		log.Printf("[PERCENTER][30M_HEALTH_TELEGRAM_ERROR] %v", err)
		return
	}
	if err := percenter.SaveTelemetryDigestState(ctx, client, snapshots, now); err != nil {
		// The message was delivered, but keeping the old checkpoint is safer than
		// silently losing counters. The next digest can repeat them and this log
		// makes that duplication diagnosable.
		log.Printf("[PERCENTER][30M_HEALTH_CHECKPOINT_ERROR] %v", err)
		return
	}
	log.Printf("[PERCENTER][30M_HEALTH_SENT] status=%s snapshots=%d new_instances=%d", status, len(snapshots), newInstances)
}

func telemetryDigestStatus(adv, worker percenter.TelemetryTotals) string {
	degraded := adv.StateGetOrInitOtherErrors+adv.BackgroundRetryFailed+adv.BackgroundRetryPoolFull+adv.TelemetryPublishFailures+
		worker.FallbackStateLoadErrors+worker.FallbackParentInitErrors+worker.FallbackRouteSaveErrors+worker.StateLoadErrors+worker.StateSaveErrors+
		worker.HistoryDirtyNotifyErrors+worker.HistoryPendingErrors+worker.TickErrors+worker.TelemetryPublishFailures > 0
	if degraded {
		return "DEGRADED"
	}
	warn := adv.RequestContextCanceled+adv.RequestDeadlineExceeded > 0
	if warn {
		return "WARN"
	}
	return "HEALTHY"
}
