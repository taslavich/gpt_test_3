package clickhouse_loader

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
)

func ProcessKafkaMessagesConversions(
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
		clickhouseBatchConfig[eventspb.ConversionEvent]{
			LogName:    "CONVERSIONS",
			CommitName: "conversion",
			Unmarshal:  unmarshalConversionEvent,
			HasData:    hasDataConversionProtoCH,
			Insert:     insertBatchConversions,
		},
	)

	return err
}

func unmarshalConversionEvent(value []byte) (eventspb.ConversionEvent, error) {
	var record eventspb.ConversionEvent
	if err := proto.Unmarshal(value, &record); err != nil {
		return eventspb.ConversionEvent{}, err
	}

	return record, nil
}

func hasDataConversionProtoCH(record *eventspb.ConversionEvent) bool {
	if record == nil {
		return false
	}

	return record.ConversionsUuid != "" || record.ClicksUuid != ""
}

func insertBatchConversions(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []eventspb.ConversionEvent,
) (clickhouseInsertStats, error) {
	var stats clickhouseInsertStats

	if len(records) == 0 {
		return stats, nil
	}

	query := fmt.Sprintf(`
	INSERT INTO %s (
		conversions_uuid,
		clicks_uuid,
		payout,
		status,
		conversion_event_time
	)
	`, table)

	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		return stats, fmt.Errorf("PrepareBatch: %w", err)
	}

	for i := range records {
		r := &records[i]

		conversionsUUID, err := uuid.Parse(r.ConversionsUuid)
		if err != nil {
			stats.BadUUIDCount++
			conversionsUUID = uuid.Nil
		}

		clicksUUID, err := uuid.Parse(r.ClicksUuid)
		if err != nil {
			stats.BadUUIDCount++
			clicksUUID = uuid.Nil
		}

		payout, err := parseFloat64WithError(r.Payout)
		if err != nil {
			stats.BadPayoutCount++
			payout = 0
		}

		conversionEventTime := time.Now().UTC()
		if r.ConversionEventTimeMs > 0 {
			conversionEventTime = time.UnixMilli(
				r.ConversionEventTimeMs,
			).UTC()
		}

		if err := batch.Append(
			conversionsUUID,
			clicksUUID,
			payout,
			r.Status,
			conversionEventTime,
		); err != nil {
			stats.AppendErrors++
			return stats, fmt.Errorf(
				"Record %d: batch.Append: %w",
				i,
				err,
			)
		}
	}

	if err := batch.Send(); err != nil {
		return stats, fmt.Errorf("batch.Send: %w", err)
	}

	return stats, nil
}

func parseFloat64WithError(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}
