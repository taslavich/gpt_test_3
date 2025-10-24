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

	// Убираем вызов GiveBatch и напрямую вставляем batch
	if err := insertBatch(chDB, table, records); err != nil {
		return 0, fmt.Errorf("failed to insert batch: %v", err)
	}

	if err := reader.CommitMessages(ctx, messages...); err != nil {
		log.Printf("⚠️ Failed to commit Kafka offsets: %v", err)
	}

	log.Printf("✅ Successfully processed %d messages to ClickHouse", len(records))
	return len(records), nil
}

// Убираем функцию GiveBatch и mergeRecords, так как они больше не нужны

func insertBatch(chDB *sql.DB, table string, records []types.StatisticsRecord) error {
	if len(records) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (uuid, timestamp, spp_domain, bid_request, geo_column, bid_responses, bid_response_winner, success)
		VALUES 
	`, table)

	var valuePlaceholders []string
	var values []interface{}

	for _, record := range records {
		valuePlaceholders = append(valuePlaceholders, "(?, ?, ?, ?, ?, ?, ?, ?)")

		// ПРАВИЛЬНАЯ конвертация SUCCESS string в bool
		var successBool bool
		if record.SUCCESS == "1" {
			successBool = true
		} else {
			successBool = false // "0", "", или любое другое значение = false
		}

		values = append(values,
			coalesceEmpty(record.UUID),
			coalesceEmpty(record.TIMESTAMP),
			coalesceEmpty(record.SPP_DOMAIN),
			coalesceEmpty(record.BID_REQUEST),
			coalesceEmpty(record.GEO_COLUMN),
			coalesceEmpty(record.BID_RESPONSES),
			coalesceEmpty(record.BID_RESPONSE_WINNER),
			successBool,
		)
	}

	query += strings.Join(valuePlaceholders, ", ")

	_, err := chDB.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("failed to insert batch: %v", err)
	}

	log.Printf("📊 Inserted %d records with single batch query", len(records))
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
            success Bool,
            _version UInt64 MATERIALIZED toUnixTimestamp64Milli(now64())
        ) ENGINE = ReplacingMergeTree(_version)
        ORDER BY uuid
        PRIMARY KEY uuid
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
