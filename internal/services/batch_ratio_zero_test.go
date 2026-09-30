package services

import (
	"strings"
	"testing"
)

func TestBatchRatioCriticalZeroHandlerFiresForInitialZero(t *testing.T) {
	manager := NewBatchRatioManager(0, 0.25, true)

	var calls int
	var gotErr error
	manager.SetCriticalZeroHandler(func(err error) {
		calls++
		gotErr = err
	})

	if calls != 1 {
		t.Fatalf("critical zero handler calls = %d, want 1", calls)
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "impressions_percent=0.000000") {
		t.Fatalf("critical zero handler error = %v, want zero impressions ratio", gotErr)
	}
}

func TestBatchRatioCriticalZeroHandlerDeduplicatesUntilRecovery(t *testing.T) {
	manager := NewBatchRatioManager(0.25, 0.15, true)

	var calls int
	manager.SetCriticalZeroHandler(func(error) { calls++ })

	if err := manager.SetManual(0, 0.15); err != nil {
		t.Fatalf("SetManual(first zero): %v", err)
	}
	if calls != 1 {
		t.Fatalf("critical zero handler calls after first zero = %d, want 1", calls)
	}

	if err := manager.SetManual(0, 0.10); err != nil {
		t.Fatalf("SetManual(repeated zero): %v", err)
	}
	if calls != 1 {
		t.Fatalf("critical zero handler calls during same zero episode = %d, want 1", calls)
	}

	if err := manager.SetManual(0.20, 0.10); err != nil {
		t.Fatalf("SetManual(recovery): %v", err)
	}
	if calls != 1 {
		t.Fatalf("critical zero handler calls after recovery = %d, want 1", calls)
	}

	if err := manager.SetManual(0.20, 0); err != nil {
		t.Fatalf("SetManual(second zero): %v", err)
	}
	if calls != 2 {
		t.Fatalf("critical zero handler calls after second zero episode = %d, want 2", calls)
	}
}
