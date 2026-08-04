package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
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

func (w *Worker) run(ctx context.Context) {
	disableADV := func(reason string) {
		disableCtx, disableCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		statuses := services.SetADVWorkStatus(
			disableCtx,
			w.advControlURLs,
			false,
		)
		disableCancel()

		log.Printf(
			"ADV disabled after billing recovery race: reason=%s statuses=%v",
			reason,
			statuses,
		)
	}

	records, err := w.outbox.List()
	if err != nil {
		log.Printf("ADV outbox list failed: %v", err)
		return
	}

	now := time.Now().UTC()
	processedBillingIDs := make([]string, 0)
	billingStillPending := false

	for _, record := range records {
		billingRelevant := isADVBillingRelevant(record)

		last := record.LastAttemptAt
		if last.IsZero() {
			last = record.CreatedAt
		}

		backoff := retryBackoff(
			w.interval,
			w.maxBackoff,
			record.Attempts,
		)

		if now.Before(last.Add(backoff)) {
			if billingRelevant {
				billingStillPending = true
			}
			continue
		}

		requiresRecoveryOnFailure, processErr := w.process(ctx, record)
		if processErr != nil {
			if requiresRecoveryOnFailure && !billingRelevant {
				if markErr := w.outbox.SetADVRecoveryRequired(record.EventID, true); markErr != nil {
					log.Printf(
						"CRITICAL: cannot mark outbox event for ADV recovery: event_id=%s error=%v",
						record.EventID,
						markErr,
					)
				}
				billingRelevant = true
				disableADV("billing failure while retrying outbox event " + record.EventID)
			}

			if billingRelevant {
				billingStillPending = true
			}

			if updateErr := w.outbox.UpdateFailure(record.EventID, processErr); updateErr != nil {
				log.Printf(
					"ADV outbox failure update failed for %s: %v",
					record.EventID,
					updateErr,
				)
			}

			continue
		}

		if billingRelevant {
			// Событие применено в Redis, но пока не удаляем его
			// из outbox. Сначала безопасно включаем ADV.
			processedBillingIDs = append(
				processedBillingIDs,
				record.EventID,
			)
			continue
		}

		if err := w.outbox.Delete(record.EventID); err != nil {
			log.Printf(
				"ADV outbox delete failed for %s: %v",
				record.EventID,
				err,
			)
		}
	}

	// Пока есть хотя бы одно необработанное billing-событие,
	// ADV включать нельзя.
	if billingStillPending || len(processedBillingIDs) == 0 {
		return
	}

	enableCtx, enableCancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)
	statuses := services.SetADVWorkStatus(
		enableCtx,
		w.advControlURLs,
		true,
	)
	enableCancel()

	if !allADVStatusesOK(statuses) {
		log.Printf(
			"ADV re-enable failed; billing records remain in outbox: statuses=%v",
			statuses,
		)
		return
	}

	processedIDs := make(
		map[string]struct{},
		len(processedBillingIDs),
	)

	for _, eventID := range processedBillingIDs {
		processedIDs[eventID] = struct{}{}
	}

	// После включения ADV повторно проверяем outbox.
	// За время восстановления могло появиться новое billing-событие.
	latestRecords, err := w.outbox.List()
	if err != nil {
		log.Printf(
			"ADV outbox recheck failed after enabling ADV: %v",
			err,
		)
		disableADV("outbox recheck failed")
		return
	}

	for _, record := range latestRecords {
		if !isADVBillingRelevant(record) {
			continue
		}

		if _, alreadyProcessed := processedIDs[record.EventID]; alreadyProcessed {
			continue
		}

		log.Printf(
			"new ADV billing event appeared during recovery: event_id=%s",
			record.EventID,
		)

		disableADV("new billing event appeared")
		return
	}

	// Новых billing-событий нет.
	// Теперь удаляем успешно применённые записи.
	for _, eventID := range processedBillingIDs {
		if err := w.outbox.Delete(eventID); err != nil {
			log.Printf(
				"ADV outbox delete failed for %s: %v",
				eventID,
				err,
			)
			disableADV("cannot delete recovered billing event " + eventID)
			return
		}
	}

	log.Printf(
		"ADV billing outbox recovered; ADV enabled: statuses=%v",
		statuses,
	)
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
					return false, errors.New("ADM winner is not available in DSP DB1 or ADV DB6")
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
		); err != nil {
			return false, fmt.Errorf("persist ADM winner resolution: %w", err)
		}
		record.WinnerType = winnerType
		if winnerType == outbox.WinnerADV {
			record.UserID = advWinner.UserID
			record.CampaignID = advWinner.CampaignID
			record.Price = advWinner.Price
		}
	}

	if winnerType == outbox.WinnerADV {
		if record.UserID == "" || record.CampaignID == "" || record.Price <= 0 {
			var err error
			advWinner, err = w.store.ReadWinner(ctx, record.GlobalID, record.Format)
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return false, fmt.Errorf("restore ADV winner details: %w", err)
				}
				return true, fmt.Errorf("restore ADV winner details: %w", err)
			}
			record.UserID = advWinner.UserID
			record.CampaignID = advWinner.CampaignID
			record.Price = advWinner.Price
			if err := w.outbox.UpdateResolution(record.EventID, winnerType, record.UserID, record.CampaignID, record.Price); err != nil {
				return false, fmt.Errorf("persist ADV winner details: %w", err)
			}
		}
		if strings.EqualFold(record.Format, constants.IPP) {
			billingRecord := record
			billingRecord.Kind = outbox.KindBilling
			if err := w.store.Apply(ctx, billingRecord); err != nil {
				return true, err
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
