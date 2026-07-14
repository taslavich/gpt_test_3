package auction

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	spentUserPrefix     = "spent:user:"
	spentCampaignPrefix = "spent:campaign:"
	pacingCurrentPrefix = "pacing:current:"
	pacingSpentPrefix   = "pacing:spent:"
)

type RuntimeStore struct {
	client     *redis.Client
	currentTTL time.Duration
	slotTTL    time.Duration
}

func NewRuntimeStore(client *redis.Client, currentTTL, slotTTL time.Duration) *RuntimeStore {
	if currentTTL <= 0 {
		currentTTL = 10 * time.Minute
	}
	if slotTTL <= 0 {
		slotTTL = 48 * time.Hour
	}
	return &RuntimeStore{client: client, currentTTL: currentTTL, slotTTL: slotTTL}
}

func (s *RuntimeStore) UserSpent(ctx context.Context, userID string) (float64, error) {
	return s.floatValue(ctx, spentUserPrefix+strings.TrimSpace(userID))
}

func (s *RuntimeStore) CampaignSpent(ctx context.Context, campaignID string) (float64, error) {
	return s.floatValue(ctx, spentCampaignPrefix+strings.TrimSpace(campaignID))
}

func (s *RuntimeStore) floatValue(ctx context.Context, key string) (float64, error) {
	if s == nil || s.client == nil {
		return 0, errors.New("ADV runtime Redis client is nil")
	}
	if strings.TrimSpace(key) == "" {
		return 0, errors.New("ADV runtime Redis key is empty")
	}
	raw, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("redis GET %s: %w", key, err)
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, fmt.Errorf("invalid Redis numeric value for %s: %q", key, raw)
	}
	return value, nil
}

func (s *RuntimeStore) PacingEligible(ctx context.Context, campaign *Campaign, now time.Time, campaignSpent float64) (bool, error) {
	if campaign == nil {
		return false, nil
	}
	if s == nil || s.client == nil {
		return false, errors.New("ADV runtime Redis client is nil")
	}
	currentKey := pacingCurrentPrefix + campaign.ID
	slotKey, err := s.client.Get(ctx, currentKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", currentKey, err)
	}
	if !strings.HasPrefix(slotKey, pacingSpentPrefix+campaign.ID+":") {
		return false, fmt.Errorf("invalid current pacing key for campaign %s", campaign.ID)
	}
	slotSpent, err := s.floatValue(ctx, slotKey)
	if err != nil {
		return false, err
	}
	slotTarget, err := pacingSlotTarget(campaign, now, campaignSpent, slotSpent)
	if err != nil {
		return false, err
	}
	if slotTarget <= 0 {
		return false, nil
	}
	return slotSpent < slotTarget, nil
}

func pacingSlotTarget(campaign *Campaign, now time.Time, campaignSpent, slotSpent float64) (float64, error) {
	if campaign == nil {
		return 0, nil
	}
	if campaignSpent < 0 || slotSpent < 0 || math.IsNaN(campaignSpent) || math.IsNaN(slotSpent) || math.IsInf(campaignSpent, 0) || math.IsInf(slotSpent, 0) {
		return 0, errors.New("invalid pacing spend values")
	}
	if slotSpent > campaignSpent+1e-9 {
		return 0, fmt.Errorf("current slot spend %.12f exceeds campaign spend %.12f", slotSpent, campaignSpent)
	}

	slotStart := now.UTC().Truncate(SlotDuration)
	activeSlotsFromSlotStart := ActiveSlotsLeft(campaign, slotStart)
	currentSlotFraction := CurrentSlotActiveFraction(campaign, now)
	spentBeforeCurrentSlot := campaignSpent - slotSpent
	remainingAtSlotStart := campaign.GoalTotalDollars - spentBeforeCurrentSlot
	if remainingAtSlotStart <= 0 || activeSlotsFromSlotStart <= 0 || currentSlotFraction <= 0 {
		return 0, nil
	}

	fullSlotTarget := remainingAtSlotStart / activeSlotsFromSlotStart
	return fullSlotTarget * currentSlotFraction, nil
}

func (s *RuntimeStore) UpdatePacing(ctx context.Context, snapshot *Snapshot, now time.Time) error {
	if s == nil || s.client == nil {
		return errors.New("ADV runtime Redis client is nil")
	}
	activeCurrentKeys := make(map[string]struct{})
	pipe := s.client.Pipeline()
	if snapshot != nil {
		for _, campaign := range snapshot.Campaigns {
			if campaign == nil {
				continue
			}
			currentKey := pacingCurrentPrefix + campaign.ID
			if !campaignActiveAt(campaign, now) {
				pipe.Del(ctx, currentKey)
				continue
			}
			slotID := now.UTC().Truncate(SlotDuration).Format("20060102T150405Z")
			slotKey := pacingSpentPrefix + campaign.ID + ":" + slotID
			activeCurrentKeys[currentKey] = struct{}{}
			pipe.Set(ctx, currentKey, slotKey, s.currentTTL)
			pipe.SetNX(ctx, slotKey, "0", s.slotTTL)
			pipe.Expire(ctx, slotKey, s.slotTTL)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("update pacing keys: %w", err)
	}
	return s.removeStaleCurrentKeys(ctx, activeCurrentKeys)
}

func (s *RuntimeStore) removeStaleCurrentKeys(ctx context.Context, active map[string]struct{}) error {
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, pacingCurrentPrefix+"*", 256).Result()
		if err != nil {
			return fmt.Errorf("scan pacing current keys: %w", err)
		}
		for _, key := range keys {
			if _, ok := active[key]; !ok {
				if err := s.client.Del(ctx, key).Err(); err != nil {
					return fmt.Errorf("delete stale pacing key %s: %w", key, err)
				}
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

type WinnerRecord struct {
	Price      float64
	UserID     string
	CampaignID string
	Format     string
}

type WinnerStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewWinnerStore(client *redis.Client, ttl time.Duration) *WinnerStore {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	return &WinnerStore{client: client, ttl: ttl}
}

func (s *WinnerStore) Put(ctx context.Context, winnerUUID string, record WinnerRecord) error {
	if s == nil || s.client == nil {
		return errors.New("ADV winner Redis client is nil")
	}
	winnerUUID = strings.TrimSpace(winnerUUID)
	if winnerUUID == "" || record.Price <= 0 || strings.TrimSpace(record.UserID) == "" || strings.TrimSpace(record.CampaignID) == "" || normalizeFormat(record.Format) == "" {
		return errors.New("invalid ADV winner record")
	}
	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, winnerUUID, map[string]any{
		"price":       strconv.FormatFloat(record.Price, 'f', -1, 64),
		"user_id":     record.UserID,
		"campaign_id": record.CampaignID,
		"format":      normalizeFormat(record.Format),
	})
	pipe.Expire(ctx, winnerUUID, s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write ADV winner %s: %w", winnerUUID, err)
	}
	return nil
}
