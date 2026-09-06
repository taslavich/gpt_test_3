package percenter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	historyDirtySuffix              = ":dirty_segments"
	historyOutboxPrefix             = "percenter:history:outbox:"
	historyPendingPrefix            = "percenter:history:pending:"
	historyLegacyMigrationSuffix    = ":legacy_pending_v2_migrated"
	historyNotifyBatchSize          = 256
	historyNotifyFlushInterval      = 100 * time.Millisecond
	historyDrainBatchSize           = 256
	historyOutboxFlushBatchSize     = 256
	historyRecoveryScanCount        = 1000
	historyStorageMaxRetries        = 8
	historyLegacyMigrationBatchSize = 500
	historyLegacyMigrationPause     = 25 * time.Millisecond
)

func HistoryDirtyKey(readyKey string) string {
	readyKey = strings.TrimSpace(readyKey)
	if readyKey == "" {
		return ""
	}
	return readyKey + historyDirtySuffix
}

func HistoryOutboxKey(segmentHash string) string {
	segmentHash = strings.TrimSpace(segmentHash)
	if segmentHash == "" {
		return ""
	}
	return historyOutboxPrefix + segmentHash
}

func HistoryPendingKey(segmentHash string) string {
	segmentHash = strings.TrimSpace(segmentHash)
	if segmentHash == "" {
		return ""
	}
	return historyPendingPrefix + segmentHash
}

func historyLegacyMigrationDoneKey(readyKey string) string {
	readyKey = strings.TrimSpace(readyKey)
	if readyKey == "" {
		return ""
	}
	return readyKey + historyLegacyMigrationSuffix
}

func historySegmentHashFromPendingKey(key string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(key), historyPendingPrefix))
}

func validatePerSegmentHistoryKeyTypes(ctx context.Context, tx *redis.Tx, stateKey, outboxKey, pendingKey string) (string, error) {
	stateType, err := tx.Type(ctx, stateKey).Result()
	if err != nil {
		return "", err
	}
	if stateType != "none" && stateType != "string" {
		return "", fmt.Errorf("percenter state key %s has unexpected type: %s", stateKey, stateType)
	}
	outboxType, err := tx.Type(ctx, outboxKey).Result()
	if err != nil {
		return "", err
	}
	if outboxType != "none" && outboxType != "list" {
		return "", fmt.Errorf("percenter history outbox key %s has unexpected type: %s", outboxKey, outboxType)
	}
	pendingType, err := tx.Type(ctx, pendingKey).Result()
	if err != nil {
		return "", err
	}
	if pendingType != "none" && pendingType != "string" {
		return "", fmt.Errorf("percenter history pending key %s has unexpected type: %s", pendingKey, pendingType)
	}
	return stateType, nil
}

func historyEventRawValues(events []HistoryEvent) ([]interface{}, error) {
	if len(events) == 0 {
		return nil, nil
	}
	values := make([]interface{}, 0, len(events))
	for _, event := range events {
		raw, err := MarshalHistoryEvent(event)
		if err != nil {
			return nil, fmt.Errorf("marshal percenter history event %s: %w", event.EventID, err)
		}
		values = append(values, string(raw))
	}
	return values, nil
}

// StartHistoryDirtyNotifier batches best-effort transport hints. Durable history
// is already stored in the per-segment outbox and has its own pending marker
// before this method is notified, so dropping a local hint cannot lose history.
// Recovery scans only percenter:history:pending:* markers, never all business
// segment states.
func (s *StateStore) StartHistoryDirtyNotifier(ctx context.Context, onError func(error), onRecovered func()) {
	if s == nil || s.redis == nil || s.historyNotifyCh == nil || s.historyDirtySetKey() == "" {
		return
	}
	s.historyNotifyOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(historyNotifyFlushInterval)
			defer ticker.Stop()
			pending := make(map[string]struct{}, historyNotifyBatchSize)

			flush := func() {
				if len(pending) == 0 {
					return
				}
				args := make([]interface{}, 0, len(pending))
				for hash := range pending {
					args = append(args, hash)
				}
				if err := s.redis.SAdd(ctx, s.historyDirtySetKey(), args...).Err(); err != nil {
					if onError != nil {
						onError(fmt.Errorf("mark pending percenter history segments: %w", err))
					}
					// Do not block or retry the auction path. The durable pending marker is
					// the recovery source of truth.
				} else if onRecovered != nil {
					onRecovered()
				}
				clear(pending)
			}

			for {
				select {
				case <-ctx.Done():
					flush()
					return
				case hash := <-s.historyNotifyCh:
					hash = strings.TrimSpace(hash)
					if hash != "" {
						pending[hash] = struct{}{}
					}
					if len(pending) >= historyNotifyBatchSize {
						flush()
					}
				case <-ticker.C:
					flush()
				}
			}
		}()
	})
}

