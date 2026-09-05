package kafka_loader

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

// ProcessBatchPercenterHistory moves durable history events from Redis to Kafka.
// Redis list movement is atomic. Kafka failures restore the events to ready.
// A crash after Kafka write but before Redis cleanup can replay an event; the
// ClickHouse table deduplicates by event_id through ReplacingMergeTree.
func ProcessBatchPercenterHistory(
	ctx context.Context,
	redisClient *redis.Client,
	kafkaWriter *kafka.Writer,
	batchSize int64,
	readyKey string,
	processingKey string,
	deadKey string,
) (int, error) {
	if redisClient == nil {
		return 0, fmt.Errorf("percenter history Redis client is nil")
	}
	if kafkaWriter == nil {
		return 0, fmt.Errorf("percenter history Kafka writer is nil")
	}
	if batchSize <= 0 {
		return 0, nil
	}
	if readyKey == "" || processingKey == "" || deadKey == "" {
		return 0, fmt.Errorf("percenter history Redis keys must not be empty")
	}

	if err := recoverPercenterHistoryProcessing(ctx, redisClient, readyKey, processingKey); err != nil {
		return 0, err
	}

	raws := make([]string, 0, batchSize)
	messages := make([]kafka.Message, 0, batchSize)
	deadCount := 0

	for int64(len(raws)) < batchSize {
		raw, err := redisClient.RPopLPush(ctx, readyKey, processingKey).Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("move percenter history ready->processing: %w", err)
		}

		event, err := percenter.UnmarshalHistoryEvent([]byte(raw))
		if err != nil {
			if moveErr := moveMalformedHistoryToDead(ctx, redisClient, processingKey, deadKey, raw); moveErr != nil {
				return 0, fmt.Errorf("invalid percenter history event and dead-letter move failed: decode=%v move=%w", err, moveErr)
			}
			deadCount++
			log.Printf("[PERCENTER_HISTORY][REDIS_DEAD_LETTER] decode_error=%v", err)
			continue
		}

		raws = append(raws, raw)
		messages = append(messages, kafka.Message{Key: []byte(event.EventID), Value: []byte(raw)})
	}

	if len(messages) == 0 {
		if deadCount > 0 {
			return 0, fmt.Errorf("percenter history moved %d malformed Redis events to dead-letter", deadCount)
		}
		return 0, nil
	}

	if err := kafkaWriter.WriteMessages(ctx, messages...); err != nil {
		if restoreErr := restorePercenterHistoryBatch(ctx, redisClient, readyKey, processingKey, raws); restoreErr != nil {
			return 0, fmt.Errorf("write percenter history to Kafka failed: %v; Redis restore failed: %w", err, restoreErr)
		}
		return 0, fmt.Errorf("write percenter history to Kafka failed: %w", err)
	}

	if err := cleanupPercenterHistoryProcessing(ctx, redisClient, processingKey, raws); err != nil {
		// Kafka already acknowledged the batch. Leave processing records intact;
		// the next run/restart replays them. event_id makes the CH sink idempotent.
		return len(messages), fmt.Errorf("Kafka write succeeded but percenter history Redis cleanup failed: %w", err)
	}

	log.Printf("[PERCENTER_HISTORY][KAFKA] sent=%d dead=%d", len(messages), deadCount)
	if deadCount > 0 {
		return len(messages), fmt.Errorf("percenter history moved %d malformed Redis events to dead-letter", deadCount)
	}
	return len(messages), nil
}

func recoverPercenterHistoryProcessing(ctx context.Context, client *redis.Client, readyKey, processingKey string) error {
	for {
		raw, err := client.RPopLPush(ctx, processingKey, readyKey).Result()
		if err == redis.Nil {
			return nil
		}
		if err != nil {
			return fmt.Errorf("recover percenter history processing list: %w", err)
		}
		if raw == "" {
			continue
		}
	}
}

func cleanupPercenterHistoryProcessing(ctx context.Context, client *redis.Client, processingKey string, raws []string) error {
	pipe := client.Pipeline()
	for _, raw := range raws {
		pipe.LRem(ctx, processingKey, 1, raw)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func validatePercenterHistoryListType(ctx context.Context, client *redis.Client, key string) error {
	keyType, err := client.Type(ctx, key).Result()
	if err != nil {
		return err
	}
	if keyType != "none" && keyType != "list" {
		return fmt.Errorf("Redis key %s has unexpected type: %s", key, keyType)
	}
	return nil
}

func restorePercenterHistoryBatch(ctx context.Context, client *redis.Client, readyKey, processingKey string, raws []string) error {
	if len(raws) == 0 {
		return nil
	}
	if err := validatePercenterHistoryListType(ctx, client, readyKey); err != nil {
		return err
	}
	if err := validatePercenterHistoryListType(ctx, client, processingKey); err != nil {
		return err
	}

	_, err := client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, raw := range raws {
			pipe.LRem(ctx, processingKey, 1, raw)
			pipe.RPush(ctx, readyKey, raw)
		}
		return nil
	})
	return err
}

func moveMalformedHistoryToDead(ctx context.Context, client *redis.Client, processingKey, deadKey, raw string) error {
	const maxRetries = 8
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := client.Watch(ctx, func(tx *redis.Tx) error {
			processingType, err := tx.Type(ctx, processingKey).Result()
			if err != nil {
				return err
			}
			if processingType != "none" && processingType != "list" {
				return fmt.Errorf("Redis key %s has unexpected type: %s", processingKey, processingType)
			}
			deadType, err := tx.Type(ctx, deadKey).Result()
			if err != nil {
				return err
			}
			if deadType != "none" && deadType != "list" {
				return fmt.Errorf("Redis key %s has unexpected type: %s", deadKey, deadType)
			}

			items, err := tx.LRange(ctx, processingKey, 0, -1).Result()
			if err != nil {
				return err
			}
			found := false
			for _, item := range items {
				if item == raw {
					found = true
					break
				}
			}
			if !found {
				return nil
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.LRem(ctx, processingKey, 1, raw)
				pipe.RPush(ctx, deadKey, raw)
				return nil
			})
			return err
		}, processingKey)
		if err == nil {
			return nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}
	return fmt.Errorf("move malformed percenter history event to dead list: Redis transaction conflicted after %d retries", maxRetries)
}
