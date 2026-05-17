package redis_service

import (
	"github.com/redis/go-redis/v9"
)

type RedisClients struct {
	Ortb        *redis.Client
	Clicks      *redis.Client
	Impressions *redis.Client
}

func NewRedisClients(addr string, password string, ortbDb, impressionsDb, clicksDb int) *RedisClients {
	return &RedisClients{
		Ortb: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       ortbDb,
		}),

		Clicks: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       impressionsDb,
		}),

		Impressions: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       clicksDb,
		}),
	}
}
