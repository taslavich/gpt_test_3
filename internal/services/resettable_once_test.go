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
