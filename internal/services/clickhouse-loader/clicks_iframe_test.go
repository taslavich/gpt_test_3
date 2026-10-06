package clickhouse_loader

import "testing"

func TestClickIframeNullable(t *testing.T) {
	if got := clickIframeNullable(0); got != nil {
		t.Fatalf("unknown status=%v want nil", got)
	}
	if got := clickIframeNullable(1); got != uint8(0) {
		t.Fatalf("no-iframe status=%v want 0", got)
	}
	if got := clickIframeNullable(2); got != uint8(1) {
		t.Fatalf("iframe status=%v want 1", got)
	}
	if got := clickIframeNullable(99); got != nil {
		t.Fatalf("invalid status=%v want nil", got)
	}
}
