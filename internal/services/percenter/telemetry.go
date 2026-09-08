package percenter

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TelemetryPublishInterval   = time.Minute
	TelemetryDigestInterval    = 30 * time.Minute
	TelemetrySnapshotTTL       = 7 * 24 * time.Hour
	TelemetryFailureAlertAfter = uint64(5)

	telemetrySnapshotPrefix   = "percenter:telemetry:snapshot:"
	telemetrySnapshotIndexKey = "percenter:telemetry:snapshots"
	telemetryDigestStateKey   = "percenter:telemetry:digest:state"
	telemetryDigestLastAt     = "__last_digest_at"
)

// TelemetryTotals contains cumulative counters since one service process
// instance started. Fields not owned by a service stay zero. Cumulative values
// make a temporary Redis outage harmless: the next successful snapshot still
// contains everything counted while Redis was unavailable.
type TelemetryTotals struct {
	StateGetOrInitCalls       uint64 `json:"state_get_or_init_calls"`
	StateGetOrInitSuccess     uint64 `json:"state_get_or_init_success"`
	RequestContextCanceled    uint64 `json:"request_context_canceled"`
	RequestDeadlineExceeded   uint64 `json:"request_deadline_exceeded"`
	StateGetOrInitOtherErrors uint64 `json:"state_get_or_init_other_errors"`

	BackgroundRetryStarted      uint64 `json:"background_retry_started"`
	BackgroundRetrySuccess      uint64 `json:"background_retry_success"`
	BackgroundRetryFailed       uint64 `json:"background_retry_failed"`
	BackgroundRetryDeduplicated uint64 `json:"background_retry_deduplicated"`
	BackgroundRetryPoolFull     uint64 `json:"background_retry_pool_full"`

	FallbackRedisNil         uint64 `json:"fallback_redis_nil"`
	FallbackStateLoadErrors  uint64 `json:"fallback_state_load_errors"`
	FallbackParentInitErrors uint64 `json:"fallback_parent_init_errors"`
	FallbackRouteSaveErrors  uint64 `json:"fallback_route_save_errors"`
	StateRedisNil            uint64 `json:"state_redis_nil"`
	StateLoadErrors          uint64 `json:"state_load_errors"`
	StateSaveErrors          uint64 `json:"state_save_errors"`
	HistoryDirtyNotifyErrors uint64 `json:"history_dirty_notify_errors"`
	HistoryPendingErrors     uint64 `json:"history_pending_errors"`
	TickErrors               uint64 `json:"tick_errors"`

	TelemetryPublishFailures  uint64 `json:"telemetry_publish_failures"`
	TelemetryPublishRecovered uint64 `json:"telemetry_publish_recovered"`
}

func (t TelemetryTotals) Add(other TelemetryTotals) TelemetryTotals {
	t.StateGetOrInitCalls += other.StateGetOrInitCalls
	t.StateGetOrInitSuccess += other.StateGetOrInitSuccess
	t.RequestContextCanceled += other.RequestContextCanceled
	t.RequestDeadlineExceeded += other.RequestDeadlineExceeded
	t.StateGetOrInitOtherErrors += other.StateGetOrInitOtherErrors
	t.BackgroundRetryStarted += other.BackgroundRetryStarted
	t.BackgroundRetrySuccess += other.BackgroundRetrySuccess
	t.BackgroundRetryFailed += other.BackgroundRetryFailed
	t.BackgroundRetryDeduplicated += other.BackgroundRetryDeduplicated
	t.BackgroundRetryPoolFull += other.BackgroundRetryPoolFull
	t.FallbackRedisNil += other.FallbackRedisNil
	t.FallbackStateLoadErrors += other.FallbackStateLoadErrors
	t.FallbackParentInitErrors += other.FallbackParentInitErrors
	t.FallbackRouteSaveErrors += other.FallbackRouteSaveErrors
	t.StateRedisNil += other.StateRedisNil
	t.StateLoadErrors += other.StateLoadErrors
	t.StateSaveErrors += other.StateSaveErrors
	t.HistoryDirtyNotifyErrors += other.HistoryDirtyNotifyErrors
	t.HistoryPendingErrors += other.HistoryPendingErrors
	t.TickErrors += other.TickErrors
	t.TelemetryPublishFailures += other.TelemetryPublishFailures
	t.TelemetryPublishRecovered += other.TelemetryPublishRecovered
	return t
}

