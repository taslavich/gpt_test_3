package kafka_service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

type SpentTotalMessage struct {
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	SpentTotal float64 `json:"spent_total"`
	SnapshotTS string  `json:"snapshot_ts"`
}

func SpentTotalKey(entityType, entityID string) []byte { return []byte(entityType + ":" + entityID) }
func ScanSpentTotals(ctx context.Context, rdb *redis.Client, scanCount int64, publish func(context.Context, SpentTotalMessage) error) error {
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
				if err != nil {
					log.Printf("⚠️ spent total get failed key=%s: %v", key, err)
					continue
				}
				val, err := strconv.ParseFloat(raw, 64)
				if err != nil {
					log.Printf("⚠️ invalid spent total key=%s value=%q: %v", key, raw, err)
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
				if err := publish(ctx, SpentTotalMessage{EntityType: typ, EntityID: parts[2], SpentTotal: val, SnapshotTS: time.Now().UTC().Format(time.RFC3339)}); err != nil {
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
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return writer.WriteMessages(ctx, kafka.Message{Key: SpentTotalKey(msg.EntityType, msg.EntityID), Value: b})
}
func ApplySpentTotal(ctx context.Context, db *sql.DB, msg SpentTotalMessage) error {
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
	default:
		return fmt.Errorf("unknown entity_type %q", msg.EntityType)
	}
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("spent total target not found: %s:%s", msg.EntityType, msg.EntityID)
	}
	return tx.Commit()
}
