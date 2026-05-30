package kafka_loader

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

func ProcessKafkaMessages(
	ctx context.Context,

	redisClientOrtb *redis.Client,
	redisClientImpressions *redis.Client,
	redisClientClicks *redis.Client,

	kafkaWriterOrtb *kafka.Writer,
	kafkaWriterImpressions *kafka.Writer,
	kafkaWriterClicks *kafka.Writer,

	batchSizeOrtb int64,
	batchSizeImpressions int64,
	batchSizeClicks int64,
	redisSetOrtb string,
	redisSetImpressions string,
	redisSetClicks string,
) error {

	if err := ProcessBatchOrtb(ctx, redisClientOrtb, kafkaWriterOrtb, batchSizeOrtb, redisSetOrtb); err != nil {
		return fmt.Errorf("Cannot ProcessBatchOrtb: %w", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := ProcessBatchImpressions(ctx, redisClientImpressions, kafkaWriterImpressions, batchSizeImpressions, redisSetImpressions); err != nil {
			errCh <- fmt.Errorf("Cannot ProcessBatchImpressions: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := ProcessBatchClicks(ctx, redisClientClicks, kafkaWriterClicks, batchSizeClicks, redisSetClicks); err != nil {
			errCh <- fmt.Errorf("Cannot ProcessBatchClicks: %w", err)
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

func stringSliceToAny(slice []string) []interface{} {
	result := make([]interface{}, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}
