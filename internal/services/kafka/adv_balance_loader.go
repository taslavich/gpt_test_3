package kafka_service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

type advBalanceEvent struct {
	CampaignID        string   `json:"campaign_id"`
	UserID            string   `json:"user_id"`
	CampaignThreshold *float64 `json:"campaign_threshold"`
	UserThreshold     *float64 `json:"user_threshold"`
	Threshold         *float64 `json:"threshold"`
	BalanceThreshold  *float64 `json:"balance_threshold"`
}

func StartAdvBalanceRedisTicker(
	ctx context.Context,
	cfg config.KafkaConfig,
	redisAddr string,
	redisPassword string,
	poolSize int,
	minIdleConns int,
) error {
	userThresholdRedis, err := redisService.NewRedisClient(redisAddr, redisPassword, 5, poolSize, minIdleConns)
	if err != nil {
		return fmt.Errorf("init user threshold redis: %w", err)
	}
	userSpentRedis, err := redisService.NewRedisClient(redisAddr, redisPassword, 6, poolSize, minIdleConns)
	if err != nil {
		userThresholdRedis.Close()
		return fmt.Errorf("init user spent redis: %w", err)
	}
	userPlusReader, err := InitKafkaReader(cfg, cfg.KafkaTopicUserBalancePlus, cfg.KafkaGroupIDUserBalancePlus)
	if err != nil {
		closeRedisClients(userThresholdRedis, userSpentRedis)
		return fmt.Errorf("init user balance plus reader: %w", err)
	}

	go func() {
		defer closeRedisClients(userThresholdRedis, userSpentRedis)
		defer userPlusReader.Close()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				drainAdvBalanceReader(ctx, userPlusReader, func(event advBalanceEvent) error {
					threshold, ok := event.userThresholdValue()
					if !ok || event.UserID == "" {
						return nil
					}
					if err := userThresholdRedis.Set(ctx, event.UserID, strconv.FormatFloat(threshold, 'f', -1, 64), 0).Err(); err != nil {
						return err
					}
					return userSpentRedis.Set(ctx, event.UserID, "0", 0).Err()
				})
			}
		}
	}()

	return nil
}

func drainAdvBalanceReader(ctx context.Context, reader *kafka.Reader, handle func(advBalanceEvent) error) {
	for {
		readCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		message, err := reader.FetchMessage(readCtx)
		cancel()
		if err != nil {
			return
		}

		var event advBalanceEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			log.Printf("⚠️ failed to unmarshal ADV balance event: %v", err)
			_ = reader.CommitMessages(ctx, message)
			continue
		}

		if err := handle(event); err != nil {
			log.Printf("⚠️ failed to write ADV balance event to redis: %v", err)
			return
		}
		if err := reader.CommitMessages(ctx, message); err != nil {
			log.Printf("⚠️ failed to commit ADV balance event: %v", err)
			return
		}
	}
}

func (e advBalanceEvent) userThresholdValue() (float64, bool) {
	for _, value := range []*float64{e.UserThreshold, e.Threshold, e.BalanceThreshold} {
		if value != nil {
			return *value, true
		}
	}
	return 0, false
}

func (e advBalanceEvent) campaignThresholdValue() (float64, bool) {
	for _, value := range []*float64{e.CampaignThreshold, e.Threshold, e.BalanceThreshold} {
		if value != nil {
			return *value, true
		}
	}
	return 0, false
}

func closeRedisClients(clients ...*redis.Client) {
	for _, client := range clients {
		if client != nil {
			if err := client.Close(); err != nil {
				log.Printf("⚠️ failed to close ADV balance redis client: %v", err)
			}
		}
	}
}
