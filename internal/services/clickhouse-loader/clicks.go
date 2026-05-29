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
	clicksMessagesBuffer []kafka.Message
	clicksRecordsBuffer  []*eventspb.ClickEvent
)

func ProcessKafkaMessagesClicks(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
) error {
	readCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	for len(clicksRecordsBuffer) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return err
		}

		record := &eventspb.ClickEvent{}
		if err := proto.Unmarshal(msg.Value, record); err != nil {
			log.Printf("!!!! Failed to parse Kafka click protobuf message: %v", err)

			if err := reader.CommitMessages(ctx, msg); err != nil {
				return fmt.Errorf("commit bad click message: %w", err)
			}

			continue
		}

		if record.Uuid == "" || record.EventTimeClicksMs == 0 {
			if err := reader.CommitMessages(ctx, msg); err != nil {
				return fmt.Errorf("commit empty click message: %w", err)
			}

			continue
		}

		clicksRecordsBuffer = append(clicksRecordsBuffer, record)
		clicksMessagesBuffer = append(clicksMessagesBuffer, msg)
	}

	if len(clicksRecordsBuffer) < batchSize {
		log.Printf(
			"CLICKS BUFFER WAIT: records=%d messages=%d batchSize=%d timeoutSec=%d",
			len(clicksRecordsBuffer),
			len(clicksMessagesBuffer),
			batchSize,
			timeoutSec,
		)
		return nil
	}

	records := clicksRecordsBuffer[:batchSize]
	messages := clicksMessagesBuffer[:batchSize]

	log.Printf(
		"CLICKS READY: records=%d messages=%d batchSize=%d",
		len(records),
		len(messages),
		batchSize,
	)

	if err := insertBatchClicks(ctx, ch, table, records); err != nil {
		return err
	}

	if err := reader.CommitMessages(ctx, messages...); err != nil {
		return fmt.Errorf("commit click messages: %w", err)
	}

	log.Printf("COMMITED CLICKS %d", len(records))

	clicksRecordsBuffer = clicksRecordsBuffer[batchSize:]
	clicksMessagesBuffer = clicksMessagesBuffer[batchSize:]

	return nil
}

func insertBatchClicks(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []*eventspb.ClickEvent,
) error {
	if len(records) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			uuid,
			event_time_clicks
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

		ts := time.UnixMilli(r.EventTimeClicksMs).UTC()

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
