package utils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	redis_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
)

const RedisKeyTTL = 50 * time.Minute

// WriteStringToRedis записывает строку в Redis, автоматически выбирая шард по uuid
func WriteStringToRedis(ctx context.Context, redisClients []*redis.Client, uuid, column string, data string, logged bool) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, uuid)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", uuid, err)
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write string to Redis (uuid=%s, column=%s, shard=%d): %w", uuid, column, idx, err)
	}
	return nil
}

// WriteUint32ToRedis записывает uint32 в Redis, автоматически выбирая шард по uuid
func WriteUint32ToRedis(ctx context.Context, redisClients []*redis.Client, uuid, column string, data uint32, logged bool) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, uuid)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", uuid, err)
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write uint32 to Redis (uuid=%s, column=%s, shard=%d): %w", uuid, column, idx, err)
	}
	return nil
}

func WriteBytesToRedis(ctx context.Context, redisClients []*redis.Client, uuid, column string, data []byte, logged bool) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, uuid)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", uuid, err)
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write bytes to Redis (uuid=%s, column=%s, shard=%d): %w", uuid, column, idx, err)
	}

	return nil
}

// WriteFloat32ToRedis записывает float32 в Redis, автоматически выбирая шард по uuid
func WriteFloat32ToRedis(ctx context.Context, redisClients []*redis.Client, uuid, column string, data float32, logged bool) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, uuid)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", uuid, err)
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write float32 to Redis (uuid=%s, column=%s, shard=%d): %w", uuid, column, idx, err)
	}
	return nil
}

// WriteFloat64ToRedis записывает float64 в Redis, автоматически выбирая шард по uuid
func WriteFloat64ToRedis(ctx context.Context, redisClients []*redis.Client, uuid, column string, data float64, logged bool) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, uuid)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", uuid, err)
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, uuid, column, data)
	pipe.Expire(ctx, uuid, RedisKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write float64 to Redis (uuid=%s, column=%s, shard=%d): %w", uuid, column, idx, err)
	}
	return nil
}

// AddUUIDToRedisSet добавляет uuid в Redis set, выбирая шард по uuid
func AddUUIDToRedisSet(ctx context.Context, redisClients []*redis.Client, setName, uuid string, logged bool) error {
	if !logged || setName == "" {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, uuid)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", uuid, err)
	}

	if err := client.SAdd(ctx, setName, uuid).Err(); err != nil {
		return fmt.Errorf("failed to add UUID %s to set %s (shard=%d): %w", uuid, setName, idx, err)
	}
	return nil
}

// WriteWinStats записывает статистику выигрыша в Redis (несколько полей)
func WriteWinStats(
	ctx context.Context,
	redisClients []*redis.Client,
	globalId string,
	win *clickhouse_types.Bid,
	logged bool,
) error {
	var combinedErr error

	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.WIN_DSP_DOMAIN_COLUMN, *win.WinDspDomain, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("WinDspDomain: %w", err))
	}
	if err := WriteFloat32ToRedis(ctx, redisClients, globalId, constants.WIN_PRICE_COLUMN, *win.WinPrice, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("WinPrice: %w", err))
	}
	if err := WriteFloat32ToRedis(ctx, redisClients, globalId, constants.WIN_DSP_PRICE_COLUMN, *win.WinDspPrice, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("WinDspPrice: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.WIN_CID_COLUMN, *win.WinCid, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("WinCid: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.WIN_CRID_COLUMN, *win.WinCrid, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("WinCrid: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.WIN_USER_ID_COLUMN, *win.WinUserId, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("WinUserId: %w", err))
	}

	return combinedErr
}

// WriteStatsOrtb записывает ORTB-статистику в Redis
func WriteStatsOrtb(
	ctx context.Context,
	redisClients []*redis.Client,
	globalId string,
	logged bool,
	format string,
	typic string,
	ssp_domain string,
	ip string,
	ipv6 string,
	lang string,
	countryISO string,
	cityId uint32,
	uaFields ua.UAFields,
	siteId string,
	siteDomain string,
	bidFloor float64,
) error {
	var combinedErr error

	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.EVENT_TIME_COLUMN, time.Now().UTC().Format("2006-01-02 15:04:05.000"), logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("EventTime: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.FORMAT_COLUMN, format, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("Format: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.TYPIC_COLUMN, typic, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("Typic: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.SPP_DOMAIN_COLUMN, ssp_domain, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("SSP_Domain: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.IP_COLUMN, ip, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("IP: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.IPV6_COLUMN, ipv6, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("IPv6: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.LANG_COLUMN, lang, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("Language: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.BROWSER_COLUMN, uaFields.Browser, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("Browser: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.BROWSER_VERSION_COLUMN, uaFields.BrowserVersion, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("BrowserVersion: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.OS_COLUMN, uaFields.OS, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("OS: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.OS_VERSION_COLUMN, uaFields.OSVersion, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("OSVersion: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.DEVICE_COLUMN, uaFields.Device, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("Device: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.SITE_ID_COLUMN, siteId, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("SiteID: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.SITE_DOMAIN_COLUMN, siteDomain, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("SiteDomain: %w", err))
	}
	if err := WriteFloat64ToRedis(ctx, redisClients, globalId, constants.BID_FLOOR_COLUMN, bidFloor, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("BidFloor: %w", err))
	}
	if err := WriteStringToRedis(ctx, redisClients, globalId, constants.GEO_COLUMN, countryISO, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("Geo: %w", err))
	}
	if err := WriteUint32ToRedis(ctx, redisClients, globalId, constants.CITY_ID_COLUMN, cityId, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("CityID: %w", err))
	}

	return combinedErr
}
