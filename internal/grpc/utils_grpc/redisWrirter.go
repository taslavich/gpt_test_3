package utils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
)

const RedisKeyTTL = 5 * time.Minute

func WriteStringToRedis(ctx context.Context, redisClient *redis.Client, uuid, column string, data string, logged bool) error {
	if !logged {
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
	if !logged {
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
	if !logged {
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
	if !logged {
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

func WriteFloat64ToRedis(ctx context.Context, redisClient *redis.Client, uuid, column string, data float64, logged bool) error {
	if !logged {
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

func WriteWinStats(
	ctx context.Context,
	redisClient *redis.Client,
	globalId string,
	win *clickhouse_types.Bid,
	logged bool,
) error {
	var combinedErr error

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.WIN_DSP_DOMAIN_COLUMN, *win.WinDspDomain, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write WinDspDomain: %w", err))
	}

	if err := WriteFloat32ToRedis(ctx, redisClient, globalId, constants.WIN_PRICE_COLUMN, *win.WinPrice, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write WinPrice: %w", err))
	}

	if err := WriteFloat32ToRedis(ctx, redisClient, globalId, constants.WIN_DSP_PRICE_COLUMN, *win.WinDspPrice, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write WinDspPrice: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.WIN_CID_COLUMN, *win.WinCid, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write WinCid: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.WIN_CRID_COLUMN, *win.WinCrid, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write WinCrid: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.WIN_USER_ID_COLUMN, *win.WinUserId, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write WinUserId: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.WIN_FLAG_COLUMN, *win.WinFlag, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write WinFlag: %w", err))
	}

	return combinedErr
}
func WriteStatsOrtb(
	ctx context.Context,
	redisClient *redis.Client,
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
	uaFileds ua.UAFields,
	siteId string,
	siteDomain string,
	bidFloor float64,
) error {
	var combinedErr error

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.EVENT_TIME_COLUMN, time.Now().UTC().Format("2006-01-02 15:04:05.000"), logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write Timestamp: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.FORMAT_COLUMN, format, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write Format: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.TYPIC_COLUMN, typic, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write Typic: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.SPP_DOMAIN_COLUMN, ssp_domain, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write SSP_Domain: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.IP_COLUMN, ip, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write IP: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.IPV6_COLUMN, ipv6, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write IPv6: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.LANG_COLUMN, lang, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write Language: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.BROWSER_COLUMN, uaFileds.Browser, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write Browser: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.BROWSER_VERSION_COLUMN, uaFileds.BrowserVersion, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write Browser version: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.OS_COLUMN, uaFileds.OS, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write OS: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.OS_VERSION_COLUMN, uaFileds.OSVersion, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write OS Version: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.DEVICE_COLUMN, uaFileds.Device, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write Device: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.SITE_ID_COLUMN, siteId, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write SiteID: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.SITE_DOMAIN_COLUMN, siteDomain, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write SiteDomain: %w", err))
	}

	if err := WriteFloat64ToRedis(ctx, redisClient, globalId, constants.BID_FLOOR_COLUMN, bidFloor, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write BidFloor: %w", err))
	}

	if err := WriteStringToRedis(ctx, redisClient, globalId, constants.GEO_COLUMN, countryISO, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write Geo: %w", err))
	}

	if err := WriteUint32ToRedis(ctx, redisClient, globalId, constants.CITY_ID_COLUMN, cityId, logged); err != nil {
		combinedErr = errors.Join(combinedErr, fmt.Errorf("failed to write CityID: %w", err))
	}

	return combinedErr
}
