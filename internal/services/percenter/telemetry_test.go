package percenter

import "testing"

func TestTelemetryTotalsDeltaAndRestartReset(t *testing.T) {
	previous := TelemetryTotals{
		StateGetOrInitCalls:      100,
		BackgroundRetrySuccess:   20,
		FallbackRedisNil:         50,
		TelemetryPublishFailures: 3,
	}
	current := TelemetryTotals{
		StateGetOrInitCalls:      130,
		BackgroundRetrySuccess:   25,
		FallbackRedisNil:         10, // counter reset in a new process instance
		TelemetryPublishFailures: 4,
	}
	delta := current.Delta(previous)
	if delta.StateGetOrInitCalls != 30 {
		t.Fatalf("calls delta=%d want=30", delta.StateGetOrInitCalls)
	}
	if delta.BackgroundRetrySuccess != 5 {
		t.Fatalf("retry success delta=%d want=5", delta.BackgroundRetrySuccess)
	}
	if delta.FallbackRedisNil != 10 {
		t.Fatalf("reset counter delta=%d want current=10", delta.FallbackRedisNil)
	}
	if delta.TelemetryPublishFailures != 1 {
		t.Fatalf("publish failure delta=%d want=1", delta.TelemetryPublishFailures)
	}
}

func TestTelemetrySnapshotKeySeparatesProcessInstances(t *testing.T) {
	one := TelemetrySnapshot{Source: "adv", Hostname: "srv1", InstanceID: "one"}.Key()
	two := TelemetrySnapshot{Source: "adv", Hostname: "srv1", InstanceID: "two"}.Key()
	if one == two {
		t.Fatal("different process instances must never overwrite each other")
	}
}
