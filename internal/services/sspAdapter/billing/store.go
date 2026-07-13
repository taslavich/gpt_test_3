package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

const maxTransactionRetries = 5

type Winner struct {
	Price      float64
	UserID     string
	CampaignID string
	Format     string
}

type Store struct {
	runtime   *redis.Client
	winners   *redis.Client
	markerTTL time.Duration
}

func NewStore(runtime, winners *redis.Client, markerTTL time.Duration) *Store {
	if markerTTL <= 0 {
		markerTTL = 720 * time.Hour
	}
	return &Store{runtime: runtime, winners: winners, markerTTL: markerTTL}
}

func (s *Store) ReadWinner(ctx context.Context, winnerUUID, expectedFormat string) (Winner, error) {
	if s == nil || s.winners == nil {
		return Winner{}, errors.New("ADV winner Redis client is nil")
	}
	winnerUUID = strings.TrimSpace(winnerUUID)
	if winnerUUID == "" {
		return Winner{}, errors.New("winner UUID is empty")
	}
	values, err := s.winners.HGetAll(ctx, winnerUUID).Result()
	if err != nil {
		return Winner{}, fmt.Errorf("read ADV winner: %w", err)
	}
	if len(values) == 0 {
		return Winner{}, redis.Nil
	}
	price, err := strconv.ParseFloat(strings.TrimSpace(values["price"]), 64)
	if err != nil || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return Winner{}, errors.New("ADV winner has invalid price")
	}
	format := normalizeFormat(values["format"])
	if format == "" || format != normalizeFormat(expectedFormat) {
		return Winner{}, fmt.Errorf("ADV winner format mismatch: got %s expected %s", format, expectedFormat)
	}
	winner := Winner{Price: price, UserID: strings.TrimSpace(values["user_id"]), CampaignID: strings.TrimSpace(values["campaign_id"]), Format: format}
	if winner.UserID == "" || winner.CampaignID == "" {
		return Winner{}, errors.New("ADV winner has empty user_id or campaign_id")
	}
	return winner, nil
}

func (s *Store) Apply(ctx context.Context, record outbox.Record) error {
	if s == nil || s.runtime == nil {
		return errors.New("ADV runtime Redis client is nil")
	}
	if strings.TrimSpace(record.EventID) == "" || strings.TrimSpace(record.UserID) == "" || strings.TrimSpace(record.CampaignID) == "" || record.Price <= 0 {
		return errors.New("invalid ADV billing event")
	}
	markerKey := "outbox:applied:" + record.EventID
	currentKey := "pacing:current:" + record.CampaignID
	watchKeys := []string{markerKey, currentKey}
	var lastErr error
	for attempt := 0; attempt < maxTransactionRetries; attempt++ {
		err := s.runtime.Watch(ctx, func(tx *redis.Tx) error {
			applied, err := tx.Exists(ctx, markerKey).Result()
			if err != nil {
				return err
			}
			if applied > 0 {
				return nil
			}
			slotKey, err := tx.Get(ctx, currentKey).Result()
			if errors.Is(err, redis.Nil) {
				slotKey = ""
			} else if err != nil {
				return err
			}
			if slotKey != "" && !strings.HasPrefix(slotKey, "pacing:spent:"+record.CampaignID+":") {
				return fmt.Errorf("invalid pacing current value %q", slotKey)
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.IncrByFloat(ctx, "spent:user:"+record.UserID, record.Price)
				pipe.IncrByFloat(ctx, "spent:campaign:"+record.CampaignID, record.Price)
				if slotKey != "" {
					pipe.IncrByFloat(ctx, slotKey, record.Price)
				}
				pipe.Set(ctx, markerKey, "1", s.markerTTL)
				return nil
			})
			return err
		}, watchKeys...)
		if err == nil {
			return nil
		}
		lastErr = err
		if !errors.Is(err, redis.TxFailedErr) {
			return fmt.Errorf("ADV billing transaction: %w", err)
		}
	}
	return fmt.Errorf("ADV billing transaction conflicted after retries: %w", lastErr)
}

func normalizeFormat(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if _, ok := constants.FormatToCodes[value]; ok {
		return value
	}
	return ""
}
