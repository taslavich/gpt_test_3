package kafka_loader

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	"google.golang.org/protobuf/proto"
)

const redisCleanupChunkSize = 5000

type redisKafkaBatchConfig struct {
	Name                 string
	BatchDurationLogName string
	PopUUIDsName         string
	HMGetDataName        string
	GetDataName          string
	WriteMessagesName    string
	SuccessLogName       string
	HMGetFields          []string
	BuildMessage         func(shardID int, uuid string, values []interface{}) (kafka.Message, bool, error)
}

func processRedisKafkaBatch(
	ctx context.Context,
	redisClients []*redis.Client,
	kafkaWriter *kafka.Writer,
	batchSize int64,
	setName string,
	cfg redisKafkaBatchConfig,
) (int, error) {
	if batchSize <= 0 {
		return 0, nil
	}

	if setName == "" {
		return 0, fmt.Errorf("redis set name is empty")
	}

	if len(redisClients) == 0 {
		return 0, fmt.Errorf("redis clients list is empty")
	}

	if cfg.BatchDurationLogName != "" {
		start := time.Now()
		defer func() {
			log.Printf("⏱️ %s batch took %s", cfg.BatchDurationLogName, time.Since(start))
		}()
	}

	perShardLimit := kafkaLoaderSplitLimit(batchSize, len(redisClients))

	var wg sync.WaitGroup
	errCh := make(chan error, len(redisClients))
	var mu sync.Mutex
	totalProcessed := 0

	for shardID, redisClient := range redisClients {
		wg.Add(1)

		go func(shardID int, redisClient *redis.Client) {
			defer wg.Done()

			processed, err := processRedisKafkaShard(
				ctx,
				shardID,
				redisClient,
				kafkaWriter,
				perShardLimit,
				setName,
				cfg,
			)
			if err != nil {
				errCh <- err
				return
			}

			mu.Lock()
			totalProcessed += processed
			mu.Unlock()
		}(shardID, redisClient)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return totalProcessed, err
		}
	}

	return totalProcessed, nil
}

func processRedisKafkaShard(
	ctx context.Context,
	shardID int,
	redisClient *redis.Client,
	kafkaWriter *kafka.Writer,
	batchSize int64,
	setName string,
	cfg redisKafkaBatchConfig,
) (int, error) {
	processingSetName := fmt.Sprintf("%s:processing:%d", setName, shardID)

	uuids, err := popUUIDsToProcessing(ctx, redisClient, setName, processingSetName, batchSize)
	if err != nil {
		return 0, fmt.Errorf("shard %d: failed to pop %s to processing: %w", shardID, cfg.PopUUIDsName, err)
	}

	if len(uuids) == 0 {
		return 0, nil
	}

	readPipe := redisClient.Pipeline()
	cmds := make([]*redis.SliceCmd, 0, len(uuids))

	for _, uuid := range uuids {
		cmds = append(cmds, readPipe.HMGet(ctx, uuid, cfg.HMGetFields...))
	}

	if _, err := readPipe.Exec(ctx); err != nil {
		newCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if restoreErr := restoreUUIDsFromProcessingToReady(newCtx, redisClient, setName, processingSetName, uuids); restoreErr != nil {
			return 0, fmt.Errorf("shard %d: failed to write %s messages to Kafka: err=%s", shardID, cfg.WriteMessagesName, compactKafkaWriteError(err))
		}

		return 0, fmt.Errorf("shard %d: failed to HMGET %s from Redis: %w", shardID, cfg.HMGetDataName, err)
	}

	kafkaMessages := make([]kafka.Message, 0, len(uuids))
	uuidsToDelete := make([]string, 0, len(uuids))
	emptyCount := 0

	for i, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil {
			newCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if restoreErr := restoreUUIDsFromProcessingToReady(newCtx, redisClient, setName, processingSetName, uuids); restoreErr != nil {
				return 0, fmt.Errorf("shard %d: failed to write %s messages to Kafka: messages=%d err=%s", shardID, cfg.WriteMessagesName, len(kafkaMessages), compactKafkaWriteError(err))
			}

			return 0, fmt.Errorf("⚠️ shard %d: failed to get %s for UUID %s: %v", shardID, cfg.GetDataName, uuids[i], err)
		}

		key := uuids[i]

		message, shouldSend, err := cfg.BuildMessage(shardID, key, values)
		if err != nil {
			newCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if restoreErr := restoreUUIDsFromProcessingToReady(newCtx, redisClient, setName, processingSetName, uuids); restoreErr != nil {
				return 0, fmt.Errorf("shard %d: failed to write %s messages to Kafka: messages=%d err=%s", shardID, cfg.WriteMessagesName, len(kafkaMessages), compactKafkaWriteError(err))
			}

			return 0, err
		}

		if !shouldSend {
			emptyCount++
			uuidsToDelete = append(uuidsToDelete, key)
			continue
		}

		kafkaMessages = append(kafkaMessages, message)
		uuidsToDelete = append(uuidsToDelete, key)
	}

	if len(kafkaMessages) == 0 {
		if err := cleanupProcessedRedisRecordsFromProcessing(ctx, redisClient, processingSetName, uuidsToDelete); err != nil {
			return 0, err
		}

		log.Printf(
			"✅ %s shard %d processed: uuids=%d kafka_messages=0 empty=%d",
			cfg.SuccessLogName,
			shardID,
			len(uuids),
			emptyCount,
		)
		return 0, nil
	}

	if len(kafkaMessages) > 0 {
		if err := kafkaWriter.WriteMessages(ctx, kafkaMessages...); err != nil {
			newCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if restoreErr := restoreUUIDsFromProcessingToReady(newCtx, redisClient, setName, processingSetName, uuids); restoreErr != nil {
				return 0, fmt.Errorf("shard %d: failed to write %s messages to Kafka: messages=%d err=%s", shardID, cfg.WriteMessagesName, len(kafkaMessages), compactKafkaWriteError(err))
			}

			return 0, fmt.Errorf(
				"shard %d: failed to write %s messages to Kafka: messages=%d err=%s",
				shardID,
				cfg.WriteMessagesName,
				len(kafkaMessages),
				compactKafkaWriteError(err),
			)
		}
	}

	if err := cleanupProcessedRedisRecordsFromProcessing(ctx, redisClient, processingSetName, uuidsToDelete); err != nil {
		return 0, err
	}

	log.Printf(
		"✅ %s shard %d processed: uuids=%d kafka_messages=%d empty=%d",
		cfg.SuccessLogName,
		shardID,
		len(uuids),
		len(kafkaMessages),
		emptyCount,
	)

	return len(kafkaMessages), nil
}

