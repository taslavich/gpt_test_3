package clickhouse_loader

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/segmentio/kafka-go"
)

type clickhouseInsertStats struct {
	BadUUIDCount int
	BadIPCount   int
	AppendErrors int
}

type clickhouseBatchConfig[T any] struct {
	LogName    string
	CommitName string

	Unmarshal func(value []byte) (T, error)
	HasData   func(record *T) bool
	Insert    func(ctx context.Context, ch clickhouse.Conn, table string, records []T) (clickhouseInsertStats, error)
}

func processKafkaMessagesBatch[T any](
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
	timeoutMs int,
	cfg clickhouseBatchConfig[T],
) (int, error) {
	if batchSize <= 0 {
		return 0, nil
	}

	if cfg.Unmarshal == nil {
		return 0, fmt.Errorf("%s unmarshal func is nil", cfg.LogName)
	}

	if cfg.HasData == nil {
		return 0, fmt.Errorf("%s hasData func is nil", cfg.LogName)
	}

	if cfg.Insert == nil {
		return 0, fmt.Errorf("%s insert func is nil", cfg.LogName)
	}

	if cfg.CommitName == "" {
		cfg.CommitName = cfg.LogName
	}

	start := time.Now()
	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMs))
	defer cancel()

	records := make([]T, 0, batchSize)
	commitMap := make(map[int]kafka.Message, 64)

	readCount := 0
	emptyCount := 0

	for len(records) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}

			return 0, err
		}

		readCount++

		record, err := cfg.Unmarshal(msg.Value)
		if err != nil {
			return 0, fmt.Errorf(
				"%s bad proto message: partition=%d offset=%d: %w",
				cfg.LogName,
				msg.Partition,
				msg.Offset,
				err,
			)
		}

		if !cfg.HasData(&record) {
			emptyCount++
			rememberCommitMessage(commitMap, msg)
			continue
		}

		records = append(records, record)
		rememberCommitMessage(commitMap, msg)
	}

	if len(records) == 0 {
		commitMessages := compactCommitMessages(commitMap)
		if len(commitMessages) > 0 {
			if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
				return 0, fmt.Errorf("commit %s empty offsets failed: %w", cfg.CommitName, err)
			}
		}

		if readCount > 0 {
			log.Printf(
				"%s batch: read=%d inserted=0 offsets=%d empty=%d duration=%s",
				cfg.LogName,
				readCount,
				len(commitMessages),
				emptyCount,
				time.Since(start),
			)
		}

		return 0, nil
	}

	stats, err := cfg.Insert(ctx, ch, table, records)
	if err != nil {
		return 0, err
	}

	commitMessages := compactCommitMessages(commitMap)
	if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
		return 0, fmt.Errorf("commit %s offsets after successful insert failed: %w", cfg.CommitName, err)
	}

	log.Printf("COMMITED %s records=%d offsets=%d", cfg.LogName, len(records), len(commitMessages))

	log.Printf(
		"%s batch: read=%d inserted=%d offsets=%d empty=%d bad_uuid=%d bad_ip=%d append_errors=%d duration=%s",
		cfg.LogName,
		readCount,
		len(records),
		len(commitMessages),
		emptyCount,
		stats.BadUUIDCount,
		stats.BadIPCount,
		stats.AppendErrors,
		time.Since(start),
	)

	return len(records), nil
}