func (t TelemetryTotals) Delta(previous TelemetryTotals) TelemetryTotals {
	return TelemetryTotals{
		StateGetOrInitCalls:         telemetryDelta(t.StateGetOrInitCalls, previous.StateGetOrInitCalls),
		StateGetOrInitSuccess:       telemetryDelta(t.StateGetOrInitSuccess, previous.StateGetOrInitSuccess),
		RequestContextCanceled:      telemetryDelta(t.RequestContextCanceled, previous.RequestContextCanceled),
		RequestDeadlineExceeded:     telemetryDelta(t.RequestDeadlineExceeded, previous.RequestDeadlineExceeded),
		StateGetOrInitOtherErrors:   telemetryDelta(t.StateGetOrInitOtherErrors, previous.StateGetOrInitOtherErrors),
		BackgroundRetryStarted:      telemetryDelta(t.BackgroundRetryStarted, previous.BackgroundRetryStarted),
		BackgroundRetrySuccess:      telemetryDelta(t.BackgroundRetrySuccess, previous.BackgroundRetrySuccess),
		BackgroundRetryFailed:       telemetryDelta(t.BackgroundRetryFailed, previous.BackgroundRetryFailed),
		BackgroundRetryDeduplicated: telemetryDelta(t.BackgroundRetryDeduplicated, previous.BackgroundRetryDeduplicated),
		BackgroundRetryPoolFull:     telemetryDelta(t.BackgroundRetryPoolFull, previous.BackgroundRetryPoolFull),
		FallbackRedisNil:            telemetryDelta(t.FallbackRedisNil, previous.FallbackRedisNil),
		FallbackStateLoadErrors:     telemetryDelta(t.FallbackStateLoadErrors, previous.FallbackStateLoadErrors),
		FallbackParentInitErrors:    telemetryDelta(t.FallbackParentInitErrors, previous.FallbackParentInitErrors),
		FallbackRouteSaveErrors:     telemetryDelta(t.FallbackRouteSaveErrors, previous.FallbackRouteSaveErrors),
		StateLoadErrors:             telemetryDelta(t.StateLoadErrors, previous.StateLoadErrors),
		StateSaveErrors:             telemetryDelta(t.StateSaveErrors, previous.StateSaveErrors),
		HistoryDirtyNotifyErrors:    telemetryDelta(t.HistoryDirtyNotifyErrors, previous.HistoryDirtyNotifyErrors),
		HistoryPendingErrors:        telemetryDelta(t.HistoryPendingErrors, previous.HistoryPendingErrors),
		TickErrors:                  telemetryDelta(t.TickErrors, previous.TickErrors),
		TelemetryPublishFailures:    telemetryDelta(t.TelemetryPublishFailures, previous.TelemetryPublishFailures),
		TelemetryPublishRecovered:   telemetryDelta(t.TelemetryPublishRecovered, previous.TelemetryPublishRecovered),
	}
}

func telemetryDelta(current, previous uint64) uint64 {
	if current < previous {
		return current
	}
	return current - previous
}

