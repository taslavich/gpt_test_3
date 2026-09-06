package main

import (
	"context"
	"fmt"
	"log"
	"time"

	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

const pendingHistoryRecoveryInterval = 10 * time.Minute

// runPendingHistoryFlusher keeps the hot path free of the shared history queue.
// State transitions persist history in a per-segment outbox + pending marker;
// this worker forwards it asynchronously to the existing Redis -> Kafka -> CH
// chain. Normal recovery never scans business-state keys.
func runPendingHistoryFlusher(
	ctx context.Context,
	store *percenter.StateStore,
	readyKey string,
	alert *services.RecoveryNotifier,
) {
	if store == nil || readyKey == "" {
		return
	}

	handleError := func(stage string, err error) {
		if err == nil {
			return
		}
		msg := fmt.Sprintf("[PERCENTER][HISTORY_PENDING_%s_ERROR] %v", stage, err)
		log.Print(msg)
		if alert != nil {
			alert.Failure(ctx, msg)
		}
	}
	handleRecovered := func() {
		if alert != nil {
			alert.Recovered(ctx, "[PERCENTER][HISTORY_PENDING_RECOVERED] pending history delivery is healthy")
		}
	}

	// Recover native outboxes first so rolling-upgrade migration cannot delay the
	// normal history path on a large existing keyspace. Legacy proj135 state
	// migration runs once in a separate goroutine and feeds the same dirty-hint
	// path as new state transitions.
	if flushed, err := store.RecoverPendingHistory(ctx, readyKey); err != nil {
		handleError("RECOVERY", err)
	} else {
		if flushed > 0 {
			log.Printf("[PERCENTER][HISTORY_PENDING_RECOVERY] flushed=%d", flushed)
		}
		handleRecovered()
	}

	go func() {
		migrated, err := store.MigrateLegacyPendingHistory(ctx, readyKey)
		if err != nil {
			handleError("LEGACY_MIGRATION", err)
			return
		}
		if migrated > 0 {
			log.Printf("[PERCENTER][HISTORY_LEGACY_MIGRATION] migrated_events=%d", migrated)
		}
	}()

	drainTicker := time.NewTicker(250 * time.Millisecond)
	defer drainTicker.Stop()
	recoveryTicker := time.NewTicker(pendingHistoryRecoveryInterval)
	defer recoveryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-drainTicker.C:
			flushed, err := store.DrainPendingHistoryHints(ctx, readyKey, 256)
			if err != nil {
				handleError("DRAIN", err)
				continue
			}
			if flushed > 0 {
				log.Printf("[PERCENTER][HISTORY_PENDING_DRAIN] flushed=%d", flushed)
			}
			handleRecovered()
		case <-recoveryTicker.C:
			flushed, err := store.RecoverPendingHistory(ctx, readyKey)
			if err != nil {
				handleError("RECOVERY", err)
				continue
			}
			if flushed > 0 {
				log.Printf("[PERCENTER][HISTORY_PENDING_RECOVERY] flushed=%d", flushed)
			}
			handleRecovered()
		}
	}
}
