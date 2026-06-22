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

func ProcessBatchClicks(
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
			Name:              "clicks",
			PopUUIDsName:      "click UUIDs",
			HMGetDataName:     "clicks data",
			GetDataName:       "click data",
			WriteMessagesName: "clicks",
			SuccessLogName:    "Clicks",
			HMGetFields: []string{
				constants.EVENT_TIME_CLICKS_COLUMN,
				constants.FORMAT_COLUMN,
			},
			BuildMessage: buildClickKafkaMessage,
		},
	)

	return err
}

func buildClickKafkaMessage(
	shardID int,
	uuid string,
	values []interface{},
) (kafka.Message, bool, error) {
	eventTime := valueAsString(values, 0)
	format := valueAsString(values, 1)

	rawRecord := types.Clicks{
		UUID:              uuid,
		EVENT_TIME_CLICKS: eventTime,
		FORMAT:            format,
	}

	if !HasDataClicks(rawRecord) {
		return kafka.Message{}, false, nil
	}

	record := &eventspb.ClickEvent{
		Uuid:              rawRecord.UUID,
		EventTimeClicksMs: parseUnixMsSafe(rawRecord.EVENT_TIME_CLICKS),
		Format:            rawRecord.FORMAT,
	}

	protoData, err := proto.Marshal(record)
	if err != nil {
		return kafka.Message{}, false, fmt.Errorf("❌ shard %d: failed to marshal click protobuf for UUID %s: %v", shardID, uuid, err)
	}

	return kafka.Message{
		Key:   []byte(uuid),
		Value: protoData,
	}, true, nil
}
