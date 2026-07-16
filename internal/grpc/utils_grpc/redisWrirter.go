package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	redis_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
)

const RedisKeyTTL = 1440 * time.Minute

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

// WriteUUIDKeyToRedis writes a UUID into a Redis hash with TTL.
func WriteUUIDKeyToRedis(ctx context.Context, redisClient *redis.Client, uuid string, ttl time.Duration) error {
	if redisClient == nil {
		return fmt.Errorf("redis client is nil")
	}
	if uuid == "" {
		//return fmt.Errorf("uuid cannot be empty")
	}
	if ttl <= 0 {
		return fmt.Errorf("redis uuid key ttl must be positive")
	}

	pipe := redisClient.Pipeline()
	pipe.HSet(ctx, uuid, "uuid", uuid)
	pipe.Expire(ctx, uuid, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write uuid hash to Redis (uuid=%s): %w", uuid, err)
	}

	return nil
}

// UUIDKeyExistsInRedis checks key existence using a single EXISTS command.
func UUIDKeyExistsInRedis(ctx context.Context, redisClient *redis.Client, uuid string) (bool, error) {
	if redisClient == nil {
		return false, fmt.Errorf("redis client is nil")
	}
	if uuid == "" {
		//return false, fmt.Errorf("uuid cannot be empty")
	}

	exists, err := redisClient.Exists(ctx, uuid).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check uuid key in Redis (uuid=%s): %w", uuid, err)
	}

	return exists > 0, nil
}

// WriteWinStats записывает статистику выигрыша в Redis (несколько полей одним HSET)
func WriteWinStats(
	ctx context.Context,
	redisClients []*redis.Client,
	globalId string,
	win *clickhouse_types.Bid,
	logged bool,
) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, globalId)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", globalId, err)
	}

	fields := map[string]interface{}{
		constants.WIN_DSP_DOMAIN_COLUMN: *win.WinDspDomain,
		constants.WIN_PRICE_COLUMN:      *win.WinPrice,
		constants.WIN_DSP_PRICE_COLUMN:  *win.WinDspPrice,
		constants.WIN_CID_COLUMN:        *win.WinCid,
		constants.WIN_CRID_COLUMN:       *win.WinCrid,
		constants.WIN_USER_ID_COLUMN:    *win.WinUserId,
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, globalId, fields)
	pipe.Expire(ctx, globalId, RedisKeyTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write win stats to Redis (uuid=%s, shard=%d): %w", globalId, idx, err)
	}

	return nil
}

// WriteStatsOrtb записывает ORTB-статистику в Redis одним HSET
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
	code int32,
	uaFields ua.UAFields,
	siteId string,
	siteDomain string,
	bidFloor float64,
) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, globalId)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", globalId, err)
	}

	fields := map[string]interface{}{
		constants.EVENT_TIME_COLUMN:      time.Now().UTC().Format("2006-01-02 15:04:05.000"),
		constants.FORMAT_COLUMN:          format,
		constants.TYPIC_COLUMN:           typic,
		constants.SPP_DOMAIN_COLUMN:      ssp_domain,
		constants.IP_COLUMN:              ip,
		constants.IPV6_COLUMN:            ipv6,
		constants.LANG_COLUMN:            lang,
		constants.BROWSER_COLUMN:         uaFields.Browser,
		constants.BROWSER_VERSION_COLUMN: uaFields.BrowserVersion,
		constants.OS_COLUMN:              uaFields.OS,
		constants.OS_VERSION_COLUMN:      uaFields.OSVersion,
		constants.DEVICE_COLUMN:          uaFields.Device,
		constants.SITE_ID_COLUMN:         siteId,
		constants.SITE_DOMAIN_COLUMN:     siteDomain,
		constants.BID_FLOOR_COLUMN:       bidFloor,
		constants.GEO_COLUMN:             countryISO,
		constants.CITY_ID_COLUMN:         cityId,
		constants.CODE_COLUMN:            code,
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, globalId, fields)
	pipe.Expire(ctx, globalId, RedisKeyTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write ORTB stats to Redis (uuid=%s, shard=%d): %w", globalId, idx, err)
	}

	return nil
}

// WriteClickStats записывает статистику клика в Redis одним HSET.
func WriteClickStats(
	ctx context.Context,
	redisClients []*redis.Client,
	clickUuid string,
	globalId string,
	format string,
	logged bool,
) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, clickUuid)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", clickUuid, err)
	}

	fields := map[string]interface{}{
		constants.ORTB_UUID:                globalId,
		constants.EVENT_TIME_CLICKS_COLUMN: time.Now().UTC().Format("2006-01-02 15:04:05.000"),
		constants.FORMAT_COLUMN:            format,
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, clickUuid, fields)
	pipe.Expire(ctx, clickUuid, RedisKeyTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write click stats to Redis (uuid=%s, shard=%d): %w", clickUuid, idx, err)
	}

	return nil
}

// WriteImpressionStats записывает статистику показа в Redis одним HSET.
func WriteImpressionStats(
	ctx context.Context,
	redisClients []*redis.Client,
	impressionsUuid string,
	globalId string,
	format string,
	logged bool,
) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, impressionsUuid)
	if err != nil {
		return fmt.Errorf("failed to select shard for uuid %s: %w", impressionsUuid, err)
	}

	fields := map[string]interface{}{
		constants.ORTB_UUID:                     globalId,
		constants.EVENT_TIME_IMPRESSIONS_COLUMN: time.Now().UTC().Format("2006-01-02 15:04:05.000"),
		constants.FORMAT_COLUMN:                 format,
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, impressionsUuid, fields)
	pipe.Expire(ctx, impressionsUuid, RedisKeyTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write impression stats to Redis (uuid=%s, shard=%d): %w", impressionsUuid, idx, err)
	}

	return nil
}

func WriteConversionStats(
	ctx context.Context,
	redisClients []*redis.Client,
	conversionsUuid string,
	clickUuid string,
	payout string,
	status string,
	conversionEventTime time.Time,
	logged bool,
) error {
	if !logged {
		return nil
	}

	client, idx, err := redis_service.SelectShard(redisClients, conversionsUuid)
	if err != nil {
		return fmt.Errorf(
			"failed to select shard for uuid %s: %w",
			conversionsUuid,
			err,
		)
	}

	fields := map[string]interface{}{
		constants.CLICKS_UUID: clickUuid,
		constants.PAYOUT:      payout,
		constants.STATUS:      status,
		constants.CONVERSION_EVENT_TIME_COLUMN: conversionEventTime.
			UTC().
			Format("2006-01-02 15:04:05.000"),
	}

	pipe := client.Pipeline()
	pipe.HSet(ctx, conversionsUuid, fields)
	pipe.Expire(ctx, conversionsUuid, RedisKeyTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf(
			"failed to write conversion stats to Redis (uuid=%s, shard=%d): %w",
			conversionsUuid,
			idx,
			err,
		)
	}

	return nil
}
