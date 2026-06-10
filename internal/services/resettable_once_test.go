package services

import "testing"

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

func TestBatchRatioManagerUsesDefaultsWhenTickerReturnsZeros(t *testing.T) {
	manager := NewBatchRatioManager(0.25, 0.15, true)

	manager.updateFromTicker(0.4, 0.3)
	state := manager.State()
	if state.ImpressionsPercent != 0.4 || state.ClicksPercent != 0.3 {
		t.Fatalf("State() after non-zero ticker update = (%v, %v), want (0.4, 0.3)", state.ImpressionsPercent, state.ClicksPercent)
	}

	manager.updateFromTicker(0, 0)
	state = manager.State()
	if state.ImpressionsPercent != 0.25 || state.ClicksPercent != 0.15 {
		t.Fatalf("State() after zero ticker update = (%v, %v), want defaults (0.25, 0.15)", state.ImpressionsPercent, state.ClicksPercent)
	}
}
