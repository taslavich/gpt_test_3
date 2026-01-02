package clickhouse_loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func ProcessKafkaMessages(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
) error {
	readCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var (
		messages []kafka.Message
		records  []types.StatisticsRecord
	)

	for i := 0; i < batchSize; i++ {

		msg, err := reader.ReadMessage(readCtx)
		if err != nil {
			// таймаут — значит новых сообщений пока нет
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return err
		}

		var record types.StatisticsRecord
		if err := json.Unmarshal(msg.Value, &record); err != nil {
			log.Printf("⚠️ Failed to parse Kafka message: %v", err)
			continue
		}

		if !hasData(record) {
			continue
		}

		records = append(records, record)
		messages = append(messages, msg)
	}

	if len(records) == 0 {
		time.Sleep(1 * time.Second)
		return nil
	}

	// native batch insert в ClickHouse
	if err := insertBatch(ctx, ch, table, records); err != nil {
		return err
	}

	// коммит offsets ТОЛЬКО после успешной вставки
	if err := reader.CommitMessages(ctx, messages...); err != nil {
		log.Printf("⚠️ Failed to commit Kafka offsets: %v", err)
	} else {
		log.Println("COMMITED %d", len(records))
	}

	return nil
}

func insertBatch(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []types.StatisticsRecord,
) error {

	if len(records) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			uuid,
			timestamp,
			typic,
			spp_domain,
			bid_request,
			geo_column,
			city_id,
			bid_responses,
			bid_response_winner,
			adm_ip,
			adm
		)
	`, table)

	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("PrepareBatch: %w", err)
	}

	for _, r := range records {

		u, err := uuid.Parse(r.UUID)
		if err != nil {
			u = uuid.Nil
		}

		ts := parseTimestampUTC(r.TIMESTAMP)

		adm := r.ADM == "1"

		var cityID uint32
		if r.CITY_ID_COLUMN != "" {
			id64, err := strconv.ParseUint(r.CITY_ID_COLUMN, 10, 32)
			if err != nil {
				log.Printf("Cannot parse CITY_ID_COLUMN %q: %v", r.CITY_ID_COLUMN, err)
			} else {
				cityID = uint32(id64)
			}
		}

		if err := batch.Append(
			u,
			ts,
			r.TYPIC,
			r.SPP_DOMAIN,
			r.BID_REQUEST,
			r.GEO_COLUMN,
			cityID,
			r.BID_RESPONSES,
			r.BID_RESPONSE_WINNER,
			r.ADM_IP,
			adm,
		); err != nil {
			return fmt.Errorf("batch.Append: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("batch.Send: %w", err)
	}

	return nil
}

func parseTimestampUTC(s string) time.Time {
	// fallback: начало Unix-времени
	fallback := time.Unix(0, 0).UTC()

	if s == "" {
		return fallback
	}

	layouts := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t
		}
	}

	return fallback
}

// Проверяет есть ли хотя бы одно непустое поле
func hasData(record types.StatisticsRecord) bool {
	return record.BID_REQUEST != "" ||
		record.GEO_COLUMN != "" ||
		record.CITY_ID_COLUMN != "" ||
		record.BID_RESPONSES != "" ||
		record.BID_RESPONSE_WINNER != "" ||
		record.ADM_IP != "" ||
		record.ADM != "" ||
		record.UUID != "" ||
		record.TIMESTAMP != "" ||
		record.SPP_DOMAIN != "" ||
		record.TYPIC != ""
}

func CreateTable(ctx context.Context, ch clickhouse.Conn, tableName string) error {
	err := ch.Exec(ctx, fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            uuid UUID,
            timestamp DateTime64(3),
			typic String,
            spp_domain String,
            bid_request String,
            geo_column String,
			city_id UInt32,
            bid_responses String,
            bid_response_winner String,
            adm_ip String,
			adm Bool,
            updated_at DateTime64(3) DEFAULT now64(),
			INDEX idx_updated_at_minmax updated_at TYPE minmax GRANULARITY 1
        ) ENGINE = ReplacingMergeTree(updated_at)
        ORDER BY (uuid, updated_at)
        PRIMARY KEY uuid
        SETTINGS index_granularity = 8192
    `, tableName))
	return err
}
