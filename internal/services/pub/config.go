package pub

import (
	"crypto/tls"
	"errors"
	"github.com/redis/go-redis/v9"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	AntiBlockEnabled        bool          `env:"PUB_ANTIBLOCK_ENABLED" env-default:"false"`
	AntiBlockDomains        string        `env:"PUB_ANTIBLOCK_DOMAINS"`
	AntiBlockStableOrigin   string        `env:"PUB_ANTIBLOCK_STABLE_ORIGIN"`
	AntiBlockRoutes         string        `env:"PUB_ANTIBLOCK_ROUTES_JSON"`
	AntiBlockRuntimeOrigin  string        `env:"PUB_ANTIBLOCK_RUNTIME_ORIGIN"`
	AntiBlockAPIOrigin      string        `env:"PUB_ANTIBLOCK_API_ORIGIN"`
	AntiBlockDeliveryOrigin string        `env:"PUB_ANTIBLOCK_DELIVERY_ORIGIN"`
	AntiBlockAutoRotate     bool          `env:"PUB_ANTIBLOCK_AUTO_ROTATE" env-default:"true"`
	AntiBlockCheckInterval  time.Duration `env:"PUB_ANTIBLOCK_CHECK_INTERVAL" env-default:"24h"`
	AntiBlockPollInterval   time.Duration `env:"PUB_ANTIBLOCK_POLL_INTERVAL" env-default:"15s"`
	AntiBlockHTTPTimeout    time.Duration `env:"PUB_ANTIBLOCK_HTTP_TIMEOUT" env-default:"10s"`
	AntiBlockMaxListBytes   int64         `env:"PUB_ANTIBLOCK_MAX_LIST_BYTES" env-default:"16777216"`
	AntiBlockFilterScript   string        `env:"PUB_ANTIBLOCK_FILTER_SCRIPT" env-default:"scripts/pub-client/filter.cjs"`
	AntiBlockNode           string        `env:"PUB_ANTIBLOCK_NODE" env-default:"node"`
	AntiBlockSamplePages    string        `env:"PUB_ANTIBLOCK_SAMPLE_PAGES" env-default:"https://publisher.invalid/"`
	RedisCluster            bool          `env:"PUBLISHER_REDIS_CLUSTER" env-default:"false"`
	RedisUsername           string        `env:"PUBLISHER_REDIS_USERNAME"`
	Listen                  string        `env:"PUB_LISTEN" env-default:"127.0.0.1:8087"`
	PublicURL               string        `env:"PUB_PUBLIC_URL" env-required:"true"`
	RedisAddrs              string        `env:"PUBLISHER_REDIS_ADDRS" env-required:"true"`
	RedisPassword           string        `env:"PUBLISHER_REDIS_PASSWORD"`
	RedisTLS                bool          `env:"PUBLISHER_REDIS_TLS" env-default:"false"`
	InternalSecret          string        `env:"PUB_INTERNAL_SECRET" env-required:"true"`
	DeliverySecret          string        `env:"PUB_DELIVERY_SECRET" env-required:"true"`
	RTBAdultURL             string        `env:"PUB_RTB_ADULT_URL" env-required:"true"`
	RTBMainstreamURL        string        `env:"PUB_RTB_MAINSTREAM_URL" env-required:"true"`
	AdultFeed               string        `env:"PUB_ADULT_FEED" env-required:"true"`
	MainstreamFeed          string        `env:"PUB_MAINSTREAM_FEED" env-required:"true"`
	CallbackOrigins         string        `env:"PUB_CALLBACK_ORIGINS" env-required:"true"`
	TrustedProxies          string        `env:"PUB_TRUSTED_PROXIES"`
	RTBTimeout              time.Duration `env:"PUB_RTB_TIMEOUT" env-default:"1500ms"`
	MaxInflight             int           `env:"PUB_MAX_INFLIGHT" env-default:"2048"`
	// Same ordered standalone ORTB shards/DB as SSP Adapter and Kafka Loader.
	ORTBAddrs         string `env:"REDIS_SHARD_ADDRS" env-required:"true"`
	ORTBPassword      string `env:"REDIS_PASSWORD"`
	ORTBTLS           bool   `env:"REDIS_USE_TLS" env-default:"false"`
	ORTBDB            int    `env:"REDIS_DB_ORTB" env-required:"true"`
	ORTBReadySet      string `env:"REDIS_SET_ORTB" env-default:"ortb:ready"`
	TelemetryDir      string `env:"PUB_TELEMETRY_DIR" env-default:"/var/lib/twinbid-pub/telemetry"`
	TelemetryMaxBytes int64  `env:"PUB_TELEMETRY_MAX_BYTES" env-default:"536870912"`
	// Operational kill switch only. Confirmation describes opening the bridge,
	// never successful loading of a cross-origin advertiser document.
	ConfirmedOpenEnabled bool `env:"PUB_CONFIRMED_OPEN_ENABLED" env-default:"true"`
}

func (c Config) Validate() error {
	if _, e := infrastructureSettings(c); e != nil {
		return e
	}
	if len(c.InternalSecret) < 32 || len(c.DeliverySecret) < 32 {
		return errors.New("PUB_INTERNAL_SECRET and PUB_DELIVERY_SECRET must be >=32 chars")
	}
	if c.MaxInflight < 1 || c.RTBTimeout <= 0 || c.RTBTimeout > 10*time.Second {
		return errors.New("invalid PUB_MAX_INFLIGHT/PUB_RTB_TIMEOUT")
	}
	for _, v := range []string{c.PublicURL, c.RTBAdultURL, c.RTBMainstreamURL} {
		u, e := url.Parse(v)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return errors.New("invalid public/RTB URL")
		}
	}
	if c.AdultFeed == "" || c.MainstreamFeed == "" || c.RedisAddrs == "" || c.CallbackOrigins == "" || c.ORTBAddrs == "" || c.ORTBReadySet == "" || c.ORTBDB < 0 || c.TelemetryMaxBytes < 1 {
		return errors.New("missing feed, Redis, callback or ORTB configuration")
	}
	return nil
}
func (c Config) Redis() redis.UniversalClient {
	o := &redis.UniversalOptions{Addrs: strings.Split(c.RedisAddrs, ","), Password: c.RedisPassword, Username: c.RedisUsername, DB: 0, PoolSize: 256, ReadTimeout: time.Second, WriteTimeout: time.Second}
	if c.RedisTLS {
		o.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if c.RedisCluster {
		return redis.NewClusterClient(&redis.ClusterOptions{Addrs: o.Addrs, Username: c.RedisUsername, Password: c.RedisPassword, TLSConfig: o.TLSConfig, ReadTimeout: o.ReadTimeout, WriteTimeout: o.WriteTimeout, PoolSize: o.PoolSize})
	}
	return redis.NewUniversalClient(o)
}

func (s *Server) feed(traffic string) (string, string, error) {
	switch traffic {
	case "adult":
		return s.Config.RTBAdultURL, s.Config.AdultFeed, nil
	case "mainstream":
		return s.Config.RTBMainstreamURL, s.Config.MainstreamFeed, nil
	default:
		return "", "", errors.New("traffic_type must be adult or mainstream")
	}
}
