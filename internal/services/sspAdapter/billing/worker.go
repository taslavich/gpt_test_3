package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type ADMRetryConfig struct {
	DSPWinnerRedis *redis.Client
	ClickRedis     []*redis.Client
	ClickSet       string
}

type Worker struct {
	store          *Store
	outbox         *outbox.Store
	interval       time.Duration
	maxBackoff     time.Duration
	advControlURLs []string
	adm            ADMRetryConfig
	lastSummary    time.Time
	// Test seams are nil in production; they preserve the real worker path.
	processOverride func(context.Context, outbox.Record) (bool, error)
	statusOverride  func(context.Context, []string, bool) map[string]int
}

func NewWorker(
	store *Store,
	outboxStore *outbox.Store,
	interval, maxBackoff time.Duration,
	advControlURLs []string,
	adm ...ADMRetryConfig,
) *Worker {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if maxBackoff <= 0 {
		maxBackoff = time.Minute
	}

	worker := &Worker{
		store:          store,
		outbox:         outboxStore,
		interval:       interval,
		maxBackoff:     maxBackoff,
		advControlURLs: append([]string(nil), advControlURLs...),
	}

	if len(adm) > 0 {
		worker.adm = adm[0]
	}

	return worker
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil || w.store == nil || w.outbox == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.run(ctx)
			}
		}
	}()
}

// ErrWinnerUnavailable means both winner lookups completed successfully but
// neither has the original auction record. A Redis outage is a different,
// retryable error. Missing winner metadata cannot be reconstructed from the
// click UUID once its TTL has expired.
var ErrWinnerUnavailable = errors.New("ADM winner metadata unavailable")

const missingWinnerRetryInterval = 30 * time.Minute
const outboxSummaryInterval = 30 * time.Second

func (w *Worker) setADVStatus(ctx context.Context, enabled bool) map[string]int {
	if w.statusOverride != nil {
		return w.statusOverride(ctx, w.advControlURLs, enabled)
	}
	return services.SetADVWorkStatus(ctx, w.advControlURLs, enabled)
}

func (w *Worker) replay(ctx context.Context, record outbox.Record) (bool, error) {
	if w.processOverride != nil {
		return w.processOverride(ctx, record)
	}
	return w.process(ctx, record)
}

func (w *Worker) disableADV(reason string) {
	disableCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statuses := w.setADVStatus(disableCtx, false)
	log.Printf("ADV disabled during billing recovery: reason=%s statuses=%v", reason, statuses)
}

