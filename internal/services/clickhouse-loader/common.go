package clickhouse_loader

import (
	"context"
	"fmt"
	"log"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	kafka_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/kafka"
)

func ProcessKafkaMessages(
	ctx context.Context,
	kafkaReaders *kafka_service.KafkaReaders,
	ch clickhouse.Conn,
	Clickhouse config.ClickhouseConfig,
	timeoutSec int,
) error {

	if err := ProcessKafkaMessagesOrtb(ctx, kafkaReaders.Ortb, ch, Clickhouse.TableOrtb, Clickhouse.BatchSizeOrtb, timeoutSec); err != nil {
		return fmt.Errorf("Cannot ProcessKafkaMessagesOrtb: %w", err)
	}

	if err := ProcessKafkaMessagesImpressions(ctx, kafkaReaders.Impressions, ch, Clickhouse.TableImpressions, Clickhouse.BatchSizeImpressions, timeoutSec); err != nil {
		log.Printf("Cannot ProcessKafkaMessagesImpressions: %v", err)
	}

	if err := ProcessKafkaMessagesClicks(ctx, kafkaReaders.Clicks, ch, Clickhouse.TableClicks, Clickhouse.BatchSizeClicks, timeoutSec); err != nil {
		log.Printf("Cannot ProcessKafkaMessagesClicks: %v", err)
	}

	return nil
}
