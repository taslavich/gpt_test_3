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

	if err := GiveBatch(chDB, table, records); err != nil {
		return 0, fmt.Errorf("failed to insert batch: %v", err)
	}

	if err := reader.CommitMessages(ctx, messages...); err != nil {
		log.Printf("⚠️ Failed to commit Kafka offsets: %v", err)
	}

	log.Printf("✅ Successfully processed %d messages to ClickHouse", len(records))
	return len(records), nil
}

func GiveBatch(chDB *sql.DB, table string, records []types.StatisticsRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Разделяем записи на INSERT и UPDATE
	var insertRecords []types.StatisticsRecord
	var updateRecords []types.StatisticsRecord

	for _, record := range records {
		if record.SUCCESS == "1" && record.UUID != "" {
			updateRecords = append(updateRecords, record)
		} else {
			insertRecords = append(insertRecords, record)
		}
	}

	// Выполняем INSERT для обычных записей
	if len(insertRecords) > 0 {
		if err := insertBatch(chDB, table, insertRecords); err != nil {
			return fmt.Errorf("failed to insert records: %v", err)
		}
	}

	// Выполняем UPDATE только для поля success
	if len(updateRecords) > 0 {
		if err := updateSuccessBatch(chDB, table, updateRecords); err != nil {
			return fmt.Errorf("failed to update success records: %v", err)
		}
	}

	log.Printf("📊 Processed %d records (%d inserted, %d success updated)",
		len(records), len(insertRecords), len(updateRecords))
	return nil
}

func updateSuccessBatch(chDB *sql.DB, table string, records []types.StatisticsRecord) error {
	// Группируем обновления по UUID для избежания дубликатов
	uuidUpdates := make(map[string]string)
	for _, record := range records {
		if record.UUID != "" && record.SUCCESS == "1" {
			uuidUpdates[record.UUID] = record.SUCCESS
		}
	}

	// Выполняем UPDATE для каждого UUID
	for uuid, success := range uuidUpdates {
		query := fmt.Sprintf(`
			ALTER TABLE %s 
			UPDATE 
				success = ?
			WHERE uuid = ?
		`, table)

		_, err := chDB.Exec(query, success, uuid)
		if err != nil {
			log.Printf("⚠️ Failed to update success for UUID %s: %v", uuid, err)
			continue
		}
	}

	log.Printf("🔄 Updated success for %d unique records", len(uuidUpdates))
	return nil
}

// Проверяет есть ли хотя бы одно непустое поле
func hasData(record types.StatisticsRecord) bool {
	return record.BID_REQUEST != "" ||
		record.GEO_COLUMN != "" ||
		record.BID_RESPONSES != "" ||
		record.BID_RESPONSE_WINNER != "" ||
		record.SUCCESS != "" ||
		record.UUID != "" ||
		record.TIMESTAMP != "" ||
		record.SPP_DOMAIN != ""
}

func insertBatch(chDB *sql.DB, table string, records []types.StatisticsRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Определяем все возможные колонки
	allColumns := []string{
		"uuid", "timestamp", "spp_domain", "bid_request", "geo_column",
		"bid_responses", "bid_response_winner", "success",
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
		valuePlaceholders = append(valuePlaceholders, "(?, ?, ?, ?, ?, ?, ?, ?)")

		values = append(values,
			coalesceEmpty(record.UUID),
			coalesceEmpty(record.TIMESTAMP),
			coalesceEmpty(record.SPP_DOMAIN),
			coalesceEmpty(record.BID_REQUEST),
			coalesceEmpty(record.GEO_COLUMN),
			coalesceEmpty(record.BID_RESPONSES),
			coalesceEmpty(record.BID_RESPONSE_WINNER),
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
            uuid UUID,
            timestamp DateTime64(3),
			spp_domain String,
            bid_request String,
            geo_column String,
            bid_responses String,
            bid_response_winner String,
            success Bool
        ) ORDER BY (uuid, timestamp)
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
