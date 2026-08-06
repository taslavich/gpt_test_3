package redis_service

import (
	"context"
	"crypto/tls"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClients struct {
	Ortb        *redis.Client
	Clicks      *redis.Client
	Impressions *redis.Client
	Conversions *redis.Client
}

type RedisShardedClients struct {
	Ortb        []*redis.Client
	Clicks      []*redis.Client
	Impressions []*redis.Client
	Conversions []*redis.Client
}

func ShardIndex(uuid string, shardCount int) int {
	if shardCount <= 0 {
		return 0
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(uuid))

	return int(h.Sum32() % uint32(shardCount))
}

func SelectShard(clients []*redis.Client, uuid string) (*redis.Client, int, error) {
	if len(clients) == 0 {
		return nil, 0, fmt.Errorf("redis shard clients list is empty")
	}

	if uuid == "" {
		//return nil, 0, fmt.Errorf("uuid cannot be empty")
	}

	idx := ShardIndex(uuid, len(clients))

	if idx < 0 || idx >= len(clients) {
		return nil, 0, fmt.Errorf("invalid shard index %d, clients count %d", idx, len(clients))
	}

	client := clients[idx]
	if client == nil {
		return nil, idx, fmt.Errorf("redis client at index %d is nil", idx)
	}

	return client, idx, nil
}

func CloseClients(clients []*redis.Client) error {
	var lastErr error
	for i, client := range clients {
		if client != nil {
			if err := client.Close(); err != nil {
				lastErr = fmt.Errorf("failed to close client %d: %w", i, err)
				// продолжаем закрывать остальные
			}
		}
	}
	return lastErr
}

func (c *RedisShardedClients) Close() error {
	if c == nil {
		return nil
	}

	var lastErr error

	if err := CloseClients(c.Ortb); err != nil {
		lastErr = fmt.Errorf("failed to close ORTB clients: %w", err)
	}

	if err := CloseClients(c.Impressions); err != nil {
		if lastErr != nil {
			lastErr = fmt.Errorf("%v; impressions: %w", lastErr, err)
		} else {
			lastErr = fmt.Errorf("failed to close impressions clients: %w", err)
		}
	}

	if err := CloseClients(c.Clicks); err != nil {
		if lastErr != nil {
			lastErr = fmt.Errorf("%v; clicks: %w", lastErr, err)
		} else {
			lastErr = fmt.Errorf("failed to close clicks clients: %w", err)
		}
	}

	if err := CloseClients(c.Conversions); err != nil {
		if lastErr != nil {
			lastErr = fmt.Errorf("%v; conversions: %w", lastErr, err)
		} else {
			lastErr = fmt.Errorf("failed to close conversions clients: %w", err)
		}
	}

	return lastErr
}

func NewRedisShardedClients(
	addrs []string,
	password string,
	ortbDb int,
	impressionsDb int,
	clicksDb int,
	conversionsDb int,
	useTLS bool,
	poolSize int,
	minIdleConns int,
) (*RedisShardedClients, error) {
	if len(addrs) == 0 {
		return nil, fmt.Errorf("redis shard addrs list is empty")
	}

	// валидация адресов
	for i, addr := range addrs {
		if addr == "" {
			return nil, fmt.Errorf("address at index %d is empty", i)
		}
	}

	ortbClients, err := newRedisClientsForDB(addrs, password, ortbDb, useTLS, poolSize, minIdleConns)
	if err != nil {
		return nil, fmt.Errorf("failed to create ORTB redis shard clients: %w", err)
	}

	impressionClients, err := newRedisClientsForDB(addrs, password, impressionsDb, useTLS, poolSize, minIdleConns)
	if err != nil {
		CloseClients(ortbClients)
		return nil, fmt.Errorf("failed to create impressions redis shard clients: %w", err)
	}

	clickClients, err := newRedisClientsForDB(addrs, password, clicksDb, useTLS, poolSize, minIdleConns)
	if err != nil {
		CloseClients(ortbClients)
		CloseClients(impressionClients)
		return nil, fmt.Errorf("failed to create clicks redis shard clients: %w", err)
	}

	conversionClients, err := newRedisClientsForDB(addrs, password, conversionsDb, useTLS, poolSize, minIdleConns)
	if err != nil {
		CloseClients(ortbClients)
		CloseClients(impressionClients)
		CloseClients(clickClients)
		return nil, fmt.Errorf("failed to create conversions redis shard clients: %w", err)
	}

	return &RedisShardedClients{
		Ortb:        ortbClients,
		Impressions: impressionClients,
		Clicks:      clickClients,
		Conversions: conversionClients,
	}, nil
}

func NewRedisClient(
	addr string,
	password string,
	db int,
	poolSize int,
	minIdleConns int,
) (*redis.Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("redis address is empty")
	}

	clients, err := newRedisClientsForDB([]string{addr}, password, db, false, poolSize, minIdleConns)
	if err != nil {
		return nil, err
	}

	return clients[0], nil
}

func NewRedisShardedClientsForDB(
	addrs []string,
	password string,
	db int,
	useTLS bool,
	poolSize int,
	minIdleConns int,
) ([]*redis.Client, error) {
	return newRedisClientsForDB(addrs, password, db, useTLS, poolSize, minIdleConns)
}

func newRedisClientsForDB(
	addrs []string,
	password string,
	db int,
	useTLS bool,
	poolSize int,
	minIdleConns int,
) ([]*redis.Client, error) {
	if poolSize <= 0 {
		poolSize = 64
	}

	if minIdleConns <= 0 {
		minIdleConns = 16
	}

	if minIdleConns > poolSize {
		minIdleConns = poolSize
	}

	clients := make([]*redis.Client, 0, len(addrs))

	for i, addr := range addrs {
		options := &redis.Options{
			Addr:         addr,
			Password:     password,
			DB:           db,
			PoolSize:     poolSize,
			MinIdleConns: minIdleConns,
			PoolTimeout:  3 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		}

		if useTLS {
			options.TLSConfig = &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			}
		}

		client := redis.NewClient(options)

		// проверяем что клиент создан
		if client == nil {
			return nil, fmt.Errorf("failed to create redis client for address %s (index %d)", addr, i)
		}

		clients = append(clients, client)
	}

	return clients, nil
}

func PingClients(ctx context.Context, name string, clients []*redis.Client) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	if len(clients) == 0 {
		return fmt.Errorf("%s redis clients list is empty", name)
	}

	for i, client := range clients {
		if client == nil {
			return fmt.Errorf("%s redis client at index %d is nil", name, i)
		}

		if err := client.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("%s redis shard %d ping failed: %w", name, i, err)
		}
	}

	return nil
}

func PingAllShards(ctx context.Context, sharded *RedisShardedClients) error {
	if sharded == nil {
		return fmt.Errorf("sharded clients is nil")
	}

	if err := PingClients(ctx, "ortb", sharded.Ortb); err != nil {
		return err
	}

	if err := PingClients(ctx, "impressions", sharded.Impressions); err != nil {
		return err
	}

	if err := PingClients(ctx, "clicks", sharded.Clicks); err != nil {
		return err
	}

	if err := PingClients(ctx, "conversions", sharded.Conversions); err != nil {
		return err
	}

	return nil
}
