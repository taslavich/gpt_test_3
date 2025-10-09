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
		log.Println("PASSED")
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
	tx, err := chDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Динамически строим запрос на основе непустых полей
	for _, record := range records {
		columns, placeholders, values := buildDynamicQuery(record)
		if len(columns) == 0 {
			continue // Пропускаем полностью пустые записи
		}

		query := fmt.Sprintf(`
			INSERT INTO %s (%s) VALUES (%s)
		`, table, strings.Join(columns, ", "), strings.Join(placeholders, ", "))

		_, err := tx.ExecContext(context.Background(), query, values...)
		if err != nil {
			return fmt.Errorf("failed to insert record: %v", err)
		}
	}

	return tx.Commit()
}

// Строит динамический запрос на основе непустых полей
func buildDynamicQuery(record types.StatisticsRecord) ([]string, []string, []interface{}) {
	var columns []string
	var placeholders []string
	var values []interface{}

	if record.UUID != "" {
		columns = append(columns, "uuid")
		placeholders = append(placeholders, "?")
		values = append(values, record.UUID)
	}

	if record.TIMESTAMP != "" {
		columns = append(columns, "timestamp")
		placeholders = append(placeholders, "?")
		values = append(values, record.TIMESTAMP)
	}

	if record.SPP_DOMAIN != "" {
		columns = append(columns, "spp_domain")
		placeholders = append(placeholders, "?")
		values = append(values, record.SPP_DOMAIN)
	}

	if record.BID_REQUEST != "" {
		columns = append(columns, "bid_request")
		placeholders = append(placeholders, "?")
		values = append(values, record.BID_REQUEST)
	}

	if record.GEO_COLUMN != "" {
		columns = append(columns, "geo_column")
		placeholders = append(placeholders, "?")
		values = append(values, record.GEO_COLUMN)
	}

	if record.BID_RESPONSES != "" {
		columns = append(columns, "bid_responses")
		placeholders = append(placeholders, "?")
		values = append(values, record.BID_RESPONSES)
	}

	if record.BID_RESPONSE_WINNER != "" {
		columns = append(columns, "bid_response_winner")
		placeholders = append(placeholders, "?")
		values = append(values, record.BID_RESPONSE_WINNER)
	}

	if record.BID_RESPONSE_WINNER_BY_DSP_PRICE != "" {
		columns = append(columns, "bid_response_winner_by_dsp_price")
		placeholders = append(placeholders, "?")
		values = append(values, record.BID_RESPONSE_WINNER_BY_DSP_PRICE)
	}

	if record.SUCCESS != "" {
		columns = append(columns, "success")
		placeholders = append(placeholders, "?")
		values = append(values, record.SUCCESS)
	}

	return columns, placeholders, values
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

// Остальные функции без изменений...
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
