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
			DB:       clicksDb,
		}),

		Impressions: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       impressionsDb,
		}),
	}
}

func NewRedisImpClicksClients(addr string, password string, impressionsDb, clicksDb int) (*redis.Client, *redis.Client) {
	return redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       impressionsDb,
		}),
		redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       clicksDb,
		})
}
