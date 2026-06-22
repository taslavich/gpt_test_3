package kafka_loader

import (
	"context"
	"fmt"

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
	_, err := processRedisKafkaBatch(
		ctx,
		redisClients,
		kafkaWriter,
		batchSize,
		setName,
		redisKafkaBatchConfig{
			Name:              "impressions",
			PopUUIDsName:      "impression UUIDs",
			HMGetDataName:     "impressions data",
			GetDataName:       "impression data",
			WriteMessagesName: "impressions",
			SuccessLogName:    "Impressions",
			HMGetFields: []string{
				constants.EVENT_TIME_IMPRESSIONS_COLUMN,
				constants.FORMAT_COLUMN,
			},
			BuildMessage: buildImpressionKafkaMessage,
		},
	)

	return err
}

func buildImpressionKafkaMessage(
	shardID int,
	uuid string,
	values []interface{},
) (kafka.Message, bool, error) {
	eventTime := valueAsString(values, 0)
	format := valueAsString(values, 1)

	rawRecord := types.Impressions{
		UUID:                   uuid,
		EVENT_TIME_IMPRESSIONS: eventTime,
		FORMAT:                 format,
	}

	if !HasDataImpressions(rawRecord) {
		return kafka.Message{}, false, nil
	}

	record := &eventspb.ImpressionEvent{
		Uuid:                   rawRecord.UUID,
		EventTimeImpressionsMs: parseUnixMsSafe(rawRecord.EVENT_TIME_IMPRESSIONS),
		Format:                 rawRecord.FORMAT,
	}

	protoData, err := proto.Marshal(record)
	if err != nil {
		return kafka.Message{}, false, fmt.Errorf("❌ shard %d: failed to marshal impression protobuf for UUID %s: %v", shardID, uuid, err)
	}

	return kafka.Message{
		Key:   []byte(uuid),
		Value: protoData,
	}, true, nil
}