func (w *Worker) run(ctx context.Context) {
	records, err := w.outbox.List()
	if err != nil {
		log.Printf("ADV outbox list failed: %v", err)
		return
	}

	// Persist the recovery latch BEFORE removing a single successful event.
	// Otherwise a crash after draining the last item but before re-enabling
	// ADV leaves the service disabled forever with an empty outbox.
	for _, record := range records {
		if isADVBillingRelevant(record) {
			pending, err := w.outbox.ADVRecoveryPending()
			if err != nil {
				log.Printf("CRITICAL: cannot read ADV recovery state: %v", err)
				return
			}
			if !pending {
				if err := w.outbox.SetADVRecoveryPending(true); err != nil {
					log.Printf("CRITICAL: cannot persist ADV outbox recovery state: %v", err)
					return
				}
			}
			break
		}
	}

	now := time.Now().UTC()
	attempted, delivered, failed, deferred := 0, 0, 0, 0
	errorCounts := make(map[string]int)
	failureSamples := make([]string, 0, 3)
	addFailureSample := func(eventID, reason string) {
		if len(failureSamples) >= 3 {
			return
		}
		if len(reason) > 180 {
			reason = reason[:180] + "..."
		}
		failureSamples = append(failureSamples, fmt.Sprintf("%s: %s", eventID, reason))
	}

	for _, record := range records {
		if ctx.Err() != nil {
			return
		}
		billingRelevant := isADVBillingRelevant(record)
		last := record.LastAttemptAt
		if last.IsZero() {
			last = record.CreatedAt
		}
		backoff := retryBackoff(w.interval, w.maxBackoff, record.Attempts)
		if record.ErrorClass == "winner_missing" && backoff < missingWinnerRetryInterval {
			backoff = missingWinnerRetryInterval
		}
		if now.Before(last.Add(backoff)) {
			deferred++
			if record.ErrorClass == "winner_missing" {
				errorCounts["winner_missing (manual investigation required)"]++
				addFailureSample(record.EventID, record.LastError)
			}
			continue
		}

		attempted++
		requiresRecovery, processErr := w.replay(ctx, record)
		if processErr != nil {
			failed++
			class := "redis_or_processing_error"
			if errors.Is(processErr, ErrWinnerUnavailable) {
				class = "winner_missing"
			}
			errorCounts[class]++
			addFailureSample(record.EventID, processErr.Error())

			if requiresRecovery && !billingRelevant {
				if err := w.outbox.SetADVRecoveryPending(true); err != nil {
					log.Printf("CRITICAL: cannot persist ADV recovery for event_id=%s: %v", record.EventID, err)
				}
				if err := w.outbox.SetADVRecoveryRequired(record.EventID, true); err != nil {
					log.Printf("CRITICAL: cannot mark outbox event for ADV recovery: event_id=%s error=%v", record.EventID, err)
				}
				billingRelevant = true
				w.disableADV("billing failure while retrying outbox event " + record.EventID)
			}
			if err := w.outbox.UpdateFailureWithClass(record.EventID, processErr, class); err != nil {
				log.Printf("ADV outbox failure update failed for %s: %v", record.EventID, err)
			}
			continue
		}

		// Redis billing is idempotent by event_id. A successful event is
		// acknowledged independently even if other records remain broken.
		if err := w.outbox.Delete(record.EventID); err != nil {
			failed++
			errorCounts["ack_failed"]++
			log.Printf("ADV outbox delete failed after successful replay: event_id=%s error=%v", record.EventID, err)
			continue
		}
		delivered++
	}

	// This second snapshot is authoritative for the re-enable gate, including
	// events written concurrently by HTTP handlers while the batch ran.
	latestRecords, err := w.outbox.List()
	if err != nil {
		log.Printf("ADV outbox post-replay inspection failed: %v", err)
		return
	}
	remainingBilling := 0
	for _, record := range latestRecords {
		if isADVBillingRelevant(record) {
			remainingBilling++
		}
	}
	if delivered > 0 || (len(latestRecords) > 0 && now.Sub(w.lastSummary) >= outboxSummaryInterval) {
		log.Printf("ADV outbox replay: attempted=%d delivered=%d failed=%d backoff=%d remaining_total=%d remaining_billing=%d error_classes=%v sample_failures=%v",
			attempted, delivered, failed, deferred, len(latestRecords), remainingBilling, errorCounts, failureSamples)
		w.lastSummary = now
	}
	if remainingBilling != 0 || ctx.Err() != nil {
		return
	}

	recoveryPending, err := w.outbox.ADVRecoveryPending()
	if err != nil {
		log.Printf("CRITICAL: cannot read ADV recovery state: %v", err)
		return
	}
	if !recoveryPending {
		return
	}

	enableCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	statuses := w.setADVStatus(enableCtx, true)
	cancel()
	if !allADVStatusesOK(statuses) {
		log.Printf("ADV re-enable failed; recovery state retained: statuses=%v", statuses)
		return
	}

	// The final verification and clearing of the durable recovery latch must
	// be one BoltDB transaction; ordinary callback writers use the same DB.
	drained, err := w.outbox.CompleteADVRecoveryIfDrained()
	if err != nil {
		log.Printf("ADV outbox atomic recovery recheck failed: %v", err)
		w.disableADV("outbox atomic recheck failed")
		return
	}
	if !drained {
		log.Printf("new ADV billing event appeared during recovery")
		w.disableADV("new billing event appeared")
		return
	}
	log.Printf("ADV billing outbox recovered; ADV enabled: statuses=%v", statuses)
}

