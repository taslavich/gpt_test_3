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

func ProcessKafkaMessagesClicks(
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
		clickhouseBatchConfig[eventspb.ClickEvent]{
			LogName:    "CLICKS",
			CommitName: "click",
			Unmarshal:  unmarshalClickEvent,
			HasData:    hasDataClickProtoCH,
			Insert:     insertBatchClicks,
		},
	)

	return err
}

func unmarshalClickEvent(value []byte) (eventspb.ClickEvent, error) {
	var record eventspb.ClickEvent
	if err := proto.Unmarshal(value, &record); err != nil {
		return eventspb.ClickEvent{}, err
	}

	return record, nil
}

func hasDataClickProtoCH(record *eventspb.ClickEvent) bool {
	if record == nil {
		return false
	}

	return record.ClicksUuid != "" && record.OrtbUuid != ""
}

func insertBatchClicks(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []eventspb.ClickEvent,
) (clickhouseInsertStats, error) {
	var stats clickhouseInsertStats

	if len(records) == 0 {
		return stats, nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			clicks_uuid,
			uuid,
			event_time_clicks,
			ad_format
		)
	`, table)

	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		return stats, fmt.Errorf("PrepareBatch: %w", err)
	}

	for i := range records {
		r := &records[i]

		clicks_u, err := uuid.Parse(r.ClicksUuid)
		if err != nil {
			stats.BadUUIDCount++
			clicks_u = uuid.Nil
		}

		ortb_u, err := uuid.Parse(r.OrtbUuid)
		if err != nil {
			stats.BadUUIDCount++
			ortb_u = uuid.Nil
		}

		ts := time.UnixMilli(r.EventTimeClicksMs).UTC()

		if err := batch.Append(clicks_u, ortb_u, ts, r.Format); err != nil {
			stats.AppendErrors++
			return stats, fmt.Errorf("Record %d: batch.Append: %w", i, err)
		}
	}

	if err := batch.Send(); err != nil {
		return stats, fmt.Errorf("batch.Send: %w", err)
	}

	return stats, nil
}
