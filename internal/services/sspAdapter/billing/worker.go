package billing

import (
	"context"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type Worker struct {
	store    *Store
	outbox   *outbox.Store
	interval time.Duration
}

func NewWorker(store *Store, out *outbox.Store, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = time.Second
	}
	return &Worker{store: store, outbox: out, interval: interval}
}
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		w.Once(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
func (w *Worker) Once(ctx context.Context) {
	if w == nil || w.store == nil || w.outbox == nil {
		return
	}
	records, err := w.outbox.List()
	if err != nil {
		return
	}
	for _, r := range records {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, err := w.store.Apply(ctx, Event{EventID: r.EventID, UserID: r.UserID, CampaignID: r.CampaignID, Format: r.Format, Price: r.Price})
		if err == nil {
			_ = w.outbox.Delete(r.EventID)
		} else {
			_ = w.outbox.UpdateFailure(r.EventID, err.Error())
		}
	}
}
