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

	return len(records), nil
}

func insertBatch(chDB *sql.DB, table string, records []types.StatisticsRecord) error {
	if len(records) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (uuid, timestamp, typic, spp_domain, bid_request, geo_column, bid_responses, bid_response_winner, adm_ip, adm)
		VALUES 
	`, table)

	var valuePlaceholders []string
	var values []interface{}

	for _, record := range records {
		valuePlaceholders = append(valuePlaceholders, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")

		var admBool bool
		if record.ADM == "1" {
			admBool = true
		} else {
			admBool = false // "0", "", или любое другое значение = false
		}

		values = append(values,
			coalesceEmpty(record.UUID),
			coalesceEmpty(record.TIMESTAMP),
			coalesceEmpty(record.TYPIC),
			coalesceEmpty(record.SPP_DOMAIN),
			coalesceEmpty(record.BID_REQUEST),
			coalesceEmpty(record.GEO_COLUMN),
			coalesceEmpty(record.BID_RESPONSES),
			coalesceEmpty(record.BID_RESPONSE_WINNER),
			coalesceEmpty(record.ADM_IP),
			admBool,
		)
	}

	query += strings.Join(valuePlaceholders, ", ")

	_, err := chDB.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("failed to insert batch: %v", err)
	}

	return nil
}

// Проверяет есть ли хотя бы одно непустое поле
func hasData(record types.StatisticsRecord) bool {
	return record.BID_REQUEST != "" ||
		record.GEO_COLUMN != "" ||
		record.BID_RESPONSES != "" ||
		record.BID_RESPONSE_WINNER != "" ||
		record.ADM_IP != "" ||
		record.ADM != "" ||
		record.UUID != "" ||
		record.TIMESTAMP != "" ||
		record.SPP_DOMAIN != "" ||
		record.TYPIC != ""
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
			typic String,
            spp_domain String,
            bid_request String,
            geo_column String,
            bid_responses String,
            bid_response_winner String,
            adm_ip String,
			adm Bool,
            updated_at DateTime64(3) DEFAULT now64(),
			INDEX idx_updated_at_minmax updated_at TYPE minmax GRANULARITY 1
        ) ENGINE = ReplacingMergeTree(updated_at)
        ORDER BY (uuid, updated_at)
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
