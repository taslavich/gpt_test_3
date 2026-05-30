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

	for len(records) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			return err
		}

		readCount++

		var record types.Impressions
		if err := json.Unmarshal(msg.Value, &record); err != nil {
			badJSONCount++
			rememberCommitMessage(commitMap, msg)
			continue
		}

		if !types.HasDataImpressions(record) {
			emptyCount++
			rememberCommitMessage(commitMap, msg)
			continue
		}

		records = append(records, record)
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
