package pub

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

func (s *Server) RefreshSafety(ctx context.Context) error {
	check := func(ctx context.Context, r *redis.Client) error {
		v, e := r.ConfigGet(ctx, "maxmemory-policy").Result()
		if e != nil {
			return e
		}
		if v["maxmemory-policy"] != "noeviction" {
			return fmt.Errorf("publisher Redis requires maxmemory-policy=noeviction")
		}
		return nil
	}
	var e error
	switch r := s.Redis.(type) {
	case *redis.Client:
		e = check(ctx, r)
	case *redis.ClusterClient:
		e = r.ForEachMaster(ctx, check)
	default:
		e = fmt.Errorf("unsupported publisher Redis mode")
	}
	if e == nil {
		for _, client := range s.ORTB {
			if e = check(ctx, client); e != nil {
				break
			}
		}
	}
	if e != nil {
		s.safetyUntil.Store(0)
		return e
	}
	s.safetyUntil.Store(time.Now().Add(35 * time.Second).Unix())
	return nil
}
