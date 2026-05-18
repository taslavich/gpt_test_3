package clickhouse_loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func ProcessKafkaMessagesImpressions(
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
		records  []types.Impressions
	)

	for i := 0; i < batchSize; i++ {

		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			// таймаут — значит новых сообщений пока нет
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return err
		}

		var record types.Impressions
		if err := json.Unmarshal(msg.Value, &record); err != nil {
			log.Printf("!!! Failed to parse Kafka message: %v", err)
			continue
		}

		if !types.HasDataImpressions(record) {
			continue
		}

		records = append(records, record)
		messages = append(messages, msg)
	}

	if len(records) < batchSize {
		time.Sleep(1 * time.Second)
		return nil
	}

	// native batch insert в ClickHouse
	if err := insertBatchImpressions(ctx, ch, table, records); err != nil {
		return err
	}

	// коммит offsets ТОЛЬКО после успешной вставки
	if err := reader.CommitMessages(ctx, messages...); err != nil {
		log.Printf("⚠️ Failed to commit Kafka offsets: %v", err)
	} else {
		log.Printf("COMMITED %d", len(records))
	}

	return nil
}

func insertBatchImpressions(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []types.Impressions,
) error {

	if len(records) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			uuid,
			event_time_impressions
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

		ts := parseTimestampUTC(r.EVENT_TIME_IMPRESSIONS)

		if err := batch.Append(
			u,
			ts,
		); err != nil {
			return fmt.Errorf("batch.Append: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("batch.Send: %w", err)
	}

	return nil
}
