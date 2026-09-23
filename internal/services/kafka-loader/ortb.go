package kafka_loader

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	"google.golang.org/protobuf/proto"
)

var ortbHMGetFields = []string{
	constants.EVENT_TIME_COLUMN,
	constants.TYPIC_COLUMN,
	constants.FORMAT_COLUMN,
	constants.SPP_DOMAIN_COLUMN,
	constants.GEO_COLUMN,
	constants.CITY_ID_COLUMN,
	constants.CODE_COLUMN,
	constants.BID_RESPONSES_COLUMN,
	constants.ADV_RTB_RESPONSES_COLUMN,
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
	constants.EXACT_SEGMENT_HASH_COLUMN,
	constants.SEGMENT_HASH_COLUMN,
	constants.PERCENTER_POINT_VERSION_COLUMN,
}

func ProcessBatchOrtb(
	ctx context.Context,
	redisClients []*redis.Client,
	kafkaWriter *kafka.Writer,
	batchSize int64,
	setName string,
) (int, error) {
	return processRedisKafkaBatch(
		ctx,
		redisClients,
		kafkaWriter,
		batchSize,
		setName,
		redisKafkaBatchConfig{
			Name:                 "ORTB",
			BatchDurationLogName: "ORTB",
			PopUUIDsName:         "UUIDs",
			HMGetDataName:        "data",
			GetDataName:          "data",
			WriteMessagesName:    "ORTB",
			SuccessLogName:       "ORTB",
			HMGetFields:          ortbHMGetFields,
			BuildMessage:         buildOrtbKafkaMessage,
		},
	)
}

func buildOrtbKafkaMessage(
	shardID int,
	uuid string,
	values []interface{},
) (kafka.Message, bool, error) {
	rawRecord := types.Ortb{
		UUID:                    uuid,
		EVENT_TIME:              valueAsString(values, 0),
		TYPIC:                   valueAsString(values, 1),
		FORMAT:                  valueAsString(values, 2),
		SPP_DOMAIN:              valueAsString(values, 3),
		GEO:                     valueAsString(values, 4),
		CITY_ID:                 valueAsString(values, 5),
		CODE:                    valueAsString(values, 6),
		BID_RESPONSES:           valueAsString(values, 7),
		ADV_RTB_RESPONSES:       valueAsString(values, 8),
		IP:                      valueAsString(values, 9),
		IPV6:                    valueAsString(values, 10),
		LANG:                    valueAsString(values, 11),
		BROWSER:                 valueAsString(values, 12),
		BROWSER_VERSION:         valueAsString(values, 13),
		OS:                      valueAsString(values, 14),
		OS_VERSION:              valueAsString(values, 15),
		DEVICE:                  valueAsString(values, 16),
		SITE_ID:                 valueAsString(values, 17),
		SITE_DOMAIN:             valueAsString(values, 18),
		BID_FLOOR:               valueAsString(values, 19),
		WIN_DSP_DOMAIN:          valueAsString(values, 20),
		WIN_PRICE:               valueAsString(values, 21),
		WIN_DSP_PRICE:           valueAsString(values, 22),
		WIN_CID:                 valueAsString(values, 23),
		WIN_CRID:                valueAsString(values, 24),
		WIN_USER_ID:             valueAsString(values, 25),
		EXACT_SEGMENT_HASH:      valueAsString(values, 26),
		SEGMENT_HASH:            valueAsString(values, 27),
		PERCENTER_POINT_VERSION: valueAsString(values, 28),
	}

	if !HasDataOrtb(rawRecord) {
		return kafka.Message{}, false, nil
	}

	bidResponses, err := parseBidResponsesFromRedis(values, 7)
	if err != nil {
		log.Printf("Ошибка парсинга bidResponses из Redis (index 7): %v", err)
		bidResponses = make(map[string]string)
	}

	advRTBResponses, err := parseBidResponsesFromRedis(values, 8)
	if err != nil {
		log.Printf("failed to parse ADV RTB responses from Redis (index 8): %v", err)
		advRTBResponses = make(map[string]string)
	}
	for campaignID, code := range advRTBResponses {
		bidResponses[constants.ADVRTBResponseStatsPrefix+campaignID] = code
	}
	if rawRecord.EXACT_SEGMENT_HASH != "" {
		bidResponses[constants.PercenterExactSegmentHashTransportKey] = rawRecord.EXACT_SEGMENT_HASH
	}
	if rawRecord.SEGMENT_HASH != "" {
		bidResponses[constants.PercenterSegmentHashTransportKey] = rawRecord.SEGMENT_HASH
	}
	if rawRecord.PERCENTER_POINT_VERSION != "" {
		bidResponses[constants.PercenterPointVersionTransportKey] = rawRecord.PERCENTER_POINT_VERSION
	}

	event := &eventspb.OrtbEvent{
		Uuid:           rawRecord.UUID,
		EventTimeMs:    parseUnixMsSafe(rawRecord.EVENT_TIME),
		Typic:          rawRecord.TYPIC,
		Format:         rawRecord.FORMAT,
		SppDomain:      rawRecord.SPP_DOMAIN,
		Geo:            rawRecord.GEO,
		CityId:         parseUint32Safe(rawRecord.CITY_ID),
		Code:           parseUint32Safe(rawRecord.CODE),
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
		return kafka.Message{}, false, fmt.Errorf("❌ shard %d: failed to marshal ORTB protobuf for UUID %s: %v", shardID, uuid, err)
	}

	return kafka.Message{
		Key:   []byte(uuid),
		Value: protoData,
	}, true, nil
}
