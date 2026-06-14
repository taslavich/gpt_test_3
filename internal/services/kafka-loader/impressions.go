package kafka_loader

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	"google.golang.org/protobuf/proto"
)

func ProcessBatchImpressions(
	ctx context.Context,
	redisClients []*redis.Client,
	kafkaWriter *kafka.Writer,
	batchSize int64,
	setName string,
) error {
	if batchSize <= 0 {
		return nil
	}

	if setName == "" {
		return fmt.Errorf("redis set name is empty")
	}

	if len(redisClients) == 0 {
		return fmt.Errorf("redis clients list is empty")
	}

	perShardLimit := kafkaLoaderSplitLimit(batchSize, len(redisClients))

	var wg sync.WaitGroup
	errCh := make(chan error, len(redisClients))

	for shardID, redisClient := range redisClients {
		wg.Add(1)

		go func(shardID int, redisClient *redis.Client) {
			defer wg.Done()

			if err := processImpressionsShard(
				ctx,
				shardID,
				redisClient,
				kafkaWriter,
				perShardLimit,
				setName,
			); err != nil {
				errCh <- err
			}
		}(shardID, redisClient)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func processImpressionsShard(
	ctx context.Context,
	shardID int,
	redisClient *redis.Client,
	kafkaWriter *kafka.Writer,
	batchSize int64,
	setName string,
) error {
	processingSetName := fmt.Sprintf("%s:processing:%d", setName, shardID)

	uuids, err := popUUIDsToProcessing(ctx, redisClient, setName, processingSetName, batchSize)
	if err != nil {
		return fmt.Errorf("shard %d: failed to pop impression UUIDs to processing: %w", shardID, err)
	}

	if len(uuids) == 0 {
		return nil
	}

	readPipe := redisClient.Pipeline()
	cmds := make([]*redis.SliceCmd, 0, len(uuids))

	for _, uuid := range uuids {
		cmds = append(cmds, readPipe.HMGet(ctx, uuid, constants.EVENT_TIME_IMPRESSIONS_COLUMN, constants.FORMAT_COLUMN))
	}

	if _, err := readPipe.Exec(ctx); err != nil {
		return fmt.Errorf("shard %d: failed to HMGET impressions data from Redis: %w", shardID, err)
	}

	kafkaMessages := make([]kafka.Message, 0, len(uuids))
	uuidsToDelete := make([]string, 0, len(uuids))

	for i, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil {
			log.Printf("⚠️ shard %d: failed to get impression data for UUID %s: %v", shardID, uuids[i], err)
			continue
		}

		key := uuids[i]
		eventTime := valueAsString(values, 0)
		format := valueAsString(values, 1)

		rawRecord := types.Impressions{
			UUID:                   key,
			EVENT_TIME_IMPRESSIONS: eventTime,
			FORMAT:                 format,
		}

		if !types.HasDataImpressions(rawRecord) {
			continue
		}

		record := &eventspb.ImpressionEvent{
			Uuid:                   rawRecord.UUID,
			EventTimeImpressionsMs: parseUnixMsSafe(rawRecord.EVENT_TIME_IMPRESSIONS),
			Format:                 rawRecord.FORMAT,
		}

		protoData, err := proto.Marshal(record)
		if err != nil {
			log.Printf("❌ shard %d: failed to marshal impression protobuf for UUID %s: %v", shardID, key, err)
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
		return nil
	}

	if len(kafkaMessages) > 0 {
		if err := kafkaWriter.WriteMessages(ctx, kafkaMessages...); err != nil {
			restoreUUIDsFromProcessingToReady(ctx, redisClient, setName, processingSetName, uuids)

			return fmt.Errorf(
				"shard %d: failed to write impressions messages to Kafka: messages=%d err=%s",
				shardID,
				len(kafkaMessages),
				compactKafkaWriteError(err),
			)
		}
	}

	cleanupProcessedRedisRecordsFromProcessing(ctx, redisClient, processingSetName, uuidsToDelete)

	log.Printf(
		"✅ Impressions shard %d processed: uuids=%d kafka_messages=%d",
		shardID,
		len(uuids),
		len(kafkaMessages),
	)

	return nil
}
