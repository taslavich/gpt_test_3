package services

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

func TestResettableOnceAllowsDoAfterReset(t *testing.T) {
	once := NewResettableOnce()
	calls := 0

	once.Do(func() { calls++ })
	once.Do(func() { calls++ })
	if calls != 1 {
		t.Fatalf("Do() before Reset() calls = %d, want 1", calls)
	}

	once.Reset()
	once.Do(func() { calls++ })
	once.Do(func() { calls++ })
	if calls != 2 {
		t.Fatalf("Do() after Reset() calls = %d, want 2", calls)
	}
}

func TestLoaderControlAddOnStartRunsOnlyOnStoppedToRunningTransition(t *testing.T) {
	control := NewLoaderControl(false)
	calls := 0
	control.AddOnStart(func() { calls++ })

	control.Start()
	control.Start()
	if calls != 1 {
		t.Fatalf("callbacks after duplicate Start() calls = %d, want 1", calls)
	}

	control.Stop()
	control.Start()
	if calls != 2 {
		t.Fatalf("callbacks after Stop()+Start() = %d, want 2", calls)
	}
}

func TestLoaderControlStatusReflectsRunningState(t *testing.T) {
	control := NewLoaderControl(false)
	if got := control.Status(); got != "stopped" {
		t.Fatalf("Status() before Start() = %q, want stopped", got)
	}

	control.Start()
	if got := control.Status(); got != "started" {
		t.Fatalf("Status() after Start() = %q, want started", got)
	}

	control.Stop()
	if got := control.Status(); got != "stopped" {
		t.Fatalf("Status() after Stop() = %q, want stopped", got)
	}
}

func TestBatchRatioManagerAdjustsPercentsFromDiffLimits(t *testing.T) {
	manager := NewBatchRatioManager(0.25, 0.15, true)
	cfg := config.BatchRatioConfig{
		ImpressionsDiffLeftSec:  -300,
		ImpressionsDiffRightSec: 300,
		ClicksDiffLeftSec:       -300,
		ClicksDiffRightSec:      300,
		AdjustFactor:            4,
	}

	impressionsPercent, clicksPercent, adjusted := manager.adjustFromTickerDiffs(301, -301, cfg)
	if !adjusted {
		t.Fatalf("adjustFromTickerDiffs() adjusted = false, want true")
	}
	if impressionsPercent != 1 || clicksPercent != 0.0375 {
		t.Fatalf("adjustFromTickerDiffs() = (%v, %v), want (1, 0.0375)", impressionsPercent, clicksPercent)
	}

	impressionsPercent, clicksPercent, adjusted = manager.adjustFromTickerDiffs(300, 300, cfg)
	if !adjusted {
		t.Fatalf("adjustFromTickerDiffs() adjusted = false, want true")
	}
	if impressionsPercent != 1 || clicksPercent != 0.0375 {
		t.Fatalf("adjustFromTickerDiffs() on equal limits = (%v, %v), want unchanged (1, 0.0375)", impressionsPercent, clicksPercent)
	}
}
