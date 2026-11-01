package sppAdapterWeb

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestPublisherOwnerReadUsesExistingORTBAttribution(t *testing.T) {
	ctx := context.Background()
	// An ordinary request must not even need a Redis connection.
	if got, err := publisherOwnerForAuction(ctx, nil, ""); err != nil || got != "" {
		t.Fatal(got, err)
	}
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	c := redis.NewClient(&redis.Options{Addr: addr, DB: 13})
	defer c.Close()
	id, owner := uuid.NewString(), uuid.NewString()
	defer c.Del(ctx, id)
	if err := c.HSet(ctx, id, "pub_user_id", owner, "ad_code_id", "code").Err(); err != nil {
		t.Fatal(err)
	}
	before := c.HGetAll(ctx, id).Val()
	if got, err := publisherOwnerForAuction(ctx, []*redis.Client{c}, id); err != nil || got != owner {
		t.Fatal(got, err)
	}
	after := c.HGetAll(ctx, id).Val()
	if len(before) != len(after) || after["pub_user_id"] != owner || after["ad_code_id"] != "code" {
		t.Fatal("original attribution changed")
	}
	if err := c.HDel(ctx, id, "pub_user_id").Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := publisherOwnerForAuction(ctx, []*redis.Client{c}, id); err == nil {
		t.Fatal("missing owner silently bypassed map")
	}
}
