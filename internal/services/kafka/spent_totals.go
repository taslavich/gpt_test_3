package kafka_service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

const (
	defaultSpentTotalsScanCount        = 2000
	defaultSpentTotalsExportBatchSize  = 2000
	defaultSpentTotalsExportBatchBytes = 4 << 20
)

// SpentAmount keeps the Redis/Kafka/PostgreSQL amount as an exact decimal.
// Redis stores INCRBYFLOAT results as strings, Kafka carries a JSON number,
// and PostgreSQL receives the same decimal text cast to NUMERIC. No float64
// conversion is performed in this synchronization path.
type SpentAmount struct {
	decimal.Decimal
	valid bool
}

func ParseSpentAmount(raw string) (SpentAmount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SpentAmount{}, errors.New("spent amount is empty")
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return SpentAmount{}, fmt.Errorf("parse spent amount %q: %w", raw, err)
	}
	if value.IsNegative() {
		return SpentAmount{}, errors.New("spent amount is negative")
	}
	return SpentAmount{Decimal: value, valid: true}, nil
}

func (a SpentAmount) MarshalJSON() ([]byte, error) {
	if !a.valid {
		return nil, errors.New("spent amount is not set")
	}
	if a.Decimal.IsNegative() {
		return nil, errors.New("spent amount is negative")
	}
	// Decimal.String() is a valid finite JSON number and preserves the exact
	// decimal value read from Redis.
	return []byte(a.Decimal.String()), nil
}

func (a *SpentAmount) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("spent amount target is nil")
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		return errors.New("spent amount is empty")
	}
	// Accept both the canonical numeric payload and a quoted decimal for
	// backward/operational compatibility. New exports always use a JSON number.
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		var unquoted string
		if err := json.Unmarshal(data, &unquoted); err != nil {
			return fmt.Errorf("decode quoted spent amount: %w", err)
		}
		raw = unquoted
	}
	value, err := ParseSpentAmount(raw)
	if err != nil {
		return err
	}
	*a = value
	return nil
}

type SpentTotal struct {
	EntityType string      `json:"entity_type"`
	EntityID   string      `json:"entity_id"`
	SpentTotal SpentAmount `json:"spent_total"`
	SnapshotTS time.Time   `json:"snapshot_ts"`
}

type SpentTotalsExportConfig struct {
	ScanCount  int64
	BatchSize  int
	BatchBytes int
}

func (cfg SpentTotalsExportConfig) withDefaults() SpentTotalsExportConfig {
	if cfg.ScanCount <= 0 {
		cfg.ScanCount = defaultSpentTotalsScanCount
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultSpentTotalsExportBatchSize
	}
	if cfg.BatchBytes <= 0 {
		cfg.BatchBytes = defaultSpentTotalsExportBatchBytes
	}
	return cfg
}

// ExportSpentTotals retains the old API and uses bounded production defaults.
func ExportSpentTotals(ctx context.Context, client *redis.Client, writer *kafka.Writer) error {
	return ExportSpentTotalsWithConfig(ctx, client, writer, SpentTotalsExportConfig{})
}

