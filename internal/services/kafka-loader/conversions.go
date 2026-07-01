package kafka_loader

/*
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
				constants.PAYOUT,
			},
			BuildMessage: buildConversionKafkaMessage,
		},
	)

	return err
}

func buildConversionKafkaMessage(
	shardID int,
	conversion_uuid string,
	values []interface{},
) (kafka.Message, bool, error) {
	payout := valueAsString(values, 0)

	rawRecord := types.Conversions{
		CONVERSIONS_UUID: conversion_uuid,
		PAYOUT:           payout,
	}

	if !HasDataConversions(rawRecord) {
		return kafka.Message{}, false, nil
	}

	record := &eventspb.ConversionEvent{
		ConversionsUuid: rawRecord.CONVERSIONS_UUID,
		Payout:          rawRecord.PAYOUT,
	}

	protoData, err := proto.Marshal(record)
	if err != nil {
		return kafka.Message{}, false, fmt.Errorf("❌ shard %d: failed to marshal conversion protobuf for UUID %s: %v", shardID, conversion_uuid, err)
	}

	return kafka.Message{
		Key:   []byte(conversion_uuid),
		Value: protoData,
	}, true, nil
}
*/
