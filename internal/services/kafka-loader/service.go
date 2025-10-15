package kafka_loader

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func ProcessBatch(ctx context.Context, redisClient *redis.Client, kafkaWriter *kafka.Writer, batchSize int64) error {
	totalKeys, err := redisClient.DBSize(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to get total keys count: %v", err)
	}

	if totalKeys < batchSize*2 {
		log.Printf("PASSED, got %d", totalKeys)
		return nil
	}

	allKeys, err := redisClient.Keys(ctx, "stats:*").Result()
	if err != nil {
		return fmt.Errorf("failed to get stats keys: %v", err)
	}

	uuids := allKeys[:batchSize]

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
		record := types.StatisticsRecord{}
		key := uuids[i]

		if bidRequest, exists := data[constants.BID_REQUEST_COLUMN]; exists {
			record.BID_REQUEST = bidRequest
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.BID_REQUEST_COLUMN)
		}

		if geoColumn, exists := data[constants.GEO_COLUMN]; exists {
			record.GEO_COLUMN = geoColumn
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.GEO_COLUMN)
		}

		if bidResponses, exists := data[constants.BID_RESPONSES_COLUMN]; exists {
			record.BID_RESPONSES = bidResponses
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.BID_RESPONSES_COLUMN)
		}

		if bidResponseWinner, exists := data[constants.BID_RESPONSE_WINNER_COLUMN]; exists {
			record.BID_RESPONSE_WINNER = bidResponseWinner
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.BID_RESPONSE_WINNER_COLUMN)
		}

		if bidResponseWinnerByDspPrice, exists := data[constants.BID_RESPONSE_WINNER_BY_DSP_PRICE_COLUMN]; exists {
			record.BID_RESPONSE_WINNER_BY_DSP_PRICE = bidResponseWinnerByDspPrice
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.BID_RESPONSE_WINNER_BY_DSP_PRICE_COLUMN)
		}

		if success, exists := data[constants.RESULT_COLUMN]; exists {
			record.SUCCESS = success
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.RESULT_COLUMN)
		}

		if timestamp, exists := data[constants.TIMESTAMP_COLUMN]; exists {
			record.TIMESTAMP = timestamp
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.TIMESTAMP_COLUMN)
		}

		if spp_domain, exists := data[constants.SPP_DOMAIN_COLUMN]; exists {
			record.SPP_DOMAIN = spp_domain
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.SPP_DOMAIN_COLUMN)
		}

		record.UUID = key
		fieldsToDelete[key] = append(fieldsToDelete[key], constants.UUID_COLUMN)

		// Проверяем есть ли данные в записи
		if hasData(record) {
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

// Вспомогательная функция для проверки наличия данных
func hasData(record types.StatisticsRecord) bool {
	return record.BID_REQUEST != "" ||
		record.GEO_COLUMN != "" ||
		record.BID_RESPONSES != "" ||
		record.BID_RESPONSE_WINNER != "" ||
		record.BID_RESPONSE_WINNER_BY_DSP_PRICE != "" ||
		record.SUCCESS != "" ||
		record.UUID != "" ||
		record.TIMESTAMP != "" ||
		record.SPP_DOMAIN != ""
}

func EnsureTopicExists(broker string, topic string) error {
	var conn *kafka.Conn
	var err error

	conn, err = kafka.Dial("tcp", broker)
	if err != nil {
		return fmt.Errorf("⚠️ Failed to connect to broker %s: %v", broker, err)
	}
	log.Printf("✅ Connected to Kafka broker: %s", broker)
	defer conn.Close()

	if conn == nil {
		return fmt.Errorf("failed to connect to any Kafka broker: %v", broker)
	}

	// Получаем контроллера (ведущий брокер)
	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %v", err)
	}

	// Подключаемся к контроллеру для создания темы
	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %v", err)
	}
	defer controllerConn.Close()

	// Получаем список тем
	partitions, err := controllerConn.ReadPartitions()
	if err != nil {
		return fmt.Errorf("failed to read partitions: %v", err)
	}

	// Проверяем существует ли тема
	for _, p := range partitions {
		if p.Topic == topic {
			log.Printf("✅ Topic %s already exists", topic)
			return nil
		}
	}

	retentionMs := 5 * 60 * 60 * 1000 // 5 часов в миллисекундах

	configs := []kafka.ConfigEntry{
		{
			ConfigName:  "retention.ms",
			ConfigValue: fmt.Sprintf("%d", retentionMs),
		},
		{
			ConfigName:  "cleanup.policy",
			ConfigValue: "delete",
		},
	}

	// Создаем тему если не существует
	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
			ConfigEntries:     configs,
		},
	}

	err = controllerConn.CreateTopics(topicConfigs...)
	if err != nil {
		return fmt.Errorf("failed to create topic: %v", err)
	}

	log.Printf("✅ Created topic: %s with %d partitions", topic, 3)

	// Даем время для создания темы
	time.Sleep(2 * time.Second)
	return nil
}
