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

var (
	impressionsMessagesBuffer []kafka.Message
	impressionsRecordsBuffer  []types.Impressions
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

	for len(impressionsRecordsBuffer) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return err
		}

		var record types.Impressions
		if err := json.Unmarshal(msg.Value, &record); err != nil {
			log.Printf("!!! Failed to parse Kafka impression message: %v, value=%s", err, string(msg.Value))

			// Битое сообщение коммитим, чтобы consumer не застрял на нем
			if err := reader.CommitMessages(ctx, msg); err != nil {
				return fmt.Errorf("commit bad impression message: %w", err)
			}

			continue
		}

		if !types.HasDataImpressions(record) {
			log.Printf("!!! Empty impression message, skip and commit: value=%s", string(msg.Value))

			// Пустое сообщение тоже коммитим отдельно
			if err := reader.CommitMessages(ctx, msg); err != nil {
				return fmt.Errorf("commit empty impression message: %w", err)
			}

			continue
		}

		impressionsRecordsBuffer = append(impressionsRecordsBuffer, record)
		impressionsMessagesBuffer = append(impressionsMessagesBuffer, msg)
	}

	if len(impressionsRecordsBuffer) < batchSize {
		log.Printf(
			"IMPRESSIONS BUFFER WAIT: records=%d messages=%d batchSize=%d timeoutSec=%d",
			len(impressionsRecordsBuffer),
			len(impressionsMessagesBuffer),
			batchSize,
			timeoutSec,
		)
		return nil
	}

	records := impressionsRecordsBuffer[:batchSize]
	messages := impressionsMessagesBuffer[:batchSize]

	log.Printf(
		"IMPRESSIONS READY: records=%d messages=%d batchSize=%d",
		len(records),
		len(messages),
		batchSize,
	)

	if err := insertBatchImpressions(ctx, ch, table, records); err != nil {
		return err
	}

	if err := reader.CommitMessages(ctx, messages...); err != nil {
		return fmt.Errorf("commit impression messages: %w", err)
	}

	log.Printf("COMMITED IMPRESSIONS %d", len(records))

	impressionsRecordsBuffer = impressionsRecordsBuffer[batchSize:]
	impressionsMessagesBuffer = impressionsMessagesBuffer[batchSize:]

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
