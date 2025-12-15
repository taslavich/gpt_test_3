package utils

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const RedisKeyTTL = 5 * time.Minute

func WriteStringToRedis(ctx context.Context, redisClient *redis.Client, uuid, column string, data string, logged bool) error {
	log.Printf("DEBUG WriteStringToRedis: logged=%v, uuid=%s, column=%s, data=%s", logged, uuid, column, data)

	if !logged {
		log.Printf("DEBUG: Skipping Redis write (logged=false)")
		return nil
	}

	pipe := redisClient.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL) // Ключ умрет через 2 минуты
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to write string to Redis: %v", err)
	}
	return nil
}

func WriteUint32ToRedis(ctx context.Context, redisClient *redis.Client, uuid, column string, data uint32, logged bool) error {
	log.Printf("DEBUG WriteStringToRedis: logged=%v, uuid=%s, column=%s, data=%s", logged, uuid, column, data)

	if !logged {
		log.Printf("DEBUG: Skipping Redis write (logged=false)")
		return nil
	}

	pipe := redisClient.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL) // Ключ умрет через 2 минуты
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to write string to Redis: %v", err)
	}
	return nil
}

func WriteJsonToRedis(ctx context.Context, redisClient *redis.Client, uuid, column string, data []byte, logged bool) error {
	log.Printf("DEBUG WriteStringToRedis: logged=%v, uuid=%s, column=%s, data=%s", logged, uuid, column, data)

	if !logged {
		log.Printf("DEBUG: Skipping Redis write (logged=false)")
		return nil
	}

	pipe := redisClient.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL) // Ключ умрет через 2 минуты
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to write JSON to Redis: %v", err)
	}
	return nil
}

func WriteFloat32ToRedis(ctx context.Context, redisClient *redis.Client, uuid, column string, data float32, logged bool) error {
	log.Printf("DEBUG WriteStringToRedis: logged=%v, uuid=%s, column=%s, data=%s", logged, uuid, column, data)

	if !logged {
		log.Printf("DEBUG: Skipping Redis write (logged=false)")
		return nil
	}

	pipe := redisClient.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL) // Ключ умрет через 2 минуты
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to write JSON to Redis: %v", err)
	}
	return nil
}
