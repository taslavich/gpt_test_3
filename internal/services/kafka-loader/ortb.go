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
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	"google.golang.org/protobuf/proto"
)

const redisCleanupChunkSize = 5000

var ortbHMGetFields = []string{
	constants.EVENT_TIME_COLUMN,
	constants.TYPIC_COLUMN,
	constants.FORMAT_COLUMN,
	constants.SPP_DOMAIN_COLUMN,
	constants.GEO_COLUMN,
	constants.CITY_ID_COLUMN,
	constants.BID_RESPONSES_COLUMN,
	constants.IP_COLUMN,
	constants.IPV6_COLUMN,
	constants.LANG_COLUMN,
	constants.BROWSER_COLUMN,
	constants.BROWSER_VERSION_COLUMN,
	constants.OS_COLUMN,
	constants.OS_VERSION_COLUMN,
	constants.DEVICE_COLUMN,
	constants.SITE_ID_COLUMN,
	constants.SITE_DOMAIN_COLUMN,
	constants.BID_FLOOR_COLUMN,
	constants.WIN_DSP_DOMAIN_COLUMN,
	constants.WIN_PRICE_COLUMN,
	constants.WIN_DSP_PRICE_COLUMN,
	constants.WIN_CID_COLUMN,
	constants.WIN_CRID_COLUMN,
	constants.WIN_USER_ID_COLUMN,
}

