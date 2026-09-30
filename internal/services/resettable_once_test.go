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
	if impressionsPercent != 1 || clicksPercent != 0.15 {
		t.Fatalf("adjustFromTickerDiffs() = (%v, %v), want (1, 0.15)", impressionsPercent, clicksPercent)
	}

	impressionsPercent, clicksPercent, adjusted = manager.adjustFromTickerDiffs(300, 300, cfg)
	if !adjusted {
		t.Fatalf("adjustFromTickerDiffs() adjusted = false, want true")
	}
	if impressionsPercent != 1 || clicksPercent != 0.15 {
		t.Fatalf("adjustFromTickerDiffs() on equal limits = (%v, %v), want unchanged (1, 0.15)", impressionsPercent, clicksPercent)
	}
}

func TestBatchRatioManagerResetPercentsToDefaults(t *testing.T) {
	manager := NewBatchRatioManager(0.00001, 0.00002, true)
	cfg := config.BatchRatioConfig{
		ImpressionsDiffLeftSec:  -300,
		ImpressionsDiffRightSec: 300,
		ClicksDiffLeftSec:       -300,
		ClicksDiffRightSec:      300,
		AdjustFactor:            2,
	}

	manager.adjustFromTickerDiffs(301, 301, cfg)
	before := manager.State()
	if before.ImpressionsPercent == 0.00001 || before.ClicksPercent == 0.00002 {
		t.Fatalf("expected adjusted percents before reset, got %+v", before)
	}

	manager.ResetPercentsToDefaults()
	after := manager.State()
	if after.ImpressionsPercent != 0.00001 || after.ClicksPercent != 0.00002 {
		t.Fatalf("ResetPercentsToDefaults() = (%v, %v), want (0.00001, 0.00002)", after.ImpressionsPercent, after.ClicksPercent)
	}
}

func TestBatchRatioManagerDoesNotAdjustWhileLoaderStopped(t *testing.T) {
	manager := NewBatchRatioManager(0.25, 0.15, true)
	control := NewLoaderControl(false)
	cfg := config.BatchRatioConfig{
		ImpressionsDiffLeftSec:  -300,
		ImpressionsDiffRightSec: 300,
		ClicksDiffLeftSec:       -300,
		ClicksDiffRightSec:      300,
		AdjustFactor:            2,
	}

	impressionsPercent, clicksPercent, adjusted := manager.adjustFromTickerDiffsIfRunning(control, 301, -301, cfg)
	if adjusted {
		t.Fatal("adjustFromTickerDiffsIfRunning() adjusted while loader was stopped")
	}
	if impressionsPercent != 0.25 || clicksPercent != 0.15 {
		t.Fatalf("stopped loader changed percents to (%v, %v), want (0.25, 0.15)", impressionsPercent, clicksPercent)
	}

	control.Start()
	impressionsPercent, clicksPercent, adjusted = manager.adjustFromTickerDiffsIfRunning(control, 301, -301, cfg)
	if !adjusted {
		t.Fatal("adjustFromTickerDiffsIfRunning() did not adjust while loader was running")
	}
	if impressionsPercent != 0.5 || clicksPercent != 0.15 {
		t.Fatalf("running loader adjusted percents to (%v, %v), want (0.5, 0.15)", impressionsPercent, clicksPercent)
	}
}

func TestBatchRatioManagerAutoAdjustmentNeverDropsBelowDefaults(t *testing.T) {
	manager := NewBatchRatioManager(0.00001, 0.00002, true)
	cfg := config.BatchRatioConfig{
		ImpressionsDiffLeftSec:  -300,
		ImpressionsDiffRightSec: 300,
		ClicksDiffLeftSec:       -300,
		ClicksDiffRightSec:      300,
		AdjustFactor:            2,
	}

	// Move both ratios above their defaults, then repeatedly ask the automatic
	// regulator to decrease them. The configured defaults are hard lower floors.
	manager.adjustFromTickerDiffs(301, 301, cfg)
	for i := 0; i < 20; i++ {
		impressionsPercent, clicksPercent, adjusted := manager.adjustFromTickerDiffs(-301, -301, cfg)
		if !adjusted {
			t.Fatalf("adjustFromTickerDiffs() iteration %d adjusted = false, want true", i)
		}
		if impressionsPercent < 0.00001 {
			t.Fatalf("impressions percent dropped below default on iteration %d: %v", i, impressionsPercent)
		}
		if clicksPercent < 0.00002 {
			t.Fatalf("clicks percent dropped below default on iteration %d: %v", i, clicksPercent)
		}
	}

	state := manager.State()
	if state.ImpressionsPercent != 0.00001 || state.ClicksPercent != 0.00002 {
		t.Fatalf("auto-adjustment floor = (%v, %v), want defaults (0.00001, 0.00002)", state.ImpressionsPercent, state.ClicksPercent)
	}
}
