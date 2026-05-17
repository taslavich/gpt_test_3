package kafka_loader

import (
	"context"
	"fmt"
	"log"

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

	if err := ProcessBatchImpressions(ctx, redisClientImpressions, kafkaWriterImpressions, batchSizeImpressions, redisSetImpressions); err != nil {
		log.Printf("Cannot ProcessBatchImpressions: %v", err)
	}

	if err := ProcessBatchClicks(ctx, redisClientClicks, kafkaWriterClicks, batchSizeClicks, redisSetClicks); err != nil {
		log.Printf("Cannot ProcessBatchClicks: %v", err)
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
