package kafka_loader

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func ProcessBatchClicks(ctx context.Context, redisClient *redis.Client, kafkaWriter *kafka.Writer, batchSize int64) error {
	if batchSize <= 0 {
		return nil
	}

	allKeys, err := redisClient.Keys(ctx, "*").Result()
	if err != nil {
		return fmt.Errorf("failed to get keys: %v", err)
	}

	keysToProcess := len(allKeys)
	if int64(keysToProcess) > batchSize {
		keysToProcess = int(batchSize)
	}

	if keysToProcess == 0 {
		return nil
	}

	uuids := allKeys[:keysToProcess]

	pipe := redisClient.Pipeline()
	for _, uuid := range uuids {
		pipe.HGetAll(ctx, uuid)
	}

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to get data from Redis: %v", err)
	}

	var kafkaMessages []kafka.Message
	fieldsToDelete := make(map[string][]string)

	for i, cmd := range cmds {
		data, err := cmd.(*redis.MapStringStringCmd).Result()
		if err != nil {
			log.Printf("⚠️ Failed to get data for UUID %s: %v", uuids[i], err)
			continue
		}

		// Используем структуру вместо map
		record := types.Clicks{}
		key := uuids[i]

		if eventTimeClicks, exists := data[constants.EVENT_TIME_CLICKS_COLUMN]; exists {
			record.EVENT_TIME_CLICKS = eventTimeClicks
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.EVENT_TIME_CLICKS_COLUMN)
		}

		record.UUID = key

		// Проверяем есть ли данные в записи
		if types.HasDataClicks(record) {
			jsonData, err := json.Marshal(record)
			if err != nil {
				log.Printf("❌ Failed to marshal record for UUID %s: %v", uuids[i], err)
				continue
			}

			kafkaMessages = append(kafkaMessages, kafka.Message{
				Value: jsonData,
			})
		}
	}

	if len(kafkaMessages) > 0 {
		if err := kafkaWriter.WriteMessages(ctx, kafkaMessages...); err != nil {
			return fmt.Errorf("failed to write to Kafka: %v", err)
		}
	}

	if len(fieldsToDelete) > 0 {
		pipe := redisClient.Pipeline()

		for key, fields := range fieldsToDelete {
			pipe.HDel(ctx, key, fields...)
		}

		if _, err := pipe.Exec(ctx); err != nil {
			log.Printf("⚠️ Failed to delete some fields from Redis: %v", err)
		}
	}

	return nil
}
