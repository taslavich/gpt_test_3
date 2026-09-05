package kafka_loader

import (
	"context"
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

func restorePercenterHistoryBatch(ctx context.Context, client *redis.Client, readyKey, processingKey string, raws []string) error {
	script := redis.NewScript(`
for i = 1, #ARGV do
  redis.call('LREM', KEYS[2], 1, ARGV[i])
  redis.call('RPUSH', KEYS[1], ARGV[i])
end
return #ARGV
`)
	args := make([]interface{}, len(raws))
	for i, raw := range raws {
		args[i] = raw
	}
	return script.Run(ctx, client, []string{readyKey, processingKey}, args...).Err()
}

func moveMalformedHistoryToDead(ctx context.Context, client *redis.Client, processingKey, deadKey, raw string) error {
	script := redis.NewScript(`
local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
if removed > 0 then redis.call('RPUSH', KEYS[2], ARGV[1]) end
return removed
`)
	return script.Run(ctx, client, []string{processingKey, deadKey}, raw).Err()
}
