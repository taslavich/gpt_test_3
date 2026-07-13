package auction

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RuntimeStore struct {
	client  *redis.Client
	slotTTL time.Duration
}

func NewRuntimeStore(client *redis.Client, slotTTL time.Duration) *RuntimeStore {
	if slotTTL <= 0 {
		slotTTL = 15 * time.Minute
	}
	return &RuntimeStore{client: client, slotTTL: slotTTL}
}
func (s *RuntimeStore) getFloat(ctx context.Context, key string) (float64, error) {
	if s == nil || s.client == nil {
		return 0, nil
	}
	raw, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("malformed redis float %s=%q: %w", key, raw, err)
	}
	return v, nil
}
func (s *RuntimeStore) UserSpent(ctx context.Context, userID string) (float64, error) {
	return s.getFloat(ctx, "spent:user:"+userID)
}
func (s *RuntimeStore) CampaignSpent(ctx context.Context, campaignID string) (float64, error) {
	return s.getFloat(ctx, "spent:campaign:"+campaignID)
}
func (s *RuntimeStore) SlotSpent(ctx context.Context, campaignID, slotID string) (float64, error) {
	return s.getFloat(ctx, "pacing:spent:"+campaignID+":"+slotID)
}
func (s *RuntimeStore) CurrentSlot(ctx context.Context, campaignID string) (string, error) {
	if s == nil || s.client == nil {
		return "", nil
	}
	raw, err := s.client.Get(ctx, "pacing:current:"+campaignID).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return raw, nil
}
func (s *RuntimeStore) SetCurrentSlot(ctx context.Context, campaignID, slotID string) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Set(ctx, "pacing:current:"+campaignID, slotID, s.slotTTL).Err()
}
func (s *RuntimeStore) ClearCurrentSlot(ctx context.Context, campaignID string) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Del(ctx, "pacing:current:"+campaignID).Err()
}
func SlotID(t time.Time) string { return t.UTC().Truncate(SlotDuration).Format("20060102T150405Z") }
