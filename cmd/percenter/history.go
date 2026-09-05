package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

type historyRecorder struct {
	outbox *percenter.HistoryOutbox
	alert  *services.RecoveryNotifier
}

func (r *historyRecorder) Record(ctx context.Context, event percenter.HistoryEvent) {
	if r == nil || r.outbox == nil {
		return
	}
	if err := r.outbox.Save(event); err != nil {
		msg := fmt.Sprintf("[PERCENTER][HISTORY_OUTBOX_SAVE_ERROR] event_id=%s type=%s state_hash=%s error=%v", event.EventID, event.EventType, event.StateSegmentHash, err)
		log.Print(msg)
		if r.alert != nil {
			r.alert.Failure(ctx, msg)
		}
	}
}

func runHistoryOutboxFlusher(ctx context.Context, recorder *historyRecorder, redisClient *redis.Client, readyKey string) {
	if recorder == nil || recorder.outbox == nil || redisClient == nil || readyKey == "" {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	flush := func() {
		events, err := recorder.outbox.List(500)
		if err != nil {
			msg := fmt.Sprintf("[PERCENTER][HISTORY_OUTBOX_READ_ERROR] %v", err)
			log.Print(msg)
			if recorder.alert != nil {
				recorder.alert.Failure(ctx, msg)
			}
			return
		}
		if len(events) == 0 {
			return
		}
		pipe := redisClient.Pipeline()
		raws := make([][]byte, 0, len(events))
		for _, event := range events {
			raw, err := percenter.MarshalHistoryEvent(event)
			if err != nil {
				msg := fmt.Sprintf("[PERCENTER][HISTORY_OUTBOX_ENCODE_ERROR] event_id=%s error=%v", event.EventID, err)
				log.Print(msg)
				if recorder.alert != nil {
					recorder.alert.Failure(ctx, msg)
				}
				return
			}
			raws = append(raws, raw)
			pipe.RPush(ctx, readyKey, raw)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			msg := fmt.Sprintf("[PERCENTER][HISTORY_REDIS_QUEUE_ERROR] pending=%d error=%v", len(events), err)
			log.Print(msg)
			if recorder.alert != nil {
				recorder.alert.Failure(ctx, msg)
			}
			return
		}
		// Redis accepted the batch. Deletion is intentionally after enqueue.
		// If deletion fails, the event is replayed; event_id deduplicates it downstream.
		for _, event := range events {
			if err := recorder.outbox.Delete(event.EventID); err != nil {
				msg := fmt.Sprintf("[PERCENTER][HISTORY_OUTBOX_DELETE_ERROR] event_id=%s error=%v", event.EventID, err)
				log.Print(msg)
				if recorder.alert != nil {
					recorder.alert.Failure(ctx, msg)
				}
				return
			}
		}
		if recorder.alert != nil {
			recorder.alert.Recovered(ctx, "[PERCENTER][HISTORY_PIPELINE_RECOVERED] local outbox -> Redis queue is healthy")
		}
		log.Printf("[PERCENTER][HISTORY_OUTBOX_FLUSH] queued=%d", len(events))
	}
	flush()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flush()
		}
	}
}
