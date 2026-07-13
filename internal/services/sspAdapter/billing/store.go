package billing

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Event struct {
	EventID, UserID, CampaignID string
	Price                       float64
}
type Result struct {
	AlreadyApplied bool
	SlotKey        string
}
type Store struct {
	client    *redis.Client
	markerTTL time.Duration
	retries   int
	backoff   time.Duration
}

func NewStore(client *redis.Client, markerTTL time.Duration, retries int, backoff time.Duration) *Store {
	if retries <= 0 {
		retries = 3
	}
	if backoff <= 0 {
		backoff = 20 * time.Millisecond
	}
	return &Store{client: client, markerTTL: markerTTL, retries: retries, backoff: backoff}
}
func (s *Store) Apply(ctx context.Context, e Event) (Result, error) {
	if s.client == nil {
		return Result{}, fmt.Errorf("redis client is nil")
	}
	if e.EventID == "" || e.UserID == "" || e.CampaignID == "" || e.Price <= 0 {
		return Result{}, fmt.Errorf("invalid billing event")
	}
	applied := "outbox:applied:" + e.EventID
	current := "pacing:current:" + e.CampaignID
	var res Result
	for i := 0; i < s.retries; i++ {
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			exists, err := tx.Exists(ctx, applied).Result()
			if err != nil {
				return err
			}
			if exists > 0 {
				res.AlreadyApplied = true
				return nil
			}
			slot, err := tx.Get(ctx, current).Result()
			if errors.Is(err, redis.Nil) {
				slot = ""
			} else if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				price := strconv.FormatFloat(e.Price, 'f', -1, 64)
				pipe.IncrByFloat(ctx, "spent:user:"+e.UserID, e.Price)
				pipe.IncrByFloat(ctx, "spent:campaign:"+e.CampaignID, e.Price)
				if slot != "" {
					pipe.IncrByFloat(ctx, slot, e.Price)
				}
				pipe.Set(ctx, applied, "1", s.markerTTL)
				_ = price
				return nil
			})
			res.SlotKey = slot
			return err
		}, applied, current)
		if err == nil {
			return res, nil
		}
		if errors.Is(err, redis.TxFailedErr) {
			time.Sleep(s.backoff)
			continue
		}
		return res, err
	}
	return res, redis.TxFailedErr
}
