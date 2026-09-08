package main

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

func TestTelemetryDigestStatus(t *testing.T) {
	if got := telemetryDigestStatus(percenter.TelemetryTotals{}, percenter.TelemetryTotals{}); got != "HEALTHY" {
		t.Fatalf("empty status=%s want=HEALTHY", got)
	}
	if got := telemetryDigestStatus(percenter.TelemetryTotals{RequestDeadlineExceeded: 1}, percenter.TelemetryTotals{}); got != "WARN" {
		t.Fatalf("recovered request deadline status=%s want=WARN", got)
	}
	if got := telemetryDigestStatus(percenter.TelemetryTotals{BackgroundRetryFailed: 1}, percenter.TelemetryTotals{}); got != "DEGRADED" {
		t.Fatalf("failed retry status=%s want=DEGRADED", got)
	}
	if got := telemetryDigestStatus(percenter.TelemetryTotals{}, percenter.TelemetryTotals{FallbackRedisNil: 100}); got != "HEALTHY" {
		t.Fatalf("redis.Nil alone status=%s want=HEALTHY", got)
	}
}