func kafkaLoaderSplitLimit(total int64, shardCount int) int64 {
	if total <= 0 {
		return 0
	}

	if shardCount <= 0 {
		return total
	}

	limit := total / int64(shardCount)
	if total%int64(shardCount) != 0 {
		limit++
	}

	if limit <= 0 {
		return 1
	}

	return limit
}

func valueAsString(values []interface{}, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}

	if values[index] == nil {
		return ""
	}

	switch v := values[index].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func stringSliceToAny(slice []string) []interface{} {
	result := make([]interface{}, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

func popUUIDsToProcessing(
	ctx context.Context,
	redisClient *redis.Client,
	readySetName string,
	processingSetName string,
	batchSize int64,
) ([]string, error) {
	if batchSize <= 0 {
		return nil, nil
	}

	uuids, err := redisClient.SPopN(ctx, readySetName, batchSize).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to SPOP from ready set %q: %w", readySetName, err)
	}

	if len(uuids) == 0 {
		return nil, nil
	}

	if err := redisClient.SAdd(ctx, processingSetName, stringSliceToAny(uuids)...).Err(); err != nil {
		// Возвращаем обратно в ready, потому что не смогли положить в processing.
		if restoreErr := redisClient.SAdd(ctx, readySetName, stringSliceToAny(uuids)...).Err(); restoreErr != nil {
			return nil, fmt.Errorf("failed to restore UUIDs to ready set %q: %w", readySetName, restoreErr)
		}

		return nil, fmt.Errorf("failed to SADD to processing set %q: %w", processingSetName, err)
	}

	// Защита от вечного зависания processing-set.
	if err := redisClient.Expire(ctx, processingSetName, 10*time.Minute).Err(); err != nil {
		return nil, fmt.Errorf("failed to set expiration for processing set %q: %w", processingSetName, err)
	}

	return uuids, nil
}

func cleanupProcessedRedisRecordsFromProcessing(
	ctx context.Context,
	redisClient *redis.Client,
	processingSetName string,
	uuids []string,
) error {
	if len(uuids) == 0 {
		return nil
	}

	for start := 0; start < len(uuids); start += redisCleanupChunkSize {
		end := start + redisCleanupChunkSize
		if end > len(uuids) {
			end = len(uuids)
		}

		chunk := uuids[start:end]

		if err := redisClient.SRem(ctx, processingSetName, stringSliceToAny(chunk)...).Err(); err != nil {
			return fmt.Errorf("⚠️ failed to SREM UUIDs from processing set %q: %v", processingSetName, err)
		}

		if err := redisClient.Unlink(ctx, chunk...).Err(); err != nil {
			return fmt.Errorf("⚠️ failed to UNLINK processed Redis records: %v", err)
		}
	}

	return nil
}

func restoreUUIDsFromProcessingToReady(
	ctx context.Context,
	redisClient *redis.Client,
	readySetName string,
	processingSetName string,
	uuids []string,
) error {
	if len(uuids) == 0 {
		return nil
	}

	for start := 0; start < len(uuids); start += redisCleanupChunkSize {
		end := start + redisCleanupChunkSize
		if end > len(uuids) {
			end = len(uuids)
		}

		chunk := uuids[start:end]

		if err := redisClient.SAdd(ctx, readySetName, stringSliceToAny(chunk)...).Err(); err != nil {
			return fmt.Errorf("⚠️ failed to restore UUIDs to ready set %q: %v", readySetName, err)
		}

		if err := redisClient.SRem(ctx, processingSetName, stringSliceToAny(chunk)...).Err(); err != nil {
			return fmt.Errorf("⚠️ failed to remove restored UUIDs from processing set %q: %v", processingSetName, err)
		}
	}

	return nil
}

func valueAsBytes(values []interface{}, index int) []byte {
	if index < 0 || index >= len(values) {
		return nil
	}

	if values[index] == nil {
		return nil
	}

	switch v := values[index].(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		return []byte(fmt.Sprint(v))
	}
}

func parseUint32Safe(s string) uint32 {
	if s == "" {
		log.Printf("parseUint32Safe: empty string, returning 0")
		return 0
	}

	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		log.Printf("parseUint32Safe: failed to parse %q as uint32: %v, returning 0", s, err)
		return 0
	}

	return uint32(v)
}

