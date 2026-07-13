package kafka_service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

type SpentTotal struct {
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	SpentTotal float64   `json:"spent_total"`
	SnapshotTS time.Time `json:"snapshot_ts"`
}

func ExportSpentTotals(ctx context.Context, client *redis.Client, writer *kafka.Writer) error {
	if client == nil || writer == nil {
		return errors.New("spent totals Redis or Kafka is nil")
	}
	messages := make([]kafka.Message, 0)
	for _, item := range []struct{ pattern, prefix, entity string }{
		{"spent:user:*", "spent:user:", "user"}, {"spent:campaign:*", "spent:campaign:", "campaign"},
	} {
		var cursor uint64
		for {
			keys, next, err := client.Scan(ctx, cursor, item.pattern, 512).Result()
			if err != nil {
				return fmt.Errorf("SCAN %s: %w", item.pattern, err)
			}
			if len(keys) > 0 {
				values, err := client.MGet(ctx, keys...).Result()
				if err != nil {
					return fmt.Errorf("MGET %s: %w", item.pattern, err)
				}
				for i, value := range values {
					if value == nil {
						continue
					}
					spent, err := strconv.ParseFloat(fmt.Sprint(value), 64)
					if err != nil || spent < 0 || math.IsNaN(spent) || math.IsInf(spent, 0) {
						return fmt.Errorf("invalid absolute total in %s", keys[i])
					}
					event := SpentTotal{EntityType: item.entity, EntityID: strings.TrimPrefix(keys[i], item.prefix), SpentTotal: spent, SnapshotTS: time.Now().UTC()}
					if err := ValidateSpentTotal(event); err != nil {
						return err
					}
					data, err := json.Marshal(event)
					if err != nil {
						return err
					}
					messages = append(messages, kafka.Message{Key: []byte(event.EntityType + ":" + event.EntityID), Value: data, Time: event.SnapshotTS})
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	if len(messages) == 0 {
		return nil
	}
	return writer.WriteMessages(ctx, messages...)
}

func DecodeSpentTotal(message kafka.Message) (SpentTotal, error) {
	var event SpentTotal
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return event, err
	}
	if err := ValidateSpentTotal(event); err != nil {
		return event, err
	}
	if string(message.Key) != event.EntityType+":"+event.EntityID {
		return event, errors.New("spent total Kafka key does not match payload")
	}
	return event, nil
}

func ValidateSpentTotal(event SpentTotal) error {
	if event.EntityType != "user" && event.EntityType != "campaign" {
		return errors.New("invalid spent total entity_type")
	}
	if strings.TrimSpace(event.EntityID) == "" {
		return errors.New("spent total entity_id is empty")
	}
	if event.SpentTotal < 0 || math.IsNaN(event.SpentTotal) || math.IsInf(event.SpentTotal, 0) {
		return errors.New("invalid spent_total")
	}
	if event.SnapshotTS.IsZero() {
		return errors.New("snapshot_ts is empty")
	}
	return nil
}

func ApplySpentTotal(ctx context.Context, db *sql.DB, event SpentTotal) error {
	if db == nil {
		return errors.New("PostgreSQL is nil")
	}
	if err := ValidateSpentTotal(event); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := `UPDATE users SET spent = GREATEST(spent, $1) WHERE id::text = $2`
	if event.EntityType == "campaign" {
		query = `UPDATE campaigns SET spent = GREATEST(spent, $1) WHERE campaign_id::text = $2`
	}
	result, err := tx.ExecContext(ctx, query, event.SpentTotal, event.EntityID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("spent total target %s:%s not found", event.EntityType, event.EntityID)
	}
	return tx.Commit()
}