// DrainPendingHistoryHints pops best-effort dirty hashes and flushes their
// durable per-segment outboxes. Failed hashes are put back into the dirty set;
// even if that requeue fails, their persistent pending marker remains recoverable.
func (s *StateStore) DrainPendingHistoryHints(ctx context.Context, readyKey string, limit int64) (int, error) {
	if s == nil || s.redis == nil {
		return 0, errors.New("percenter redis is unavailable")
	}
	dirtyKey := HistoryDirtyKey(readyKey)
	if dirtyKey == "" {
		return 0, errors.New("percenter history ready key is empty")
	}
	if limit <= 0 {
		limit = historyDrainBatchSize
	}
	hashes, err := s.redis.SPopN(ctx, dirtyKey, limit).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("pop percenter history dirty segments: %w", err)
	}

	flushed := 0
	for _, hash := range hashes {
		count, flushErr := s.FlushPendingHistoryForSegment(ctx, hash, readyKey)
		if flushErr != nil {
			_ = s.redis.SAdd(context.WithoutCancel(ctx), dirtyKey, hash).Err()
			return flushed, flushErr
		}
		flushed += count
	}
	return flushed, nil
}

// FlushPendingHistoryForSegment is at-least-once by design:
//  1. read a bounded prefix from the per-segment outbox;
//  2. enqueue that prefix to the shared ready LIST;
//  3. WATCH/CAS-remove only the same prefix from the outbox.
//
// A crash after step 2 replays events rather than losing them. A concurrent
// state transition only appends to the tail, and a concurrent flusher cannot
// remove a different prefix because the outbox key is watched during ACK.
func (s *StateStore) FlushPendingHistoryForSegment(ctx context.Context, segmentHash, readyKey string) (int, error) {
	if s == nil || s.redis == nil {
		return 0, errors.New("percenter redis is unavailable")
	}
	segmentHash = strings.TrimSpace(segmentHash)
	readyKey = strings.TrimSpace(readyKey)
	if segmentHash == "" || readyKey == "" {
		return 0, nil
	}

	// Compatibility with proj135 records that still contain pending_history in
	// the business-state JSON. New writes never put history there.
	if _, err := s.migrateLegacyPendingHistoryForSegment(ctx, segmentHash); err != nil {
		return 0, err
	}

	outboxKey := HistoryOutboxKey(segmentHash)
	pendingKey := HistoryPendingKey(segmentHash)
	raws, err := s.redis.LRange(ctx, outboxKey, 0, historyOutboxFlushBatchSize-1).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, fmt.Errorf("read percenter history outbox segment=%s: %w", segmentHash, err)
	}
	if len(raws) == 0 {
		if err := s.clearHistoryPendingMarkerIfEmpty(ctx, outboxKey, pendingKey); err != nil {
			return 0, err
		}
		return 0, nil
	}

	args := make([]interface{}, 0, len(raws))
	for _, raw := range raws {
		if _, err := UnmarshalHistoryEvent([]byte(raw)); err != nil {
			return 0, fmt.Errorf("invalid event in percenter history outbox segment=%s: %w", segmentHash, err)
		}
		args = append(args, raw)
	}

	// LPUSH producer + RPOP consumer preserves FIFO order for this prefix.
	if err := s.redis.LPush(ctx, readyKey, args...).Err(); err != nil {
		return 0, fmt.Errorf("enqueue percenter history segment=%s: %w", segmentHash, err)
	}

	acked, remaining, err := s.ackHistoryOutboxPrefix(ctx, outboxKey, pendingKey, raws)
	if err != nil {
		s.notifyHistoryPending(segmentHash)
		return len(raws), err
	}
	if !acked || remaining > 0 {
		s.notifyHistoryPending(segmentHash)
	}
	return len(raws), nil
}

