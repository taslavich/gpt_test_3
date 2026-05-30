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
) error {
<<<<<<< HEAD
	if err := ProcessKafkaMessagesOrtb(
		ctx,
		kafkaReaders.Ortb,
		ch,
		Clickhouse.TableOrtb,
		Clickhouse.BatchSizeOrtb,
		timeoutSec,
		Clickhouse.BatchTimeoutMS,
	); err != nil {
=======
	if err := ProcessKafkaMessagesOrtb(ctx, kafkaReaders.Ortb, ch, Clickhouse.TableOrtb, Clickhouse.BatchSizeOrtb, timeoutSec, timeoutMS); err != nil {
>>>>>>> opt
		return fmt.Errorf("Cannot ProcessKafkaMessagesOrtb: %w", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()
<<<<<<< HEAD
		if err := ProcessKafkaMessagesImpressions(
			ctx,
			kafkaReaders.Impressions,
			ch,
			Clickhouse.TableImpressions,
			Clickhouse.BatchSizeImpressions,
			timeoutSec,
			Clickhouse.BatchTimeoutMS,
		); err != nil {
=======
		if err := ProcessKafkaMessagesImpressions(ctx, kafkaReaders.Impressions, ch, Clickhouse.TableImpressions, Clickhouse.BatchSizeImpressions, timeoutSec, timeoutMS); err != nil {
>>>>>>> opt
			errCh <- fmt.Errorf("Cannot ProcessKafkaMessagesImpressions: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
<<<<<<< HEAD
		if err := ProcessKafkaMessagesClicks(
			ctx,
			kafkaReaders.Clicks,
			ch,
			Clickhouse.TableClicks,
			Clickhouse.BatchSizeClicks,
			timeoutSec,
			Clickhouse.BatchTimeoutMS,
		); err != nil {
=======
		if err := ProcessKafkaMessagesClicks(ctx, kafkaReaders.Clicks, ch, Clickhouse.TableClicks, Clickhouse.BatchSizeClicks, timeoutSec, timeoutMS); err != nil {
>>>>>>> opt
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

	return nil
}
