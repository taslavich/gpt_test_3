package kafka_loader

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func ProcessBatchOrtb(ctx context.Context, redisClient *redis.Client, kafkaWriter *kafka.Writer, batchSize int64, setName string) error {
	if batchSize <= 0 {
		return nil
	}

	if setName == "" {
		return fmt.Errorf("redis set name is empty")
	}

	uuids, err := redisClient.SRandMemberN(ctx, setName, batchSize).Result()
	if err != nil {
		return fmt.Errorf("failed to get UUIDs from Redis set %q: %v", setName, err)
	}

	if len(uuids) == 0 {
		return nil
	}

	readPipe := redisClient.Pipeline()
	for _, uuid := range uuids {
		readPipe.HGetAll(ctx, uuid)
	}

	cmds, err := readPipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to get data from Redis: %v", err)
	}

	var kafkaMessages []kafka.Message
	uuidsToDelete := make([]string, 0, len(uuids))

	for i, cmd := range cmds {
		data, err := cmd.(*redis.MapStringStringCmd).Result()
		if err != nil {
			log.Printf("⚠️ Failed to get data for UUID %s: %v", uuids[i], err)
			continue
		}

		// Используем структуру вместо map
		record := types.Ortb{}
		key := uuids[i]

		if timestamp, exists := data[constants.EVENT_TIME_COLUMN]; exists {
			record.EVENT_TIME = timestamp
		}

		if typic, exists := data[constants.TYPIC_COLUMN]; exists {
			record.TYPIC = typic
		}

		if format, exists := data[constants.FORMAT_COLUMN]; exists {
			record.FORMAT = format
		}

		if spp_domain, exists := data[constants.SPP_DOMAIN_COLUMN]; exists {
			record.SPP_DOMAIN = spp_domain
		}

		if geo, exists := data[constants.GEO_COLUMN]; exists {
			record.GEO = geo
		}

		if cityId, exists := data[constants.CITY_ID_COLUMN]; exists {
			record.CITY_ID = cityId
		}

		if bidResponses, exists := data[constants.BID_RESPONSES_COLUMN]; exists {
			record.BID_RESPONSES = bidResponses
		}

		if ip, exists := data[constants.IP_COLUMN]; exists {
			record.IP = ip
		}

		if ipv6, exists := data[constants.IPV6_COLUMN]; exists {
			record.IPV6 = ipv6
		}

		if lang, exists := data[constants.LANG_COLUMN]; exists {
			record.LANG = lang
		}

		if browser, exists := data[constants.BROWSER_COLUMN]; exists {
			record.BROWSER = browser
		}

		if browserVersion, exists := data[constants.BROWSER_VERSION_COLUMN]; exists {
			record.BROWSER_VERSION = browserVersion
		}

		if os, exists := data[constants.OS_COLUMN]; exists {
			record.OS = os
		}

		if osVersion, exists := data[constants.OS_VERSION_COLUMN]; exists {
			record.OS_VERSION = osVersion
		}

		if device, exists := data[constants.DEVICE_COLUMN]; exists {
			record.DEVICE = device
		}

		if siteId, exists := data[constants.SITE_ID_COLUMN]; exists {
			record.SITE_ID = siteId
		}

		if siteDomain, exists := data[constants.SITE_DOMAIN_COLUMN]; exists {
			record.SITE_DOMAIN = siteDomain
		}

		if bidFloor, exists := data[constants.BID_FLOOR_COLUMN]; exists {
			record.BID_FLOOR = bidFloor
		}

		if winDspDomain, exists := data[constants.WIN_DSP_DOMAIN_COLUMN]; exists {
			record.WIN_DSP_DOMAIN = winDspDomain
		}

		if winPrice, exists := data[constants.WIN_PRICE_COLUMN]; exists {
			record.WIN_PRICE = winPrice
		}

		if winDspPrice, exists := data[constants.WIN_DSP_PRICE_COLUMN]; exists {
			record.WIN_DSP_PRICE = winDspPrice
		}

		if winCid, exists := data[constants.WIN_CID_COLUMN]; exists {
			record.WIN_CID = winCid
		}

		if winCrid, exists := data[constants.WIN_CRID_COLUMN]; exists {
			record.WIN_CRID = winCrid
		}

		if winUserId, exists := data[constants.WIN_USER_ID_COLUMN]; exists {
			record.WIN_USER_ID = winUserId
		}

		if winFlag, exists := data[constants.WIN_FLAG_COLUMN]; exists {
			record.WIN_FLAG = winFlag
		}

		record.UUID = key

		// Проверяем есть ли данные в записи
		if types.HasDataOrtb(record) {
			jsonData, err := json.Marshal(record)
			if err != nil {
				log.Printf("❌ Failed to marshal record for UUID %s: %v", uuids[i], err)
				continue
			}

			kafkaMessages = append(kafkaMessages, kafka.Message{
				Value: jsonData,
			})
			uuidsToDelete = append(uuidsToDelete, key)
		}
	}

	if len(kafkaMessages) > 0 {
		if err := kafkaWriter.WriteMessages(ctx, kafkaMessages...); err != nil {
			return fmt.Errorf("failed to write to Kafka: %v", err)
		}

		if err := redisClient.SRem(ctx, setName, stringSliceToAny(uuidsToDelete)...).Err(); err != nil {
			log.Printf("⚠️ Failed to remove UUIDs from set %q: %v", setName, err)
		}
	}

	if len(uuidsToDelete) > 0 {
		delPipe := redisClient.Pipeline()

		for _, uuid := range uuidsToDelete {
			delPipe.Del(ctx, uuid)
		}

		if _, err := delPipe.Exec(ctx); err != nil {
			log.Printf("⚠️ Failed to delete processed Redis records from set %q: %v", setName, err)
		}
	}

	return nil
}
