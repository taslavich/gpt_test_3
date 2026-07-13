package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrAlreadyApplied = errors.New("billing event already applied")
var ErrUncertain = errors.New("billing uncertain network error")

type Event struct {
	EventID, UserID, CampaignID, Format string
	Price                               float64
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
	if markerTTL <= 0 {
		markerTTL = 72 * time.Hour
	}
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
	if e.EventID == "" || e.UserID == "" || e.CampaignID == "" || e.Price <= 0 || math.IsNaN(e.Price) || math.IsInf(e.Price, 0) {
		return Result{}, fmt.Errorf("invalid billing event")
	}
	marker := "outbox:applied:" + e.EventID
	current := "pacing:current:" + e.CampaignID
	var res Result
	for i := 0; i < s.retries; i++ {
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			exists, err := tx.Exists(ctx, marker).Result()
			if err != nil {
				return err
			}
			if exists > 0 {
				res.AlreadyApplied = true
				return ErrAlreadyApplied
			}
			slot, err := tx.Get(ctx, current).Result()
			if errors.Is(err, redis.Nil) {
				slot = ""
			} else if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.IncrByFloat(ctx, "spent:user:"+e.UserID, e.Price)
				pipe.IncrByFloat(ctx, "spent:campaign:"+e.CampaignID, e.Price)
				if slot != "" {
					pipe.IncrByFloat(ctx, "pacing:spent:"+e.CampaignID+":"+slot, e.Price)
				}
				pipe.Set(ctx, marker, "1", s.markerTTL)
				return nil
			})
			res.SlotKey = slot
			return err
		}, marker, current)
		if err == nil || errors.Is(err, ErrAlreadyApplied) {
			return res, nil
		}
		if errors.Is(err, redis.TxFailedErr) {
			select {
			case <-ctx.Done():
				return res, ctx.Err()
			case <-time.After(s.backoff):
			}
			continue
		}
		return res, fmt.Errorf("%w: %v", ErrUncertain, err)
	}
	return res, redis.TxFailedErr
}
