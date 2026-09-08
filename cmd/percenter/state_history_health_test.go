package main

import (
	"context"
	"errors"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

func TestShouldReportStateHistoryErrorIgnoresRedisNil(t *testing.T) {
	if shouldReportStateHistoryError(goredis.Nil) {
		t.Fatal("redis.Nil must not be treated as a state/history health failure")
	}
	if shouldReportStateHistoryError(nil) {
		t.Fatal("nil error must not be reported")
	}
	if !shouldReportStateHistoryError(context.DeadlineExceeded) {
		t.Fatal("real timeout must be reported")
	}
	if !shouldReportStateHistoryError(errors.New("redis connection failed")) {
		t.Fatal("real Redis error must be reported")
	}
}

func TestStateHistoryTickHealthKeepsFailureForWholeTick(t *testing.T) {
	health := &stateHistoryTickHealth{}
	health.failure(context.Background(), "boom")
	health.success()
	if !health.failed || !health.succeeded {
		t.Fatalf("unexpected tick health state: failed=%t succeeded=%t", health.failed, health.succeeded)
	}
	health.finish(context.Background())
	if !health.failed {
		t.Fatal("a later success in the same tick must not clear a real failure")
	}
}