func ProcessBatchOrtb(
	ctx context.Context,
	redisClients []*redis.Client,
	kafkaWriter *kafka.Writer,
	batchSize int64,
	setName string,
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

	perShardLimit := kafkaLoaderSplitLimit(batchSize, len(redisClients))

	var wg sync.WaitGroup
	errCh := make(chan error, len(redisClients))
	var mu sync.Mutex
	totalProcessed := 0

	for shardID, redisClient := range redisClients {
		wg.Add(1)

		go func(shardID int, redisClient *redis.Client) {
			defer wg.Done()

			processed, err := processOrtbShard(
				ctx,
				shardID,
				redisClient,
				kafkaWriter,
				perShardLimit,
				setName,
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

func processOrtbShard(
	ctx context.Context,
	shardID int,
	redisClient *redis.Client,
	kafkaWriter *kafka.Writer,
	batchSize int64,
	setName string,
) (int, error) {
	processingSetName := fmt.Sprintf("%s:processing:%d", setName, shardID)

	uuids, err := popUUIDsToProcessing(ctx, redisClient, setName, processingSetName, batchSize)
	if err != nil {
		return 0, fmt.Errorf("shard %d: failed to pop UUIDs to processing: %w", shardID, err)
	}

	if len(uuids) == 0 {
		return 0, nil
	}

	readPipe := redisClient.Pipeline()
	cmds := make([]*redis.SliceCmd, 0, len(uuids))

	for _, uuid := range uuids {
		cmds = append(cmds, readPipe.HMGet(ctx, uuid, ortbHMGetFields...))
	}

	if _, err := readPipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("shard %d: failed to HMGET data from Redis: %w", shardID, err)
	}

	kafkaMessages := make([]kafka.Message, 0, len(uuids))
	uuidsToDelete := make([]string, 0, len(uuids))

	for i, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil {
			log.Printf("⚠️ shard %d: failed to get data for UUID %s: %v", shardID, uuids[i], err)
			continue
		}

		key := uuids[i]

		rawRecord := types.Ortb{
			UUID:            key,
			EVENT_TIME:      valueAsString(values, 0),
			TYPIC:           valueAsString(values, 1),
			FORMAT:          valueAsString(values, 2),
			SPP_DOMAIN:      valueAsString(values, 3),
			GEO:             valueAsString(values, 4),
			CITY_ID:         valueAsString(values, 5),
			BID_RESPONSES:   valueAsString(values, 6),
			IP:              valueAsString(values, 7),
			IPV6:            valueAsString(values, 8),
			LANG:            valueAsString(values, 9),
			BROWSER:         valueAsString(values, 10),
			BROWSER_VERSION: valueAsString(values, 11),
			OS:              valueAsString(values, 12),
			OS_VERSION:      valueAsString(values, 13),
			DEVICE:          valueAsString(values, 14),
			SITE_ID:         valueAsString(values, 15),
			SITE_DOMAIN:     valueAsString(values, 16),
			BID_FLOOR:       valueAsString(values, 17),
			WIN_DSP_DOMAIN:  valueAsString(values, 18),
			WIN_PRICE:       valueAsString(values, 19),
			WIN_DSP_PRICE:   valueAsString(values, 20),
			WIN_CID:         valueAsString(values, 21),
			WIN_CRID:        valueAsString(values, 22),
			WIN_USER_ID:     valueAsString(values, 23),
		}

		if !types.HasDataOrtb(rawRecord) {
			continue
		}

		bidResponses, err := parseBidResponsesFromRedis(values, 6)
		if err != nil {
			log.Printf("Ошибка парсинга bidResponses из Redis (index 6): %v", err)
			bidResponses = make(map[string]int32)
		}

		event := &eventspb.OrtbEvent{
			Uuid:           rawRecord.UUID,
			EventTimeMs:    parseUnixMsSafe(rawRecord.EVENT_TIME),
			Typic:          rawRecord.TYPIC,
			Format:         rawRecord.FORMAT,
			SppDomain:      rawRecord.SPP_DOMAIN,
			Geo:            rawRecord.GEO,
			CityId:         parseUint32Safe(rawRecord.CITY_ID),
			BidResponses:   bidResponses,
			Ip:             rawRecord.IP,
			Ipv6:           rawRecord.IPV6,
			Lang:           rawRecord.LANG,
			Browser:        rawRecord.BROWSER,
			BrowserVersion: rawRecord.BROWSER_VERSION,
			Os:             rawRecord.OS,
			OsVersion:      rawRecord.OS_VERSION,
			Device:         rawRecord.DEVICE,
			SiteId:         rawRecord.SITE_ID,
			SiteDomain:     rawRecord.SITE_DOMAIN,
			BidFloor:       parseFloat64Safe(rawRecord.BID_FLOOR),
			WinDspDomain:   rawRecord.WIN_DSP_DOMAIN,
			WinPrice:       parseFloat64Safe(rawRecord.WIN_PRICE),
			WinDspPrice:    parseFloat64Safe(rawRecord.WIN_DSP_PRICE),
			WinCid:         rawRecord.WIN_CID,
			WinCrid:        rawRecord.WIN_CRID,
			WinUserId:      rawRecord.WIN_USER_ID,
		}

		protoData, err := proto.Marshal(event)
		if err != nil {
			log.Printf("❌ shard %d: failed to marshal ORTB protobuf for UUID %s: %v", shardID, key, err)
			continue
		}

		kafkaMessages = append(kafkaMessages, kafka.Message{
			Key:   []byte(key),
			Value: protoData,
		})

		uuidsToDelete = append(uuidsToDelete, key)
	}

	if len(kafkaMessages) == 0 {
		restoreUUIDsFromProcessingToReady(ctx, redisClient, setName, processingSetName, uuids)
		return 0, nil
	}

	if len(kafkaMessages) > 0 {
		if err := kafkaWriter.WriteMessages(ctx, kafkaMessages...); err != nil {
			restoreUUIDsFromProcessingToReady(ctx, redisClient, setName, processingSetName, uuids)
			return 0, fmt.Errorf("shard %d: failed to write ORTB messages to Kafka: %w", shardID, err)
		}
	}

	cleanupProcessedRedisRecordsFromProcessing(ctx, redisClient, processingSetName, uuidsToDelete)

	log.Printf(
		"✅ ORTB shard %d processed: uuids=%d kafka_messages=%d",
		shardID,
		len(uuids),
		len(kafkaMessages),
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
		_ = redisClient.SAdd(ctx, readySetName, stringSliceToAny(uuids)...).Err()
		return nil, fmt.Errorf("failed to SADD to processing set %q: %w", processingSetName, err)
	}

	// Защита от вечного зависания processing-set.
	_ = redisClient.Expire(ctx, processingSetName, 10*time.Minute).Err()

	return uuids, nil
}

func cleanupProcessedRedisRecordsFromProcessing(
	ctx context.Context,
	redisClient *redis.Client,
	processingSetName string,
	uuids []string,
) {
	if len(uuids) == 0 {
		return
	}

	for start := 0; start < len(uuids); start += redisCleanupChunkSize {
		end := start + redisCleanupChunkSize
		if end > len(uuids) {
			end = len(uuids)
		}

		chunk := uuids[start:end]

		if err := redisClient.SRem(ctx, processingSetName, stringSliceToAny(chunk)...).Err(); err != nil {
			log.Printf("⚠️ failed to SREM UUIDs from processing set %q: %v", processingSetName, err)
		}

		if err := redisClient.Unlink(ctx, chunk...).Err(); err != nil {
			log.Printf("⚠️ failed to UNLINK processed Redis records: %v", err)
		}
	}
}

func restoreUUIDsFromProcessingToReady(
	ctx context.Context,
	redisClient *redis.Client,
	readySetName string,
	processingSetName string,
	uuids []string,
) {
	if len(uuids) == 0 {
		return
	}

	for start := 0; start < len(uuids); start += redisCleanupChunkSize {
		end := start + redisCleanupChunkSize
		if end > len(uuids) {
			end = len(uuids)
		}

		chunk := uuids[start:end]

		if err := redisClient.SAdd(ctx, readySetName, stringSliceToAny(chunk)...).Err(); err != nil {
			log.Printf("⚠️ failed to restore UUIDs to ready set %q: %v", readySetName, err)
		}

		if err := redisClient.SRem(ctx, processingSetName, stringSliceToAny(chunk)...).Err(); err != nil {
			log.Printf("⚠️ failed to remove restored UUIDs from processing set %q: %v", processingSetName, err)
		}
	}
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
		return 0
	}

	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}

	return uint32(v)
}

func parseFloat64Safe(s string) float64 {
	if s == "" {
		return 0
	}

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}

	return v
}

func parseUnixMsSafe(s string) int64 {
	if s == "" {
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

	return 0
}

func parseBidResponsesFromRedis(values []interface{}, index int) (map[string]int32, error) {
	raw := valueAsBytes(values, index)
	if len(raw) == 0 {
		return make(map[string]int32), nil
	}

	br := &eventspb.BidResponses{}
	if err := proto.Unmarshal(raw, br); err != nil {
		return nil, fmt.Errorf("unmarshal BidResponses: %w", err)
	}

	if br.Items == nil {
		return make(map[string]int32), nil
	}

	return br.Items, nil
}
