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

func ProcessBatchOrtb(ctx context.Context, redisClient *redis.Client, kafkaWriter *kafka.Writer, batchSize int64) error {
	if batchSize <= 0 {
		return nil
	}

	allKeys, err := redisClient.Keys(ctx, "*").Result()
	if err != nil {
		return fmt.Errorf("failed to get keys: %v", err)
	}

	keysToProcess := len(allKeys)
	if int64(keysToProcess) > batchSize {
		keysToProcess = int(batchSize)
	}

	if keysToProcess == 0 {
		return nil
	}

	uuids := allKeys[:keysToProcess]

	pipe := redisClient.Pipeline()
	for _, uuid := range uuids {
		pipe.HGetAll(ctx, uuid)
	}

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to get data from Redis: %v", err)
	}

	var kafkaMessages []kafka.Message
	fieldsToDelete := make(map[string][]string)

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
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.EVENT_TIME_COLUMN)
		}

		if typic, exists := data[constants.TYPIC_COLUMN]; exists {
			record.TYPIC = typic
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.TYPIC_COLUMN)
		}

		if format, exists := data[constants.FORMAT_COLUMN]; exists {
			record.FORMAT = format
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.FORMAT_COLUMN)
		}

		if spp_domain, exists := data[constants.SPP_DOMAIN_COLUMN]; exists {
			record.SPP_DOMAIN = spp_domain
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.SPP_DOMAIN_COLUMN)
		}

		if geo, exists := data[constants.GEO_COLUMN]; exists {
			record.GEO = geo
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.GEO_COLUMN)
		}

		if cityId, exists := data[constants.CITY_ID_COLUMN]; exists {
			record.CITY_ID = cityId
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.CITY_ID_COLUMN)
		}

		if bidResponses, exists := data[constants.BID_RESPONSES_COLUMN]; exists {
			record.BID_RESPONSES = bidResponses
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.BID_RESPONSES_COLUMN)
		}

		if ip, exists := data[constants.IP_COLUMN]; exists {
			record.IP = ip
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.IP_COLUMN)
		}

		if ipv6, exists := data[constants.IPV6_COLUMN]; exists {
			record.IPV6 = ipv6
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.IPV6_COLUMN)
		}

		if lang, exists := data[constants.LANG_COLUMN]; exists {
			record.LANG = lang
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.LANG_COLUMN)
		}

		if browser, exists := data[constants.BROWSER_COLUMN]; exists {
			record.BROWSER = browser
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.BROWSER_COLUMN)
		}

		if browserVersion, exists := data[constants.BROWSER_VERSION_COLUMN]; exists {
			record.BROWSER_VERSION = browserVersion
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.BROWSER_VERSION_COLUMN)
		}

		if os, exists := data[constants.OS_COLUMN]; exists {
			record.OS = os
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.OS_COLUMN)
		}

		if osVersion, exists := data[constants.OS_VERSION_COLUMN]; exists {
			record.OS_VERSION = osVersion
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.OS_VERSION_COLUMN)
		}

		if device, exists := data[constants.DEVICE_COLUMN]; exists {
			record.DEVICE = device
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.DEVICE_COLUMN)
		}

		if siteId, exists := data[constants.SITE_ID_COLUMN]; exists {
			record.SITE_ID = siteId
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.SITE_ID_COLUMN)
		}

		if siteDomain, exists := data[constants.SITE_DOMAIN_COLUMN]; exists {
			record.SITE_DOMAIN = siteDomain
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.SITE_DOMAIN_COLUMN)
		}

		if bidFloor, exists := data[constants.BID_FLOOR_COLUMN]; exists {
			record.BID_FLOOR = bidFloor
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.BID_FLOOR_COLUMN)
		}

		if winDspDomain, exists := data[constants.WIN_DSP_DOMAIN_COLUMN]; exists {
			record.WIN_DSP_DOMAIN = winDspDomain
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.WIN_DSP_DOMAIN_COLUMN)
		}

		if winPrice, exists := data[constants.WIN_PRICE_COLUMN]; exists {
			record.WIN_PRICE = winPrice
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.WIN_PRICE_COLUMN)
		}

		if winDspPrice, exists := data[constants.WIN_DSP_PRICE_COLUMN]; exists {
			record.WIN_DSP_PRICE = winDspPrice
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.WIN_DSP_PRICE_COLUMN)
		}

		if winCid, exists := data[constants.WIN_CID_COLUMN]; exists {
			record.WIN_CID = winCid
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.WIN_CID_COLUMN)
		}

		if winCrid, exists := data[constants.WIN_CRID_COLUMN]; exists {
			record.WIN_CRID = winCrid
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.WIN_CRID_COLUMN)
		}

		if winUserId, exists := data[constants.WIN_USER_ID_COLUMN]; exists {
			record.WIN_USER_ID = winUserId
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.WIN_USER_ID_COLUMN)
		}

		if winFlag, exists := data[constants.WIN_FLAG_COLUMN]; exists {
			record.WIN_FLAG = winFlag
			fieldsToDelete[key] = append(fieldsToDelete[key], constants.WIN_FLAG_COLUMN)
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
		}
	}

	if len(kafkaMessages) > 0 {
		if err := kafkaWriter.WriteMessages(ctx, kafkaMessages...); err != nil {
			return fmt.Errorf("failed to write to Kafka: %v", err)
		}
	}

	if len(fieldsToDelete) > 0 {
		pipe := redisClient.Pipeline()

		for key, fields := range fieldsToDelete {
			pipe.HDel(ctx, key, fields...)
		}

		if _, err := pipe.Exec(ctx); err != nil {
			log.Printf("⚠️ Failed to delete some fields from Redis: %v", err)
		}
	}

	return nil
}
