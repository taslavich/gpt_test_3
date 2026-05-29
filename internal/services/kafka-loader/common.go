package kafka_loader

import (
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

func ProcessKafkaMessages(
	ctx context.Context,

	redisClientsOrtb []*redis.Client,
	redisClientsImpressions []*redis.Client,
	redisClientsClicks []*redis.Client,

	kafkaWriterOrtb *kafka.Writer,
	kafkaWriterImpressions *kafka.Writer,
	kafkaWriterClicks *kafka.Writer,

	batchSizeOrtb int64,
	batchSizeImpressions int64,
	batchSizeClicks int64,
	redisSetOrtb string,
	redisSetImpressions string,
	redisSetClicks string,
	runImpressionsClicks bool,
) error {
	if err := ProcessBatchOrtb(ctx, redisClientsOrtb, kafkaWriterOrtb, batchSizeOrtb, redisSetOrtb); err != nil {
		return fmt.Errorf("Cannot ProcessBatchOrtb: %w", err)
	}

	if !runImpressionsClicks {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := ProcessBatchImpressions(ctx, redisClientsImpressions, kafkaWriterImpressions, batchSizeImpressions, redisSetImpressions); err != nil {
			errCh <- fmt.Errorf("Cannot ProcessBatchImpressions: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := ProcessBatchClicks(ctx, redisClientsClicks, kafkaWriterClicks, batchSizeClicks, redisSetClicks); err != nil {
			errCh <- fmt.Errorf("Cannot ProcessBatchClicks: %w", err)
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func stringSliceToAny(slice []string) []interface{} {
	result := make([]interface{}, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

func splitLimit(total int64, shardCount int) int64 {
	if shardCount <= 0 {
		return total
	}

	if total <= 0 {
		return 0
	}

	base := total / int64(shardCount)
	if base == 0 {
		return 1
	}

	return base
}
