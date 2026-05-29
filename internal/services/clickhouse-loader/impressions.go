package clickhouse_loader

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
)

var (
	impressionsMessagesBuffer []kafka.Message
	impressionsRecordsBuffer  []*eventspb.ImpressionEvent
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

		record := &eventspb.ImpressionEvent{}
		if err := proto.Unmarshal(msg.Value, record); err != nil {
			log.Printf("!!! Failed to parse Kafka impression protobuf message: %v", err)

			if err := reader.CommitMessages(ctx, msg); err != nil {
				return fmt.Errorf("commit bad impression message: %w", err)
			}

			continue
		}

		if record.Uuid == "" || record.EventTimeImpressionsMs == 0 {
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
	records []*eventspb.ImpressionEvent,
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
		u, err := uuid.Parse(r.Uuid)
		if err != nil {
			u = uuid.Nil
		}

		ts := time.UnixMilli(r.EventTimeImpressionsMs).UTC()

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
