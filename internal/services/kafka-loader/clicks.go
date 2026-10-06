package kafka_loader

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	"google.golang.org/protobuf/proto"
)

const (
	clickIframeReportGrace = 2 * time.Second

	clickIframeStatusUnknown int32 = 0
	clickIframeStatusNo      int32 = 1
	clickIframeStatusYes     int32 = 2
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
				constants.IS_IFRAME_COLUMN,
			},
			ShouldDefer:  shouldDeferClickForIframeReport,
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
	iframeStatus := valueAsString(values, 3)

	rawRecord := types.Clicks{
		CLICKS_UUID:       clicks_uuid,
		ORTB_UUID:         ortb_uuid,
		EVENT_TIME_CLICKS: eventTime,
		FORMAT:            format,
		IS_IFRAME:         iframeStatus,
	}

	if !HasDataClicks(rawRecord) {
		return kafka.Message{}, false, nil
	}

	record := &eventspb.ClickEvent{
		ClicksUuid:        rawRecord.CLICKS_UUID,
		OrtbUuid:          rawRecord.ORTB_UUID,
		EventTimeClicksMs: parseUnixMsSafe(rawRecord.EVENT_TIME_CLICKS),
		Format:            rawRecord.FORMAT,
		IframeStatus:      clickIframeStatusProto(rawRecord.IS_IFRAME),
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
func shouldDeferClickForIframeReport(_ int, _ string, values []interface{}) bool {
	status := valueAsString(values, 3)
	if status == constants.FALSE || status == constants.TRUE {
		return false
	}

	rawEventTime := valueAsString(values, 1)
	eventTime, err := time.ParseInLocation("2006-01-02 15:04:05.000", rawEventTime, time.UTC)
	if err != nil {
		return false
	}

	age := time.Since(eventTime)
	return age >= 0 && age < clickIframeReportGrace
}

func clickIframeStatusProto(status string) int32 {
	switch status {
	case constants.FALSE:
		return clickIframeStatusNo
	case constants.TRUE:
		return clickIframeStatusYes
	default:
		return clickIframeStatusUnknown
	}
}
