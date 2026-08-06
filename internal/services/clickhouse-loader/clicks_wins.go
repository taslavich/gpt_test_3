package clickhouse_loader

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
)

func ProcessKafkaMessagesClicksWins(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
	timeoutMs int,
) error {
	_, err := processKafkaMessagesBatch(
		ctx,
		reader,
		ch,
		table,
		batchSize,
		timeoutSec,
		timeoutMs,
		clickhouseBatchConfig[eventspb.ClicksWinEvent]{
			LogName:    "CLICKS_WINS",
			CommitName: "click win",
			Unmarshal:  unmarshalClicksWinEvent,
			HasData:    hasDataClicksWinProtoCH,
			Insert:     insertBatchClicksWins,
		},
	)

	return err
}

func unmarshalClicksWinEvent(value []byte) (eventspb.ClicksWinEvent, error) {
	var record eventspb.ClicksWinEvent
	if err := proto.Unmarshal(value, &record); err != nil {
		return eventspb.ClicksWinEvent{}, err
	}

	return record, nil
}

func hasDataClicksWinProtoCH(record *eventspb.ClicksWinEvent) bool {
	if record == nil {
		return false
	}

	return record.ClicksWinsUuid != "" && record.OrtbUuid != ""
}

func insertBatchClicksWins(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []eventspb.ClicksWinEvent,
) (clickhouseInsertStats, error) {
	var stats clickhouseInsertStats

	if len(records) == 0 {
		return stats, nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			clicks_wins_uuid,
			uuid,
			event_time_clicks_wins
		)
	`, table)

	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		return stats, fmt.Errorf("PrepareBatch: %w", err)
	}

	for i := range records {
		r := &records[i]

		clicksWinsUUID, err := uuid.Parse(r.ClicksWinsUuid)
		if err != nil {
			stats.BadUUIDCount++
			clicksWinsUUID = uuid.Nil
		}

		ortbUUID, err := uuid.Parse(r.OrtbUuid)
		if err != nil {
			stats.BadUUIDCount++
			ortbUUID = uuid.Nil
		}

		eventTime := time.Now().UTC()
		if r.EventTimeClicksWinsMs > 0 {
			eventTime = time.UnixMilli(r.EventTimeClicksWinsMs).UTC()
		}

		if err := batch.Append(clicksWinsUUID, ortbUUID, eventTime); err != nil {
			stats.AppendErrors++
			return stats, fmt.Errorf("Record %d: batch.Append: %w", i, err)
		}
	}

	if err := batch.Send(); err != nil {
		return stats, fmt.Errorf("batch.Send: %w", err)
	}

	return stats, nil
}