func (s *StateStore) ackHistoryOutboxPrefix(ctx context.Context, outboxKey, pendingKey string, delivered []string) (bool, int64, error) {
	for attempt := 0; attempt < historyStorageMaxRetries; attempt++ {
		acked := false
		remaining := int64(0)
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			outboxType, err := tx.Type(ctx, outboxKey).Result()
			if err != nil {
				return err
			}
			if outboxType != "none" && outboxType != "list" {
				return fmt.Errorf("percenter history outbox key %s has unexpected type: %s", outboxKey, outboxType)
			}
			pendingType, err := tx.Type(ctx, pendingKey).Result()
			if err != nil {
				return err
			}
			if pendingType != "none" && pendingType != "string" {
				return fmt.Errorf("percenter history pending key %s has unexpected type: %s", pendingKey, pendingType)
			}

			prefix, err := tx.LRange(ctx, outboxKey, 0, int64(len(delivered))-1).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			if len(prefix) != len(delivered) {
				return nil
			}
			for i := range prefix {
				if prefix[i] != delivered[i] {
					return nil
				}
			}
			length, err := tx.LLen(ctx, outboxKey).Result()
			if err != nil {
				return err
			}
			remaining = length - int64(len(delivered))
			if remaining < 0 {
				remaining = 0
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if remaining == 0 {
					pipe.Del(ctx, outboxKey)
					pipe.Del(ctx, pendingKey)
				} else {
					pipe.LTrim(ctx, outboxKey, int64(len(delivered)), -1)
					pipe.Set(ctx, pendingKey, "1", 0)
				}
				return nil
			})
			if err == nil {
				acked = true
			}
			return err
		}, outboxKey, pendingKey)
		if err == nil {
			return acked, remaining, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return false, 0, fmt.Errorf("ack percenter history outbox %s: %w", outboxKey, err)
		}
	}
	return false, 0, fmt.Errorf("ack percenter history outbox %s: Redis transaction conflicted after %d retries", outboxKey, historyStorageMaxRetries)
}

func (s *StateStore) clearHistoryPendingMarkerIfEmpty(ctx context.Context, outboxKey, pendingKey string) error {
	for attempt := 0; attempt < historyStorageMaxRetries; attempt++ {
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			length, err := tx.LLen(ctx, outboxKey).Result()
			if err != nil {
				return err
			}
			if length != 0 {
				return nil
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Del(ctx, outboxKey)
				pipe.Del(ctx, pendingKey)
				return nil
			})
			return err
		}, outboxKey, pendingKey)
		if err == nil {
			return nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return fmt.Errorf("clear stale percenter history marker %s: %w", pendingKey, err)
		}
	}
	return fmt.Errorf("clear stale percenter history marker %s: Redis transaction conflicted after %d retries", pendingKey, historyStorageMaxRetries)
}