func parseFloat64Safe(s string) float64 {
	if s == "" {
		//log.Printf("parseFloat64Safe: empty string, returning 0")
		return 0
	}

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("parseFloat64Safe: failed to parse %q as float64: %v, returning 0", s, err)
		return 0
	}

	return v
}

func parseUnixMsSafe(s string) int64 {
	if s == "" {
		log.Printf("parseUnixMsSafe: empty string, returning 0")
		return 0
	}

	// Если в Redis уже лежит unix ms строкой.
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v
	}

	layouts := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UnixMilli()
		}
	}

	log.Printf("Cannot parseUnixMsSafe: unrecognized time format: %q", s)

	return time.Now().UnixMilli()
}

func parseBidResponsesFromRedis(values []interface{}, index int) (map[string]string, error) {
	raw := valueAsBytes(values, index)
	if len(raw) == 0 {
		return make(map[string]string), nil
	}

	br := &eventspb.BidResponses{}
	if err := proto.Unmarshal(raw, br); err != nil {
		return nil, fmt.Errorf("unmarshal BidResponses: %w", err)
	}

	if br.Items == nil {
		return make(map[string]string), nil
	}

	return br.Items, nil
}

func compactKafkaWriteError(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()

	const maxLen = 500
	if len(msg) <= maxLen {
		return msg
	}

	return msg[:maxLen] + "... [truncated]"
}

func HasDataClicks(record types.Clicks) bool {
	return record.CLICKS_UUID != "" && record.ORTB_UUID != ""
}

func HasDataConversions(record types.Conversions) bool {
	return record.CONVERSIONS_UUID != ""
}

func HasDataClicksWins(record types.ClicksWins) bool {
	return record.CLICKS_WINS_UUID != "" && record.ORTB_UUID != ""
}

func HasDataImpressions(record types.Impressions) bool {
	if record.IMPRESSIONS_UUID == "" {
		log.Println("HasDataImpressions: impression UUID is empty")
		return false
	}

	if record.ORTB_UUID == "" {
		log.Println("HasDataImpressions: ORTB UUID is empty")
		return false
	}

	return true
}

func HasDataOrtb(record types.Ortb) bool {
	return record.UUID != ""
}
