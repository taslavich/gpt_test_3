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

func ProcessKafkaMessagesClicks(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
	timeoutMS int,
) error {
	start := time.Now()
	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMS))
	defer cancel()

	commitMap := make(map[int]kafka.Message, 64)
	records := make([]eventspb.ClickEvent, 0, batchSize)

	var (
		readCount     int
		badProtoCount int
		emptyCount    int
	)

	for len(records) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return err
		}

		readCount++
		rememberCommitMessage(commitMap, msg)

		var record eventspb.ClickEvent
		if err := proto.Unmarshal(msg.Value, &record); err != nil {
			badProtoCount++
			continue
		}

		if record.Uuid == "" || record.EventTimeClicksMs == 0 {
			emptyCount++
			continue
		}

		records = append(records, record)
	}

	if len(records) == 0 {
		if len(commitMap) > 0 {
			commitMessages := compactCommitMessages(commitMap)
			if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
				return fmt.Errorf("commit skipped click messages: %w", err)
			}
		}

		return nil
	}

	if err := insertBatchClicks(ctx, ch, table, records); err != nil {
		return err
	}

	commitMessages := compactCommitMessages(commitMap)
	if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
		return fmt.Errorf("commit click messages: %w", err)
	}

	log.Printf(
		"CLICKS batch: read=%d inserted=%d bad_proto=%d empty=%d offsets=%d duration=%s",
		readCount,
		len(records),
		badProtoCount,
		emptyCount,
		len(commitMessages),
		time.Since(start),
	)

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
