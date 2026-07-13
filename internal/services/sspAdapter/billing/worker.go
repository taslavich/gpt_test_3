package billing

import (
	"context"
	"log"
	"math"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type Worker struct {
	store      *Store
	outbox     *outbox.Store
	interval   time.Duration
	maxBackoff time.Duration
}

func NewWorker(store *Store, outboxStore *outbox.Store, interval, maxBackoff time.Duration) *Worker {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if maxBackoff <= 0 {
		maxBackoff = time.Minute
	}
	return &Worker{store: store, outbox: outboxStore, interval: interval, maxBackoff: maxBackoff}
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
		backoff := w.interval * time.Duration(math.Pow(2, float64(maxInt(record.Attempts-1, 0))))
		if backoff > w.maxBackoff {
			backoff = w.maxBackoff
		}
		if now.Before(last.Add(backoff)) {
			continue
		}
		if err := w.store.Apply(ctx, record); err != nil {
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
