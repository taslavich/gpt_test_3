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
			Name:                 "clicks",
			BatchDurationLogName: "CLICKS",
			PopUUIDsName:         "click UUIDs",
			HMGetDataName:        "clicks data",
			GetDataName:          "click data",
			WriteMessagesName:    "clicks",
			SuccessLogName:       "Clicks",
			HMGetFields: []string{
				constants.ORTB_UUID,
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
	clicks_uuid string,
	values []interface{},
) (kafka.Message, bool, error) {
	ortb_uuid := valueAsString(values, 0)
	eventTime := valueAsString(values, 1)
	format := valueAsString(values, 2)

	rawRecord := types.Clicks{
		CLICKS_UUID:       clicks_uuid,
		ORTB_UUID:         ortb_uuid,
		EVENT_TIME_CLICKS: eventTime,
		FORMAT:            format,
	}

	if !HasDataClicks(rawRecord) {
		return kafka.Message{}, false, nil
	}

	record := &eventspb.ClickEvent{
		ClicksUuid:        rawRecord.CLICKS_UUID,
		OrtbUuid:          rawRecord.ORTB_UUID,
		EventTimeClicksMs: parseUnixMsSafe(rawRecord.EVENT_TIME_CLICKS),
		Format:            rawRecord.FORMAT,
	}

	protoData, err := proto.Marshal(record)
	if err != nil {
		return kafka.Message{}, false, fmt.Errorf("❌ shard %d: failed to marshal click protobuf for UUID %s: %v", shardID, clicks_uuid, err)
	}

	return kafka.Message{
		Key:   []byte(clicks_uuid),
		Value: protoData,
	}, true, nil
}
