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
<<<<<<< HEAD
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
=======
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
>>>>>>> opt
)

func ProcessKafkaMessagesImpressions(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
<<<<<<< HEAD
	timeoutMs int,
) error {
	if batchSize <= 0 {
		return nil
	}

	start := time.Now()
	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMs))
	defer cancel()

	records := make([]types.Impressions, 0, batchSize)
	commitMap := make(map[int]kafka.Message, 64)

	var readCount, badJSONCount, emptyCount int
=======
	timeoutMS int,
) error {
	start := time.Now()
	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMS))
	defer cancel()

	commitMap := make(map[int]kafka.Message, 64)
	records := make([]eventspb.ImpressionEvent, 0, batchSize)

	var (
		readCount     int
		badProtoCount int
		emptyCount    int
	)
>>>>>>> opt

	for len(records) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			return err
		}

		readCount++
<<<<<<< HEAD

		var record types.Impressions
		if err := json.Unmarshal(msg.Value, &record); err != nil {
			badJSONCount++
			rememberCommitMessage(commitMap, msg)
			continue
		}

		if !types.HasDataImpressions(record) {
			emptyCount++
			rememberCommitMessage(commitMap, msg)
=======
		rememberCommitMessage(commitMap, msg)

		var record eventspb.ImpressionEvent
		if err := proto.Unmarshal(msg.Value, &record); err != nil {
			badProtoCount++
			continue
		}

		if record.Uuid == "" || record.EventTimeImpressionsMs == 0 {
			emptyCount++
>>>>>>> opt
			continue
		}

		records = append(records, record)
<<<<<<< HEAD
		rememberCommitMessage(commitMap, msg)
	}

	if len(records) > 0 {
		if err := insertBatchImpressions(ctx, ch, table, records); err != nil {
			return err
		}
	}

	commitMessages := compactCommitMessages(commitMap)
	if len(commitMessages) > 0 {
		if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
			return fmt.Errorf("commit impression offsets: %w", err)
		}
		log.Printf(
			"IMPRESSIONS batch: read=%d inserted=%d offsets=%d bad_json=%d empty=%d duration=%s",
			readCount,
			len(records),
			len(commitMessages),
			badJSONCount,
			emptyCount,
			time.Since(start),
		)
	}

=======
	}

	if len(records) == 0 {
		if len(commitMap) > 0 {
			commitMessages := compactCommitMessages(commitMap)
			if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
				return fmt.Errorf("commit skipped impression messages: %w", err)
			}
		}

		return nil
	}

	if err := insertBatchImpressions(ctx, ch, table, records); err != nil {
		return err
	}

	commitMessages := compactCommitMessages(commitMap)
	if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
		return fmt.Errorf("commit impression messages: %w", err)
	}

	log.Printf(
		"IMPRESSIONS batch: read=%d inserted=%d bad_proto=%d empty=%d offsets=%d duration=%s",
		readCount,
		len(records),
		badProtoCount,
		emptyCount,
		len(commitMessages),
		time.Since(start),
	)

>>>>>>> opt
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
