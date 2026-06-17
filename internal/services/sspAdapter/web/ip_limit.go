package sppAdapterWeb

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type IPLimitMap map[string]struct{}

type IPLimitTables struct {
	IPv4 string
	IPv6 string
}

type IPLimitConfig struct {
	FullReloadInterval time.Duration
	BatchLoadInterval  time.Duration
	Tables             IPLimitTables
}

type IPLimitStore struct {
	ipv4 atomic.Value
	ipv6 atomic.Value

	mu              sync.Mutex
	lastIPv4BatchAt time.Time
	lastIPv6BatchAt time.Time
}

func NewIPLimitStore() *IPLimitStore {
	s := &IPLimitStore{}
	s.ipv4.Store(IPLimitMap{})
	s.ipv6.Store(IPLimitMap{})
	return s
}

func (s *IPLimitStore) ContainsIPv4(ip string) bool { return s.contains(&s.ipv4, ip) }
func (s *IPLimitStore) ContainsIPv6(ip string) bool { return s.contains(&s.ipv6, ip) }

func (s *IPLimitStore) contains(value *atomic.Value, ip string) bool {
	if s == nil || strings.TrimSpace(ip) == "" {
		return false
	}
	items, ok := value.Load().(IPLimitMap)
	if !ok {
		return false
	}
	_, exists := items[ip]
	return exists
}

func (s *IPLimitStore) StartClickHouseLoaders(ctx context.Context, ch clickhouse.Conn, cfg IPLimitConfig) error {
	if err := s.ReloadAll(ctx, ch, cfg.Tables); err != nil {
		return fmt.Errorf("load initial IP limit maps: %w", err)
	}

	go s.fullReloadLoop(ctx, ch, cfg)
	go s.latestBatchLoop(ctx, ch, cfg)

	return nil
}

func (s *IPLimitStore) fullReloadLoop(ctx context.Context, ch clickhouse.Conn, cfg IPLimitConfig) {
	ticker := time.NewTicker(cfg.FullReloadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ReloadAll(ctx, ch, cfg.Tables); err != nil {
				log.Printf("failed to reload full IP limit maps: %v", err)
			}
		}
	}
}

func (s *IPLimitStore) latestBatchLoop(ctx context.Context, ch clickhouse.Conn, cfg IPLimitConfig) {
	ticker := time.NewTicker(cfg.BatchLoadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.LoadNewBatches(ctx, ch, cfg.Tables); err != nil {
				log.Printf("failed to load new IP limit batches: %v", err)
			}
		}
	}
}

func (s *IPLimitStore) ReloadAll(ctx context.Context, ch clickhouse.Conn, tables IPLimitTables) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ipv4, lastIPv4BatchAt, err := fetchIPLimitAll(ctx, ch, tables.IPv4)
	if err != nil {
		return fmt.Errorf("load all ipv4 limits: %w", err)
	}
	ipv6, lastIPv6BatchAt, err := fetchIPLimitAll(ctx, ch, tables.IPv6)
	if err != nil {
		return fmt.Errorf("load all ipv6 limits: %w", err)
	}

	s.ipv4.Store(ipv4)
	s.ipv6.Store(ipv6)
	s.lastIPv4BatchAt = lastIPv4BatchAt
	s.lastIPv6BatchAt = lastIPv6BatchAt

	log.Printf("loaded IP limit maps: ipv4=%d last_ipv4_batch_at=%s ipv6=%d last_ipv6_batch_at=%s", len(ipv4), lastIPv4BatchAt.Format(time.RFC3339Nano), len(ipv6), lastIPv6BatchAt.Format(time.RFC3339Nano))
	return nil
}

func (s *IPLimitStore) LoadNewBatches(ctx context.Context, ch clickhouse.Conn, tables IPLimitTables) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ipv4, lastIPv4BatchAt, err := fetchIPLimitAfter(ctx, ch, tables.IPv4, s.lastIPv4BatchAt)
	if err != nil {
		return fmt.Errorf("load new ipv4 limits: %w", err)
	}
	ipv6, lastIPv6BatchAt, err := fetchIPLimitAfter(ctx, ch, tables.IPv6, s.lastIPv6BatchAt)
	if err != nil {
		return fmt.Errorf("load new ipv6 limits: %w", err)
	}

	s.merge(&s.ipv4, ipv4)
	s.merge(&s.ipv6, ipv6)
	if lastIPv4BatchAt.After(s.lastIPv4BatchAt) {
		s.lastIPv4BatchAt = lastIPv4BatchAt
	}
	if lastIPv6BatchAt.After(s.lastIPv6BatchAt) {
		s.lastIPv6BatchAt = lastIPv6BatchAt
	}

	log.Printf("loaded new IP limit batches: ipv4=%d last_ipv4_batch_at=%s ipv6=%d last_ipv6_batch_at=%s", len(ipv4), s.lastIPv4BatchAt.Format(time.RFC3339Nano), len(ipv6), s.lastIPv6BatchAt.Format(time.RFC3339Nano))
	return nil
}

func (s *IPLimitStore) merge(value *atomic.Value, batch IPLimitMap) {
	if len(batch) == 0 {
		return
	}
	current, _ := value.Load().(IPLimitMap)
	next := make(IPLimitMap, len(current)+len(batch))
	for ip := range current {
		next[ip] = struct{}{}
	}
	for ip := range batch {
		next[ip] = struct{}{}
	}
	value.Store(next)
}

func fetchIPLimitAll(ctx context.Context, ch clickhouse.Conn, table string) (IPLimitMap, time.Time, error) {
	return fetchIPLimitRows(ctx, ch, fmt.Sprintf("SELECT ip, created_at FROM %s", clickhouseIdentifier(table)))
}

func fetchIPLimitAfter(ctx context.Context, ch clickhouse.Conn, table string, after time.Time) (IPLimitMap, time.Time, error) {
	return fetchIPLimitRows(ctx, ch, fmt.Sprintf("SELECT ip, created_at FROM %s WHERE created_at > ?", clickhouseIdentifier(table)), after)
}

func fetchIPLimitRows(ctx context.Context, ch clickhouse.Conn, query string, args ...any) (IPLimitMap, time.Time, error) {
	rows, err := ch.Query(ctx, query, args...)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()

	result := make(IPLimitMap)
	var maxCreatedAt time.Time
	for rows.Next() {
		var (
			ip        string
			createdAt time.Time
		)
		if err := rows.Scan(&ip, &createdAt); err != nil {
			return nil, time.Time{}, err
		}
		if ip = strings.TrimSpace(ip); ip != "" {
			result[ip] = struct{}{}
		}
		if createdAt.After(maxCreatedAt) {
			maxCreatedAt = createdAt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, err
	}
	return result, maxCreatedAt, nil
}

func clickhouseIdentifier(identifier string) string {
	parts := strings.Split(identifier, ".")
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ReplaceAll(strings.TrimSpace(part), "`", "``")
		quoted = append(quoted, "`"+part+"`")
	}
	return strings.Join(quoted, ".")
}
