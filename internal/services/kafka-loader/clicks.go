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

func ProcessBatchClicks(ctx context.Context, redisClient *redis.Client, kafkaWriter *kafka.Writer, batchSize int64, setName string) error {
	if batchSize <= 0 {
		return nil
	}

	if setName == "" {
		return fmt.Errorf("redis set name is empty")
	}

	uuids, err := redisClient.SRandMemberN(ctx, setName, batchSize).Result()
	if err != nil {
		return fmt.Errorf("failed to get UUIDs from Redis set %q: %v", setName, err)
	}

	if len(uuids) < int(batchSize) {
		return nil
	}

	readPipe := redisClient.Pipeline()
	for _, uuid := range uuids {
		readPipe.HGetAll(ctx, uuid)
	}

	cmds, err := readPipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to get data from Redis: %v", err)
	}

	var kafkaMessages []kafka.Message
	uuidsToDelete := make([]string, 0, len(uuids))

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
			uuidsToDelete = append(uuidsToDelete, key)
		}
	}

	if len(kafkaMessages) > 0 {
		if err := kafkaWriter.WriteMessages(ctx, kafkaMessages...); err != nil {
			return fmt.Errorf("failed to write to Kafka: %v", err)
		}

		if err := redisClient.SRem(ctx, setName, stringSliceToAny(uuidsToDelete)...).Err(); err != nil {
			log.Printf("⚠️ Failed to remove UUIDs from set %q: %v", setName, err)
		}
	}

	if len(uuidsToDelete) > 0 {
		delPipe := redisClient.Pipeline()

		for _, uuid := range uuidsToDelete {
			delPipe.Del(ctx, uuid)
		}

		if _, err := delPipe.Exec(ctx); err != nil {
			log.Printf("⚠️ Failed to delete processed Redis records from set %q: %v", setName, err)
		}
	}

	return nil
}
