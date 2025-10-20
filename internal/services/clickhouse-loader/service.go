package clickhouse_loader

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func ProcessKafkaMessages(ctx context.Context, broker, topic string, reader *kafka.Reader, chDB *sql.DB, table string, batchSize, timeoutSec int) (int, error) {
	passed, err := checkMessageCount(ctx, broker, topic, batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to check Kafka message count: %v", err)
	}

	if !passed {
		return 0, nil
	}

	var messages []kafka.Message
	var records []types.StatisticsRecord

	for i := 0; i < batchSize; i++ {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if err == context.DeadlineExceeded {
				break
			}
			return 0, err
		}

		var record types.StatisticsRecord
		if err := json.Unmarshal(msg.Value, &record); err != nil {
			log.Printf("⚠️ Failed to parse Kafka message: %v", err)
			continue
		}

		// Проверяем есть ли хотя бы одно непустое поле
		if hasData(record) {
			records = append(records, record)
			messages = append(messages, msg)
		} else {
			log.Printf("📭 Skipping empty message")
		}
	}

	if len(records) == 0 {
		return 0, nil
	}

	if err := InsertBatch(chDB, table, records); err != nil {
		return 0, fmt.Errorf("failed to insert batch: %v", err)
	}

	if err := reader.CommitMessages(ctx, messages...); err != nil {
		log.Printf("⚠️ Failed to commit Kafka offsets: %v", err)
	}

	log.Printf("✅ Successfully processed %d messages to ClickHouse", len(records))
	return len(records), nil
}

// Проверяет есть ли хотя бы одно непустое поле
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

func InsertBatch(chDB *sql.DB, table string, records []types.StatisticsRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Определяем все возможные колонки
	allColumns := []string{
		"uuid", "timestamp", "spp_domain", "bid_request", "geo_column",
		"bid_responses", "bid_response_winner", "bid_response_winner_by_dsp_price", "success",
	}

	// Строим один INSERT запрос для всех записей
	query := fmt.Sprintf(`
		INSERT INTO %s (%s) VALUES 
	`, table, strings.Join(allColumns, ", "))

	var valuePlaceholders []string
	var values []interface{}

	for _, record := range records {
		// Для каждой записи добавляем значения для всех колонок
		// Если поле пустое - используем пустую строку
		valuePlaceholders = append(valuePlaceholders, "(?, ?, ?, ?, ?, ?, ?, ?, ?)")

		values = append(values,
			coalesceEmpty(record.UUID),
			coalesceEmpty(record.TIMESTAMP),
			coalesceEmpty(record.SPP_DOMAIN),
			coalesceEmpty(record.BID_REQUEST),
			coalesceEmpty(record.GEO_COLUMN),
			coalesceEmpty(record.BID_RESPONSES),
			coalesceEmpty(record.BID_RESPONSE_WINNER),
			coalesceEmpty(record.BID_RESPONSE_WINNER_BY_DSP_PRICE),
			coalesceEmpty(record.SUCCESS),
		)
	}

	query += strings.Join(valuePlaceholders, ", ")

	// Выполняем один batch insert
	_, err := chDB.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("failed to insert batch: %v", err)
	}

	log.Printf("📊 Inserted %d records with single batch query", len(records))
	return nil
}

// Вспомогательная функция для обработки пустых значений
func coalesceEmpty(s string) string {
	if s == "" {
		return ""
	}
	return s
}

func CreateTable(chDB *sql.DB, tableName string) error {
	_, err := chDB.Exec(fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            uuid String,
            timestamp String,
			spp_domain String,
            bid_request String,
            geo_column String,
            bid_responses String,
            bid_response_winner String,
            bid_response_winner_by_dsp_price String,
            success String,
            updated_at DateTime DEFAULT now()
        ) ENGINE = ReplacingMergeTree(updated_at)
        ORDER BY (uuid, timestamp)
        SETTINGS index_granularity = 8192
    `, tableName))
	return err
}

func checkMessageCount(ctx context.Context, broker, topic string, minThreshold int) (bool, error) {
	conn, err := kafka.Dial("tcp", broker)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions()
	if err != nil {
		return false, err
	}

	totalMessages := 0
	for _, p := range partitions {
		if p.Topic != topic {
			continue
		}

		partitionConn, err := kafka.DialPartition(ctx, "tcp", broker, p)
		if err != nil {
			log.Printf("Cannot DialPartition: %v", err)
			continue
		}
		defer partitionConn.Close()

		first, err := partitionConn.ReadFirstOffset()
		if err != nil {
			log.Printf("Cannot ReadFirstOffset: %v", err)
			continue
		}
		last, err := partitionConn.ReadLastOffset()
		if err != nil {
			log.Printf("Cannot ReadLastOffset: %v", err)
			continue
		}
		totalMessages += int(last - first)
	}

	return totalMessages >= minThreshold, nil
}
