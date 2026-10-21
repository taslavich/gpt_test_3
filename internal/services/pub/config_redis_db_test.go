package pub

import (
	"strings"
	"testing"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/redis/go-redis/v9"
)

func TestPublisherRedisDBEnv(t *testing.T) {
	// Read the same environment-backed Config as cmd/pub/main.go.
	env := map[string]string{
		"PUB_PUBLIC_URL":          "https://pub.example.test",
		"PUBLISHER_REDIS_ADDRS":   "127.0.0.1:6379",
		"PUBLISHER_REDIS_CLUSTER": "false",
		"PUB_ANTIBLOCK_ENABLED":   "false",
		"PUB_INTERNAL_SECRET":     strings.Repeat("x", 32),
		"PUB_DELIVERY_SECRET":     strings.Repeat("y", 32),
		"PUB_RTB_ADULT_URL":       "https://rtb.example.test/adult",
		"PUB_RTB_MAINSTREAM_URL":  "https://rtb.example.test/mainstream",
		"PUB_ADULT_FEED":          "adult",
		"PUB_MAINSTREAM_FEED":     "mainstream",
		"PUB_CALLBACK_ORIGINS":    "https://adm.example.test",
		"REDIS_SHARD_ADDRS":       "127.0.0.1:6379",
		"REDIS_DB_ORTB":           "2",
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	for _, tt := range []struct {
		name  string
		value string
		want  int
	}{
		{name: "cabinet publisher DB", value: "8", want: 8},
		{name: "another DB", value: "11", want: 11},
		{name: "DB zero", value: "0", want: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PUBLISHER_REDIS_DB", tt.value)
			var cfg Config
			if err := cleanenv.ReadEnv(&cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.RedisDB != tt.want {
				t.Fatalf("PUBLISHER_REDIS_DB parsed as %d; want %d", cfg.RedisDB, tt.want)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatal(err)
			}
			client := cfg.Redis()
			defer client.Close()
			standalone, ok := client.(*redis.Client)
			if !ok || standalone.Options().DB != tt.want {
				t.Fatalf("redis client DB mismatch: %#v", client)
			}
		})
	}
}

func TestPublisherRedisDBValidation(t *testing.T) {
	cfg := Config{
		PublicURL:         "https://pub.example.test",
		RedisAddrs:        "127.0.0.1:6379",
		RedisDB:           8,
		InternalSecret:    strings.Repeat("x", 32),
		DeliverySecret:    strings.Repeat("y", 32),
		RTBAdultURL:       "https://rtb.example.test/adult",
		RTBMainstreamURL:  "https://rtb.example.test/mainstream",
		AdultFeed:         "adult",
		MainstreamFeed:    "mainstream",
		CallbackOrigins:   "https://adm.example.test",
		ORTBAddrs:         "127.0.0.1:6379",
		ORTBDB:            2,
		ORTBReadySet:      "ortb:ready",
		TelemetryMaxBytes: 1,
		MaxInflight:       1,
		RTBTimeout:        time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.RedisDB = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("negative Redis DB accepted")
	}
	cfg.RedisDB = 8
	cfg.RedisCluster = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("nonzero DB silently ignored by Redis Cluster")
	}
	cfg.RedisDB = 0
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
