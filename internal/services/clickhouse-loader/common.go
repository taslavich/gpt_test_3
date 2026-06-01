package clickhouse_loader

import (
	"context"
	"fmt"
	"log"
	"sync"

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
	timeoutMS int,
	runImpressionsClicks bool,
) (int, error) {
	ortbInserted, err := ProcessKafkaMessagesOrtb(
		ctx,
		kafkaReaders.Ortb,
		ch,
		Clickhouse.TableOrtb,
		Clickhouse.BatchSizeOrtb,
		timeoutSec,
		Clickhouse.BatchTimeoutMS,
	)
	if err != nil {
		return ortbInserted, fmt.Errorf("Cannot ProcessKafkaMessagesOrtb: %w", err)
	}

	if ortbInserted == 0 {
		return 0, nil
	}

	if !runImpressionsClicks {
		return ortbInserted, nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := ProcessKafkaMessagesImpressions(
			ctx,
			kafkaReaders.Impressions,
			ch,
			Clickhouse.TableImpressions,
			Clickhouse.BatchSizeImpressions,
			timeoutSec,
			Clickhouse.BatchTimeoutMS,
		); err != nil {
			errCh <- fmt.Errorf("Cannot ProcessKafkaMessagesImpressions: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := ProcessKafkaMessagesClicks(
			ctx,
			kafkaReaders.Clicks,
			ch,
			Clickhouse.TableClicks,
			Clickhouse.BatchSizeClicks,
			timeoutSec,
			Clickhouse.BatchTimeoutMS,
		); err != nil {
			errCh <- fmt.Errorf("Cannot ProcessKafkaMessagesClicks: %w", err)
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			log.Printf("⚠️ %v", err)
		}
	}

	return ortbInserted, nil
}
