package auction

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShouldRetryPercenterInitOutsideRequest(t *testing.T) {
	if !shouldRetryPercenterInitOutsideRequest(context.Canceled) {
		t.Fatal("context.Canceled must use detached initialization retry")
	}
	if !shouldRetryPercenterInitOutsideRequest(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded must use detached initialization retry")
	}
	wrapped := errors.Join(errors.New("outer"), context.DeadlineExceeded)
	if !shouldRetryPercenterInitOutsideRequest(wrapped) {
		t.Fatal("wrapped request deadline must use detached initialization retry")
	}
	if shouldRetryPercenterInitOutsideRequest(errors.New("redis protocol error")) {
		t.Fatal("non-context Redis errors must stay on the normal health-error path")
	}
}

func TestDetachedPercenterInitContextSurvivesParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	retryCtx, cancelRetry := detachedPercenterInitContext(parent)
	defer cancelRetry()
	select {
	case <-retryCtx.Done():
		t.Fatalf("detached retry inherited parent cancellation: %v", retryCtx.Err())
	default:
	}
	deadline, ok := retryCtx.Deadline()
	if !ok {
		t.Fatal("detached retry must have its own bounded deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > percenterInitRetryTimeout+250*time.Millisecond {
		t.Fatalf("unexpected retry deadline remaining=%s", remaining)
	}
}

func TestPercenterInitRetryKeyIncludesCampaignVersion(t *testing.T) {
	if percenterInitRetryKey("abc", 1) == percenterInitRetryKey("abc", 2) {
		t.Fatal("different campaign versions must not share one in-flight retry key")
	}
}
