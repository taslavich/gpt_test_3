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

func ProcessBatchClicksWins(
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
			Name:                 "clicks_wins",
			BatchDurationLogName: "CLICKS_WINS",
			PopUUIDsName:         "click win UUIDs",
			HMGetDataName:        "click wins data",
			GetDataName:          "click win data",
			WriteMessagesName:    "click wins",
			SuccessLogName:       "Clicks Wins",
			HMGetFields: []string{
				constants.ORTB_UUID,
				constants.EVENT_TIME_CLICKS_WINS_COLUMN,
			},
			BuildMessage: buildClicksWinKafkaMessage,
		},
	)

	return err
}

func buildClicksWinKafkaMessage(
	shardID int,
	clicksWinsUUID string,
	values []interface{},
) (kafka.Message, bool, error) {
	rawRecord := types.ClicksWins{
		CLICKS_WINS_UUID:       clicksWinsUUID,
		ORTB_UUID:              valueAsString(values, 0),
		EVENT_TIME_CLICKS_WINS: valueAsString(values, 1),
	}

	if !HasDataClicksWins(rawRecord) {
		return kafka.Message{}, false, nil
	}

	record := &eventspb.ClicksWinEvent{
		ClicksWinsUuid:        rawRecord.CLICKS_WINS_UUID,
		OrtbUuid:              rawRecord.ORTB_UUID,
		EventTimeClicksWinsMs: parseUnixMsSafe(rawRecord.EVENT_TIME_CLICKS_WINS),
	}

	protoData, err := proto.Marshal(record)
	if err != nil {
		return kafka.Message{}, false, fmt.Errorf(
			"❌ shard %d: failed to marshal click win protobuf for UUID %s: %v",
			shardID,
			clicksWinsUUID,
			err,
		)
	}

	return kafka.Message{
		Key:   []byte(clicksWinsUUID),
		Value: protoData,
	}, true, nil
}
