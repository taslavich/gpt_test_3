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
	clicksRecordsBuffer  []eventspb.ClickEvent
)

func ProcessKafkaMessagesClicks(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
	timeoutMs int,
) error {
	if batchSize <= 0 {
		return nil
	}

	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMs))
	defer cancel()

	commitOnlyMap := make(map[int]kafka.Message, 64)

	for len(clicksRecordsBuffer) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			return err
		}

		var record eventspb.ClickEvent
		if err := proto.Unmarshal(msg.Value, &record); err != nil {
			rememberCommitMessage(commitOnlyMap, msg)
			continue
		}

		if record.Uuid == "" || record.EventTimeClicksMs == 0 {
			rememberCommitMessage(commitOnlyMap, msg)
			continue
		}

		clicksRecordsBuffer = append(clicksRecordsBuffer, record)
		clicksMessagesBuffer = append(clicksMessagesBuffer, msg)
	}

	if len(clicksRecordsBuffer) == 0 {
		commitSkipped(ctx, reader, commitOnlyMap, "click skipped")
		return nil
	}

	insertCount := len(clicksRecordsBuffer)
	if insertCount > batchSize {
		insertCount = batchSize
	}

	records := clicksRecordsBuffer[:insertCount]
	messages := clicksMessagesBuffer[:insertCount]

	if err := insertBatchClicks(ctx, ch, table, records); err != nil {
		return err
	}

	commitMessages := compactCommitMessagesFromSlice(messages)
	if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
		return fmt.Errorf("commit click offsets after successful insert failed: %w", err)
	}
	log.Printf(
		"CLICKS batch: inserted=%d offsets=%d",
		insertCount,
		len(commitMessages),
	)

	clicksRecordsBuffer = clicksRecordsBuffer[insertCount:]
	clicksMessagesBuffer = clicksMessagesBuffer[insertCount:]
	return nil
}

func insertBatchClicks(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []eventspb.ClickEvent,
) error {
	if len(records) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			uuid,
			event_time_clicks,
			ad_format
		)
	`, table)

	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("PrepareBatch: %w", err)
	}

	for i := range records {
		r := &records[i]
		u, err := uuid.Parse(r.Uuid)
		if err != nil {
			u = uuid.Nil
		}

		ts := time.UnixMilli(r.EventTimeClicksMs).UTC()

		if err := batch.Append(u, ts, r.Format); err != nil {
			return fmt.Errorf("batch.Append: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("batch.Send: %w", err)
	}

	return nil
}