// RecoverPendingHistory scans only durable pending-marker keys. It never reads
// every percenter:segment:* business state, so recovery cost scales with the
// number of segments that actually have undelivered history.
func (s *StateStore) RecoverPendingHistory(ctx context.Context, readyKey string) (int, error) {
	if s == nil || s.redis == nil {
		return 0, errors.New("percenter redis is unavailable")
	}
	var cursor uint64
	flushed := 0
	for {
		keys, next, err := s.redis.Scan(ctx, cursor, historyPendingPrefix+"*", historyRecoveryScanCount).Result()
		if err != nil {
			return flushed, fmt.Errorf("scan percenter pending-history markers: %w", err)
		}
		for _, key := range keys {
			hash := historySegmentHashFromPendingKey(key)
			if hash == "" {
				continue
			}
			count, err := s.FlushPendingHistoryForSegment(ctx, hash, readyKey)
			if err != nil {
				return flushed, err
			}
			flushed += count
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return flushed, nil
}

// MigrateLegacyPendingHistory performs a one-time, throttled compatibility scan
// for proj135 states that stored pending_history inside the business-state JSON.
// It is not part of normal recovery and is skipped after a successful migration.
func (s *StateStore) MigrateLegacyPendingHistory(ctx context.Context, readyKey string) (int, error) {
	if s == nil || s.redis == nil {
		return 0, errors.New("percenter redis is unavailable")
	}
	doneKey := historyLegacyMigrationDoneKey(readyKey)
	if doneKey == "" {
		return 0, errors.New("percenter history ready key is empty")
	}
	done, err := s.redis.Exists(ctx, doneKey).Result()
	if err != nil {
		return 0, fmt.Errorf("check percenter history legacy migration marker: %w", err)
	}
	if done > 0 {
		return 0, nil
	}

	var cursor uint64
	migrated := 0
	for {
		keys, next, err := s.redis.Scan(ctx, cursor, "percenter:segment:*", historyLegacyMigrationBatchSize).Result()
		if err != nil {
			return migrated, fmt.Errorf("scan legacy percenter states: %w", err)
		}
		if len(keys) > 0 {
			values, err := s.redis.MGet(ctx, keys...).Result()
			if err != nil {
				return migrated, fmt.Errorf("read legacy percenter states: %w", err)
			}
			for i, value := range values {
				if value == nil {
					continue
				}
				raw, ok := value.(string)
				if !ok || !strings.Contains(raw, `"pending_history"`) {
					continue
				}
				var state State
				if err := json.Unmarshal([]byte(raw), &state); err != nil {
					return migrated, fmt.Errorf("decode legacy percenter state %s: %w", keys[i], err)
				}
				if len(state.PendingHistory) == 0 {
					continue
				}
				moved, err := s.migrateLegacyPendingHistoryForSegment(ctx, state.SegmentHash)
				if err != nil {
					return migrated, err
				}
				if moved > 0 {
					migrated += moved
				}
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return migrated, ctx.Err()
		case <-time.After(historyLegacyMigrationPause):
		}
	}
	if err := s.redis.Set(ctx, doneKey, "1", 0).Err(); err != nil {
		return migrated, fmt.Errorf("mark percenter history legacy migration complete: %w", err)
	}
	return migrated, nil
}

func (s *StateStore) migrateLegacyPendingHistoryForSegment(ctx context.Context, segmentHash string) (int, error) {
	segmentHash = strings.TrimSpace(segmentHash)
	if segmentHash == "" {
		return 0, nil
	}
	stateKey := SegmentKey(segmentHash)
	outboxKey := HistoryOutboxKey(segmentHash)
	pendingKey := HistoryPendingKey(segmentHash)

	for attempt := 0; attempt < historyStorageMaxRetries; attempt++ {
		moved := 0
		var migratedState State
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			stateType, err := validatePerSegmentHistoryKeyTypes(ctx, tx, stateKey, outboxKey, pendingKey)
			if err != nil {
				return err
			}
			if stateType == "none" {
				return nil
			}
			raw, err := tx.Get(ctx, stateKey).Bytes()
			if errors.Is(err, redis.Nil) {
				return nil
			}
			if err != nil {
				return err
			}
			var current State
			if err := json.Unmarshal(raw, &current); err != nil {
				return fmt.Errorf("decode legacy percenter state %s: %w", segmentHash, err)
			}
			if len(current.PendingHistory) == 0 {
				return nil
			}
			values, err := historyEventRawValues(current.PendingHistory)
			if err != nil {
				return err
			}
			moved = len(values)
			current.PendingHistory = nil
			currentRaw, err := json.Marshal(current)
			if err != nil {
				return err
			}
			ttl, err := tx.PTTL(ctx, stateKey).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			if ttl <= 0 {
				ttl = s.policy.SegmentStateTTL
				if ttl <= 0 {
					ttl = 7 * 24 * time.Hour
				}
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, stateKey, currentRaw, ttl)
				pipe.RPush(ctx, outboxKey, values...)
				pipe.Set(ctx, pendingKey, "1", 0)
				return nil
			})
			if err == nil {
				migratedState = current
			}
			return err
		}, stateKey, outboxKey, pendingKey)
		if err == nil {
			if moved > 0 {
				s.putCache(migratedState, time.Now().UTC())
				s.notifyHistoryPending(segmentHash)
			}
			return moved, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return 0, fmt.Errorf("migrate legacy pending history segment=%s: %w", segmentHash, err)
		}
	}
	return 0, fmt.Errorf("migrate legacy pending history segment=%s: Redis transaction conflicted after %d retries", segmentHash, historyStorageMaxRetries)
}