func isADVBillingRelevant(record outbox.Record) bool {
	return outbox.NeedsADVRecovery(record)
}

func allADVStatusesOK(statuses map[string]int) bool {
	if len(statuses) == 0 {
		return false
	}

	for _, status := range statuses {
		if status != http.StatusOK {
			return false
		}
	}

	return true
}

func (w *Worker) process(ctx context.Context, record outbox.Record) (bool, error) {
	if outbox.NormalizeKind(record.Kind) == outbox.KindADM {
		return w.processADM(ctx, record)
	}

	err := w.store.Apply(ctx, record)
	return err != nil, err
}

func (w *Worker) processADM(ctx context.Context, record outbox.Record) (bool, error) {
	if w.adm.DSPWinnerRedis == nil || len(w.adm.ClickRedis) == 0 {
		return false, errors.New("ADM retry dependencies are not initialized")
	}

	winnerType := outbox.NormalizeWinnerType(record.WinnerType)
	var advWinner Winner
	if winnerType == outbox.WinnerUnknown {
		exists, err := utils.UUIDKeyExistsInRedis(ctx, w.adm.DSPWinnerRedis, record.GlobalID)
		if err != nil {
			return false, fmt.Errorf("retry DSP winner lookup: %w", err)
		}
		if exists {
			winnerType = outbox.WinnerDSP
		} else {
			advWinner, err = w.store.ReadWinner(ctx, record.GlobalID, record.Format)
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return false, fmt.Errorf("%w: DSP DB1 and ADV DB6, uuid=%s", ErrWinnerUnavailable, record.GlobalID)
				}
				return true, fmt.Errorf("retry ADV winner lookup: %w", err)
			}
			winnerType = outbox.WinnerADV
		}
		if err := w.outbox.UpdateResolution(
			record.EventID,
			winnerType,
			advWinner.UserID,
			advWinner.CampaignID,
			advWinner.Price,
			advWinner.TypeModel,
		); err != nil {
			return false, fmt.Errorf("persist ADM winner resolution: %w", err)
		}
		record.WinnerType = winnerType
		if winnerType == outbox.WinnerADV {
			record.UserID = advWinner.UserID
			record.CampaignID = advWinner.CampaignID
			record.TypeModel = advWinner.TypeModel
			record.Price = advWinner.Price
		}
	}

	if winnerType == outbox.WinnerADV {
		if record.UserID == "" || record.CampaignID == "" || record.Price <= 0 {
			var err error
			advWinner, err = w.store.ReadWinner(ctx, record.GlobalID, record.Format)
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return false, fmt.Errorf("%w: previously ADV, uuid=%s", ErrWinnerUnavailable, record.GlobalID)
				}
				return true, fmt.Errorf("restore ADV winner details: %w", err)
			}
			record.UserID = advWinner.UserID
			record.CampaignID = advWinner.CampaignID
			record.TypeModel = advWinner.TypeModel
			record.Price = advWinner.Price
			if err := w.outbox.UpdateResolution(record.EventID, winnerType, record.UserID, record.CampaignID, record.Price, record.TypeModel); err != nil {
				return false, fmt.Errorf("persist ADV winner details: %w", err)
			}
		}
	}

	if err := utils.WriteClickStats(ctx, w.adm.ClickRedis, record.ClickID, record.GlobalID, record.Format, true); err != nil {
		return false, err
	}
	if err := utils.AddUUIDToRedisSet(ctx, w.adm.ClickRedis, w.adm.ClickSet, record.ClickID, true); err != nil {
		return false, err
	}
	return false, nil
}

func retryBackoff(base, maximum time.Duration, attempts int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if maximum <= 0 || maximum < base {
		maximum = base
	}
	backoff := base
	for attempt := 1; attempt < attempts && backoff < maximum; attempt++ {
		if backoff > maximum/2 {
			return maximum
		}
		backoff *= 2
	}
	if backoff > maximum {
		return maximum
	}
	return backoff
}
