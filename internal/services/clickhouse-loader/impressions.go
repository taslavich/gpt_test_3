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
	impressionsRecordsBuffer  []eventspb.ImpressionEvent
)

func ProcessKafkaMessagesImpressions(
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

	start := time.Now()
	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMs))
	defer cancel()

	commitOnlyMap := make(map[int]kafka.Message, 64)
	var readCount, badProtoCount, emptyCount int

	for len(impressionsRecordsBuffer) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			return err
		}

		readCount++

		var record eventspb.ImpressionEvent
		if err := proto.Unmarshal(msg.Value, &record); err != nil {
			badProtoCount++
			rememberCommitMessage(commitOnlyMap, msg)
			continue
		}

		if record.Uuid == "" || record.EventTimeImpressionsMs == 0 {
			emptyCount++
			rememberCommitMessage(commitOnlyMap, msg)
			continue
		}

		impressionsRecordsBuffer = append(impressionsRecordsBuffer, record)
		impressionsMessagesBuffer = append(impressionsMessagesBuffer, msg)
	}

	if len(impressionsRecordsBuffer) < batchSize {
		commitSkipped(ctx, reader, commitOnlyMap, "impression skipped")
		if readCount > 0 {
			log.Printf(
				"IMPRESSIONS BUFFER WAIT: records=%d batchSize=%d read=%d bad_proto=%d empty=%d duration=%s",
				len(impressionsRecordsBuffer),
				batchSize,
				readCount,
				badProtoCount,
				emptyCount,
				time.Since(start),
			)
		}
		return nil
	}

	records := impressionsRecordsBuffer[:batchSize]
	messages := impressionsMessagesBuffer[:batchSize]

	if err := insertBatchImpressions(ctx, ch, table, records); err != nil {
		return err
	}

	commitMessages := compactCommitMessagesFromSlice(messages)
	if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
		return fmt.Errorf("commit impression offsets after successful insert failed: %w", err)
	}

	log.Printf(
		"IMPRESSIONS batch: inserted=%d offsets=%d read=%d bad_proto=%d empty=%d duration=%s",
		len(records),
		len(commitMessages),
		readCount,
		badProtoCount,
		emptyCount,
		time.Since(start),
	)

	impressionsRecordsBuffer = impressionsRecordsBuffer[batchSize:]
	impressionsMessagesBuffer = impressionsMessagesBuffer[batchSize:]
	return nil
}

func insertBatchImpressions(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []eventspb.ImpressionEvent,
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

	for i := range records {
		r := &records[i]
		u, err := uuid.Parse(r.Uuid)
		if err != nil {
			u = uuid.Nil
		}

		ts := time.UnixMilli(r.EventTimeImpressionsMs).UTC()

		if err := batch.Append(u, ts); err != nil {
			return fmt.Errorf("batch.Append: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("batch.Send: %w", err)
	}

	return nil
}

func commitSkipped(ctx context.Context, reader *kafka.Reader, commitMap map[int]kafka.Message, logPrefix string) {
	commitMessages := compactCommitMessages(commitMap)
	if len(commitMessages) == 0 {
		return
	}
	if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
		log.Printf("⚠️ Failed to commit %s offsets: %v", logPrefix, err)
	}
}
