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

func ProcessBatchConversions(
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
			Name:                 "conversions",
			BatchDurationLogName: "CONVERSIONS",
			PopUUIDsName:         "conversion UUIDs",
			HMGetDataName:        "conversions data",
			GetDataName:          "conversion data",
			WriteMessagesName:    "conversions",
			SuccessLogName:       "Conversions",
			HMGetFields: []string{
				constants.CLICKS_UUID,
				constants.PAYOUT,
				constants.STATUS,
				constants.CONVERSIONS_EVENT_TIME_COLUMN,
			},
			BuildMessage: buildConversionKafkaMessage,
		},
	)

	return err
}

func buildConversionKafkaMessage(
	shardID int,
	conversionsUUID string,
	values []interface{},
) (kafka.Message, bool, error) {
	clicksUuid := valueAsString(values, 0)
	payout := valueAsString(values, 1)
	status := valueAsString(values, 2)
	conversionEventTime := valueAsString(values, 3)

	rawRecord := types.Conversions{
		CONVERSIONS_UUID:       conversionsUUID,
		CLICKS_UUID:            clicksUuid,
		PAYOUT:                 payout,
		STATUS:                 status,
		CONVERSIONS_EVENT_TIME: conversionEventTime,
	}

	if !HasDataConversions(rawRecord) {
		return kafka.Message{}, false, nil
	}

	record := &eventspb.ConversionEvent{
		ConversionsUuid: rawRecord.CONVERSIONS_UUID,
		ClicksUuid:      rawRecord.CLICKS_UUID,
		Payout:          rawRecord.PAYOUT,
		Status:          rawRecord.STATUS,
		ConversionsEventTimeMs: parseUnixMsSafe(
			rawRecord.CONVERSIONS_EVENT_TIME,
		),
	}

	protoData, err := proto.Marshal(record)
	if err != nil {
		return kafka.Message{}, false, fmt.Errorf(
			"❌ shard %d: failed to marshal conversion protobuf for UUID %s: %v",
			shardID,
			conversionsUUID,
			err,
		)
	}

	return kafka.Message{
		Key:   []byte(conversionsUUID),
		Value: protoData,
	}, true, nil
}
