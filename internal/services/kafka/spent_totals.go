package kafka_service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

const defaultSpentTotalsScanCount int64 = 1000

var ErrInvalidSpentTotal = errors.New("invalid spent total")

type SpentTotalMessage struct {
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	SpentTotal float64 `json:"spent_total"`
	SnapshotTS string  `json:"snapshot_ts"`
}

func (m SpentTotalMessage) Validate() error {
	if m.EntityType != "user" && m.EntityType != "campaign" {
		return fmt.Errorf("%w: unsupported entity_type %q", ErrInvalidSpentTotal, m.EntityType)
	}
	if strings.TrimSpace(m.EntityID) == "" {
		return fmt.Errorf("%w: entity_id is empty", ErrInvalidSpentTotal)
	}
	if math.IsNaN(m.SpentTotal) || math.IsInf(m.SpentTotal, 0) || m.SpentTotal < 0 {
		return fmt.Errorf("%w: spent_total must be finite and nonnegative", ErrInvalidSpentTotal)
	}
	if strings.TrimSpace(m.SnapshotTS) == "" {
		return fmt.Errorf("%w: snapshot_ts is empty", ErrInvalidSpentTotal)
	}
	if _, err := time.Parse(time.RFC3339, m.SnapshotTS); err != nil {
		return fmt.Errorf("%w: invalid snapshot_ts: %v", ErrInvalidSpentTotal, err)
	}
	return nil
}

func SpentTotalKey(entityType, entityID string) []byte { return []byte(entityType + ":" + entityID) }

type CorruptSpentValueHook func(key, raw string, err error)

func ScanSpentTotals(ctx context.Context, rdb *redis.Client, scanCount int64, publish func(context.Context, SpentTotalMessage) error) error {
	return ScanSpentTotalsWithCorruptHook(ctx, rdb, scanCount, publish, nil)
}

func ScanSpentTotalsWithCorruptHook(ctx context.Context, rdb *redis.Client, scanCount int64, publish func(context.Context, SpentTotalMessage) error, onCorrupt CorruptSpentValueHook) error {
	if rdb == nil {
		return errors.New("redis client is nil")
	}
	if publish == nil {
		return errors.New("publish callback is nil")
	}
	if scanCount <= 0 {
		scanCount = defaultSpentTotalsScanCount
	}
	for _, pattern := range []string{"spent:user:*", "spent:campaign:*"} {
		var cursor uint64
		for {
			keys, next, err := rdb.Scan(ctx, cursor, pattern, scanCount).Result()
			if err != nil {
				return err
			}
			cursor = next
			for _, key := range keys {
				raw, err := rdb.Get(ctx, key).Result()
				if errors.Is(err, redis.Nil) {
					continue
				}
				if err != nil {
					return fmt.Errorf("spent total get failed key=%s: %w", key, err)
				}
				val, err := strconv.ParseFloat(raw, 64)
				if err != nil || math.IsNaN(val) || math.IsInf(val, 0) || val < 0 {
					if err == nil {
						err = fmt.Errorf("value must be finite and nonnegative")
					}
					log.Printf("⚠️ invalid spent total key=%s value=%q: %v", key, raw, err)
					if onCorrupt != nil {
						onCorrupt(key, raw, err)
					}
					continue
				}
				parts := strings.SplitN(key, ":", 3)
				if len(parts) != 3 {
					continue
				}
				typ := parts[1]
				if typ != "user" && typ != "campaign" {
					continue
				}
				msg := SpentTotalMessage{EntityType: typ, EntityID: parts[2], SpentTotal: val, SnapshotTS: time.Now().UTC().Format(time.RFC3339)}
				if err := msg.Validate(); err != nil {
					if onCorrupt != nil {
						onCorrupt(key, raw, err)
					}
					continue
				}
				if err := publish(ctx, msg); err != nil {
					return err
				}
			}
			if cursor == 0 {
				break
			}
		}
	}
	return nil
}

func PublishSpentTotal(ctx context.Context, writer *kafka.Writer, msg SpentTotalMessage) error {
	if writer == nil {
		return errors.New("kafka writer is nil")
	}
	if err := msg.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return writer.WriteMessages(ctx, kafka.Message{Key: SpentTotalKey(msg.EntityType, msg.EntityID), Value: b})
}

func ApplySpentTotal(ctx context.Context, db *sql.DB, msg SpentTotalMessage) error {
	if db == nil {
		return errors.New("postgres db is nil")
	}
	if err := msg.Validate(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var res sql.Result
	switch msg.EntityType {
	case "user":
		res, err = tx.ExecContext(ctx, "UPDATE users SET spent = GREATEST(spent, $1) WHERE id = $2", msg.SpentTotal, msg.EntityID)
	case "campaign":
		res, err = tx.ExecContext(ctx, "UPDATE campaigns SET cum_done_dollars = GREATEST(cum_done_dollars, $1) WHERE campaign_id = $2", msg.SpentTotal, msg.EntityID)
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("spent total target not found: %s:%s", msg.EntityType, msg.EntityID)
	}
	return tx.Commit()
}