type TelemetrySnapshot struct {
	Version    int             `json:"version"`
	Source     string          `json:"source"`
	Hostname   string          `json:"hostname"`
	InstanceID string          `json:"instance_id"`
	StartedAt  time.Time       `json:"started_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Totals     TelemetryTotals `json:"totals"`
}

func (s TelemetrySnapshot) Key() string {
	return telemetrySnapshotPrefix + telemetryKeyPart(s.Source) + ":" + telemetryKeyPart(s.Hostname) + ":" + telemetryKeyPart(s.InstanceID)
}

func telemetryKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", " ", "_")
	return replacer.Replace(value)
}

// SaveTelemetrySnapshot stores one idempotent cumulative snapshot and updates a
// small sorted-set index. The index avoids SCAN across the large percenter DB7
// keyspace when the single percenter builds the 30-minute digest.
func SaveTelemetrySnapshot(ctx context.Context, client *redis.Client, snapshot TelemetrySnapshot) error {
	if client == nil {
		return fmt.Errorf("telemetry Redis client is nil")
	}
	if strings.TrimSpace(snapshot.Source) == "" || strings.TrimSpace(snapshot.InstanceID) == "" {
		return fmt.Errorf("telemetry source and instance_id are required")
	}
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal telemetry snapshot: %w", err)
	}
	key := snapshot.Key()
	cutoff := snapshot.UpdatedAt.Add(-TelemetrySnapshotTTL).Unix()
	_, err = client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, key, payload, TelemetrySnapshotTTL)
		pipe.ZAdd(ctx, telemetrySnapshotIndexKey, redis.Z{Score: float64(snapshot.UpdatedAt.Unix()), Member: key})
		pipe.ZRemRangeByScore(ctx, telemetrySnapshotIndexKey, "-inf", strconv.FormatInt(cutoff, 10))
		return nil
	})
	if err != nil {
		return fmt.Errorf("write telemetry snapshot: %w", err)
	}
	return nil
}

func LoadTelemetrySnapshots(ctx context.Context, client *redis.Client, now time.Time) ([]TelemetrySnapshot, error) {
	if client == nil {
		return nil, fmt.Errorf("telemetry Redis client is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	keys, err := client.ZRangeByScore(ctx, telemetrySnapshotIndexKey, &redis.ZRangeBy{
		Min: strconv.FormatInt(now.Add(-TelemetrySnapshotTTL).Unix(), 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("load telemetry index: %w", err)
	}
	if len(keys) == 0 {
		return nil, nil
	}
	values, err := client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("load telemetry snapshots: %w", err)
	}
	out := make([]TelemetrySnapshot, 0, len(values))
	for _, raw := range values {
		if raw == nil {
			continue
		}
		var payload []byte
		switch value := raw.(type) {
		case string:
			payload = []byte(value)
		case []byte:
			payload = value
		default:
			continue
		}
		var snapshot TelemetrySnapshot
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			continue
		}
		if strings.TrimSpace(snapshot.InstanceID) == "" {
			continue
		}
		out = append(out, snapshot)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Hostname != out[j].Hostname {
			return out[i].Hostname < out[j].Hostname
		}
		return out[i].InstanceID < out[j].InstanceID
	})
	return out, nil
}

// RunTelemetryPublisher publishes cumulative process counters once per minute.
// A Redis failure is retried on the next minute tick. Only the fifth consecutive
// failed attempt raises an alert; further failures stay silent until recovery.
func RunTelemetryPublisher(
	ctx context.Context,
	client *redis.Client,
	snapshot func(time.Time) TelemetrySnapshot,
	onAttemptError func(uint64, error),
	onUnavailable func(uint64, error),
	onRecovered func(uint64),
	onPublished func(),
) {
	if client == nil || snapshot == nil {
		return
	}
	ticker := time.NewTicker(TelemetryPublishInterval)
	defer ticker.Stop()

	var consecutiveFailures uint64
	alerted := false
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			err := SaveTelemetrySnapshot(ctx, client, snapshot(now.UTC()))
			if err != nil {
				consecutiveFailures++
				if onAttemptError != nil {
					onAttemptError(consecutiveFailures, err)
				}
				if !alerted && consecutiveFailures >= TelemetryFailureAlertAfter {
					alerted = true
					if onUnavailable != nil {
						onUnavailable(consecutiveFailures, err)
					}
				}
				continue
			}

			if onPublished != nil {
				onPublished()
			}
			if consecutiveFailures > 0 && onRecovered != nil {
				onRecovered(consecutiveFailures)
			}
			consecutiveFailures = 0
			alerted = false
		}
	}
}

func LoadTelemetryDigestState(ctx context.Context, client *redis.Client) (map[string]TelemetryTotals, time.Time, error) {
	if client == nil {
		return nil, time.Time{}, fmt.Errorf("telemetry Redis client is nil")
	}
	values, err := client.HGetAll(ctx, telemetryDigestStateKey).Result()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("load telemetry digest state: %w", err)
	}
	checkpoints := make(map[string]TelemetryTotals, len(values))
	var lastAt time.Time
	for field, raw := range values {
		if field == telemetryDigestLastAt {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, raw); parseErr == nil {
				lastAt = parsed.UTC()
			}
			continue
		}
		var totals TelemetryTotals
		if err := json.Unmarshal([]byte(raw), &totals); err == nil {
			checkpoints[field] = totals
		}
	}
	return checkpoints, lastAt, nil
}

func SaveTelemetryDigestState(ctx context.Context, client *redis.Client, snapshots []TelemetrySnapshot, now time.Time) error {
	if client == nil {
		return fmt.Errorf("telemetry Redis client is nil")
	}
	values := make(map[string]any, len(snapshots)+1)
	values[telemetryDigestLastAt] = now.UTC().Format(time.RFC3339Nano)
	for _, snapshot := range snapshots {
		payload, err := json.Marshal(snapshot.Totals)
		if err != nil {
			return fmt.Errorf("marshal telemetry digest checkpoint: %w", err)
		}
		values[snapshot.Key()] = string(payload)
	}
	if err := client.HSet(ctx, telemetryDigestStateKey, values).Err(); err != nil {
		return fmt.Errorf("save telemetry digest state: %w", err)
	}
	if err := client.Expire(ctx, telemetryDigestStateKey, 30*24*time.Hour).Err(); err != nil {
		return fmt.Errorf("expire telemetry digest state: %w", err)
	}
	return nil
}
