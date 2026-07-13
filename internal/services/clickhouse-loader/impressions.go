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

func ProcessKafkaMessagesImpressions(
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
		clickhouseBatchConfig[*eventspb.ImpressionEvent]{
			LogName:    "IMPRESSIONS",
			CommitName: "impression",
			Unmarshal:  unmarshalImpressionEvent,
			HasData:    hasDataImpressionProtoCH,
			Insert:     insertBatchImpressions,
		},
	)

	return err
}

func unmarshalImpressionEvent(value []byte) (*eventspb.ImpressionEvent, error) {
	var record eventspb.ImpressionEvent
	if err := proto.Unmarshal(value, &record); err != nil {
		return nil, err
	}

	return &record, nil
}

func hasDataImpressionProtoCH(record *eventspb.ImpressionEvent) bool {
	if record == nil {
		return false
	}

	return record.ImpressionsUuid != "" && record.OrtbUuid != ""
}

func insertBatchImpressions(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []*eventspb.ImpressionEvent,
) (clickhouseInsertStats, error) {
	var stats clickhouseInsertStats

	if len(records) == 0 {
		return stats, nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			impressions_uuid,
			uuid,
			event_time_impressions,
			ad_format
		)
	`, table)

	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		return stats, fmt.Errorf("PrepareBatch: %w", err)
	}

	for i := range records {
		r := records[i]

		impressions_u, err := uuid.Parse(r.ImpressionsUuid)
		if err != nil {
			stats.BadUUIDCount++
			impressions_u = uuid.Nil
		}

		ortb_u, err := uuid.Parse(r.OrtbUuid)
		if err != nil {
			stats.BadUUIDCount++
			ortb_u = uuid.Nil
		}

		ts := time.UnixMilli(r.EventTimeImpressionsMs).UTC()

		if err := batch.Append(impressions_u, ortb_u, ts, r.Format); err != nil {
			stats.AppendErrors++
			return stats, fmt.Errorf("Record %d: batch.Append: %w", i, err)
		}
	}

	if err := batch.Send(); err != nil {
		return stats, fmt.Errorf("batch.Send: %w", err)
	}

	return stats, nil
}
