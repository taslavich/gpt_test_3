package clickhouse_loader

import (
	"context"
	"fmt"
	"log"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/segmentio/kafka-go"
)

func ProcessKafkaMessages(
	ctx context.Context,
	readerOrtb *kafka.Reader,
	readerClicks *kafka.Reader,
	readerImpressions *kafka.Reader,
	ch clickhouse.Conn,
	tableOrtb string,
	tableClicks string,
	tableImpressions string,
	batchSizeOrtb int,
	batchSizeClicks int,
	batchSizeImpressions int,
	timeoutSec int,
) error {

	if err := ProcessKafkaMessagesOrtb(ctx, readerOrtb, ch, tableOrtb, batchSizeOrtb, timeoutSec); err != nil {
		return fmt.Errorf("Cannot ProcessKafkaMessagesOrtb: %w", err)
	}

	if err := ProcessKafkaMessagesImpressions(ctx, readerImpressions, ch, tableImpressions, batchSizeImpressions, timeoutSec); err != nil {
		log.Printf("Cannot ProcessKafkaMessagesImpressions: %v", err)
	}

	if err := ProcessKafkaMessagesClicks(ctx, readerClicks, ch, tableClicks, batchSizeClicks, timeoutSec); err != nil {
		log.Printf("Cannot ProcessKafkaMessagesClicks: %v", err)
	}

	return nil
}
