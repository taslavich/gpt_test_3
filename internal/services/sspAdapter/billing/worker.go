package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type ADMRetryConfig struct {
	DSPWinnerRedis *redis.Client
	ClickRedis     []*redis.Client
	ClickSet       string
}

type Worker struct {
	store      *Store
	outbox     *outbox.Store
	interval   time.Duration
	maxBackoff time.Duration
	adm        ADMRetryConfig
}

func NewWorker(store *Store, outboxStore *outbox.Store, interval, maxBackoff time.Duration, adm ...ADMRetryConfig) *Worker {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if maxBackoff <= 0 {
		maxBackoff = time.Minute
	}
	worker := &Worker{store: store, outbox: outboxStore, interval: interval, maxBackoff: maxBackoff}
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
	records, err := w.outbox.List()
	if err != nil {
		log.Printf("ADV outbox list failed: %v", err)
		return
	}
	now := time.Now().UTC()
	for _, record := range records {
		last := record.LastAttemptAt
		if last.IsZero() {
			last = record.CreatedAt
		}
		backoff := retryBackoff(w.interval, w.maxBackoff, record.Attempts)
		if now.Before(last.Add(backoff)) {
			continue
		}
		if err := w.process(ctx, record); err != nil {
			if updateErr := w.outbox.UpdateFailure(record.EventID, err); updateErr != nil {
				log.Printf("ADV outbox failure update failed for %s: %v", record.EventID, updateErr)
			}
			continue
		}
		if err := w.outbox.Delete(record.EventID); err != nil {
			log.Printf("ADV outbox delete failed for %s: %v", record.EventID, err)
		}
	}
}

func (w *Worker) process(ctx context.Context, record outbox.Record) error {
	if outbox.NormalizeKind(record.Kind) == outbox.KindADM {
		return w.processADM(ctx, record)
	}
	return w.store.Apply(ctx, record)
}

func (w *Worker) processADM(ctx context.Context, record outbox.Record) error {
	if w.adm.DSPWinnerRedis == nil || len(w.adm.ClickRedis) == 0 {
		return errors.New("ADM retry dependencies are not initialized")
	}

	winnerType := outbox.NormalizeWinnerType(record.WinnerType)
	var advWinner Winner
	if winnerType == outbox.WinnerUnknown {
		exists, err := utils.UUIDKeyExistsInRedis(ctx, w.adm.DSPWinnerRedis, record.GlobalID)
		if err != nil {
			return fmt.Errorf("retry DSP winner lookup: %w", err)
		}
		if exists {
			winnerType = outbox.WinnerDSP
		} else {
			advWinner, err = w.store.ReadWinner(ctx, record.GlobalID, record.Format)
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return errors.New("ADM winner is not available in DSP DB1 or ADV DB6")
				}
				return fmt.Errorf("retry ADV winner lookup: %w", err)
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
			return fmt.Errorf("persist ADM winner resolution: %w", err)
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
				return fmt.Errorf("restore ADV winner details: %w", err)
			}
			record.UserID = advWinner.UserID
			record.CampaignID = advWinner.CampaignID
			record.Price = advWinner.Price
			if err := w.outbox.UpdateResolution(record.EventID, winnerType, record.UserID, record.CampaignID, record.Price); err != nil {
				return fmt.Errorf("persist ADV winner details: %w", err)
			}
		}
		if strings.EqualFold(record.Format, constants.IPP) {
			billingRecord := record
			billingRecord.Kind = outbox.KindBilling
			if err := w.store.Apply(ctx, billingRecord); err != nil {
				return err
			}
		}
	}

	if err := utils.WriteClickStats(ctx, w.adm.ClickRedis, record.ClickID, record.GlobalID, record.Format, true); err != nil {
		return err
	}
	if err := utils.AddUUIDToRedisSet(ctx, w.adm.ClickRedis, w.adm.ClickSet, record.ClickID, true); err != nil {
		return err
	}
	return nil
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
