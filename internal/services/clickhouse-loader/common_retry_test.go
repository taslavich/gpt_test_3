package clickhouse_loader

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestInsertBatchWithRetryRecoversWithoutReadingAnotherKafkaBatch(t *testing.T) {
	t.Parallel()

	calls := 0
	stats, err := insertBatchWithRetry(
		context.Background(),
		"TEST",
		10,
		0,
		func() (clickhouseInsertStats, error) {
			calls++
			if calls < 3 {
				return clickhouseInsertStats{}, errors.New("temporary ClickHouse error")
			}
			return clickhouseInsertStats{BadUUIDCount: 7}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("insert calls=%d, want 3", calls)
	}
	if stats.BadUUIDCount != 7 {
		t.Fatalf("stats=%+v, want successful attempt stats", stats)
	}
}

func TestInsertBatchWithRetryUsesExactlyConfiguredRetryCount(t *testing.T) {
	t.Parallel()

	calls := 0
	_, err := insertBatchWithRetry(
		context.Background(),
		"TEST",
		10,
		0,
		func() (clickhouseInsertStats, error) {
			calls++
			return clickhouseInsertStats{}, errors.New("still failing")
		},
	)
	if err == nil {
		t.Fatal("expected error after retries are exhausted")
	}
	if calls != 11 {
		t.Fatalf("insert calls=%d, want 11 (initial attempt + 10 retries)", calls)
	}
	if !strings.Contains(err.Error(), "after 10 retries (11 total attempts)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInsertBatchWithRetryDelayIsCancelable(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	_, err := insertBatchWithRetry(
		ctx,
		"TEST",
		10,
		time.Hour,
		func() (clickhouseInsertStats, error) {
			calls++
			cancel()
			return clickhouseInsertStats{}, errors.New("temporary ClickHouse error")
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("insert calls=%d, want 1", calls)
	}
}