// ExportSpentTotalsWithConfig scans Redis DB5 in bounded pages and publishes
// bounded, synchronously acknowledged Kafka chunks. Redis is never modified.
// If SCAN, MGET, parsing, marshaling or Kafka publication fails, the pass stops;
// the next scheduled pass reads the current absolute totals again.
func ExportSpentTotalsWithConfig(ctx context.Context, client *redis.Client, writer *kafka.Writer, cfg SpentTotalsExportConfig) error {
	if client == nil || writer == nil {
		return errors.New("spent totals Redis or Kafka is nil")
	}
	cfg = cfg.withDefaults()

	messages := make([]kafka.Message, 0, cfg.BatchSize)
	batchBytes := 0
	flush := func() error {
		if len(messages) == 0 {
			return nil
		}
		if err := writer.WriteMessages(ctx, messages...); err != nil {
			return fmt.Errorf("publish spent_totals chunk messages=%d bytes=%d: %w", len(messages), batchBytes, err)
		}
		messages = messages[:0]
		batchBytes = 0
		return nil
	}

	for _, item := range []struct{ pattern, prefix, entity string }{
		{"spent:user:*", "spent:user:", "user"},
		{"spent:campaign:*", "spent:campaign:", "campaign"},
	} {
		var cursor uint64
		for {
			keys, next, err := client.Scan(ctx, cursor, item.pattern, cfg.ScanCount).Result()
			if err != nil {
				return fmt.Errorf("SCAN %s cursor=%d: %w", item.pattern, cursor, err)
			}
			if len(keys) > 0 {
				values, err := client.MGet(ctx, keys...).Result()
				if err != nil {
					return fmt.Errorf("MGET %s keys=%d: %w", item.pattern, len(keys), err)
				}
				if len(values) != len(keys) {
					return fmt.Errorf("MGET %s returned %d values for %d keys", item.pattern, len(values), len(keys))
				}
				snapshotTS := time.Now().UTC()
				for i, value := range values {
					if value == nil {
						// spent:* keys are cumulative and are not intentionally deleted.
						// Treat a disappearing value as a failed pass so it is retried.
						return fmt.Errorf("MGET %s returned nil for key %s", item.pattern, keys[i])
					}
					spent, err := ParseSpentAmount(fmt.Sprint(value))
					if err != nil {
						return fmt.Errorf("invalid absolute total in %s: %w", keys[i], err)
					}
					entityID := strings.TrimSpace(strings.TrimPrefix(keys[i], item.prefix))
					event := SpentTotal{EntityType: item.entity, EntityID: entityID, SpentTotal: spent, SnapshotTS: snapshotTS}
					if err := ValidateSpentTotal(event); err != nil {
						return fmt.Errorf("validate Redis key %s: %w", keys[i], err)
					}
					data, err := json.Marshal(event)
					if err != nil {
						return fmt.Errorf("marshal %s:%s: %w", event.EntityType, event.EntityID, err)
					}
					message := kafka.Message{
						Key:   []byte(event.EntityType + ":" + event.EntityID),
						Value: data,
						Time:  event.SnapshotTS,
					}
					messageBytes := len(message.Key) + len(message.Value)
					if len(messages) > 0 && (len(messages) >= cfg.BatchSize || batchBytes+messageBytes > cfg.BatchBytes) {
						if err := flush(); err != nil {
							return err
						}
					}
					messages = append(messages, message)
					batchBytes += messageBytes
					if len(messages) >= cfg.BatchSize || batchBytes >= cfg.BatchBytes {
						if err := flush(); err != nil {
							return err
						}
					}
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return flush()
}

func DecodeSpentTotal(message kafka.Message) (SpentTotal, error) {
	var event SpentTotal
	decoder := json.NewDecoder(bytes.NewReader(message.Value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, fmt.Errorf("decode spent total: %w", err)
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
	if _, err := uuid.Parse(event.EntityID); err != nil {
		return fmt.Errorf("spent total entity_id is not UUID: %w", err)
	}
	if !event.SpentTotal.valid || event.SpentTotal.Decimal.IsNegative() {
		return errors.New("invalid spent_total")
	}
	if event.SnapshotTS.IsZero() {
		return errors.New("snapshot_ts is empty")
	}
	return nil
}

// ApplySpentTotal retains the old single-event API while using the same exact
// decimal and transactional batch implementation.
func ApplySpentTotal(ctx context.Context, db *sql.DB, event SpentTotal) error {
	return ApplySpentTotalsBatch(ctx, db, []SpentTotal{event})
}

// ApplySpentTotalsBatch validates every Kafka message, writes users and
// campaigns in one PostgreSQL transaction, and never converts money through
// float64. Duplicate absolute snapshots are all accepted; SQL takes MAX per
// entity only to produce one deterministic UPDATE target row. This does not
// collapse ADM/BURL billable events: those events were already individually
// accumulated in Redis before the absolute snapshots were exported.
func ApplySpentTotalsBatch(ctx context.Context, db *sql.DB, events []SpentTotal) error {
	if db == nil {
		return errors.New("PostgreSQL is nil")
	}
	if len(events) == 0 {
		return nil
	}

	userIDs := make([]string, 0, len(events))
	userTotals := make([]string, 0, len(events))
	campaignIDs := make([]string, 0, len(events))
	campaignTotals := make([]string, 0, len(events))
	userUnique := make(map[string]struct{})
	campaignUnique := make(map[string]struct{})

	for i, event := range events {
		if err := ValidateSpentTotal(event); err != nil {
			return fmt.Errorf("validate spent_totals event %d: %w", i, err)
		}
		switch event.EntityType {
		case "user":
			userIDs = append(userIDs, event.EntityID)
			userTotals = append(userTotals, event.SpentTotal.Decimal.String())
			userUnique[event.EntityID] = struct{}{}
		case "campaign":
			campaignIDs = append(campaignIDs, event.EntityID)
			campaignTotals = append(campaignTotals, event.SpentTotal.Decimal.String())
			campaignUnique[event.EntityID] = struct{}{}
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin spent_totals batch transaction: %w", err)
	}
	defer tx.Rollback()

	if len(userIDs) > 0 {
		const updateUsers = `
			WITH incoming(id, spent) AS (
				SELECT * FROM unnest($1::uuid[], $2::numeric[])
			), aggregated AS (
				SELECT id, MAX(spent) AS spent
				FROM incoming
				GROUP BY id
			)
			UPDATE users AS u
			SET spent = GREATEST(u.spent, aggregated.spent)
			FROM aggregated
			WHERE u.id = aggregated.id`
		result, err := tx.ExecContext(ctx, updateUsers, pq.Array(userIDs), pq.Array(userTotals))
		if err != nil {
			return fmt.Errorf("bulk update users spent: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("users spent rows affected: %w", err)
		}
		if rows != int64(len(userUnique)) {
			return fmt.Errorf("users spent targets mismatch: updated=%d expected=%d", rows, len(userUnique))
		}
	}

	if len(campaignIDs) > 0 {
		const updateCampaigns = `
			WITH incoming(id, spent) AS (
				SELECT * FROM unnest($1::uuid[], $2::numeric[])
			), aggregated AS (
				SELECT id, MAX(spent) AS spent
				FROM incoming
				GROUP BY id
			)
			UPDATE campaigns AS c
			SET spent = GREATEST(c.spent, aggregated.spent)
			FROM aggregated
			WHERE c.campaign_id = aggregated.id`
		result, err := tx.ExecContext(ctx, updateCampaigns, pq.Array(campaignIDs), pq.Array(campaignTotals))
		if err != nil {
			return fmt.Errorf("bulk update campaigns spent: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("campaigns spent rows affected: %w", err)
		}
		if rows != int64(len(campaignUnique)) {
			return fmt.Errorf("campaigns spent targets mismatch: updated=%d expected=%d", rows, len(campaignUnique))
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit spent_totals batch transaction: %w", err)
	}
	return nil
}
