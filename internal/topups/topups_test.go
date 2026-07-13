package topups

import "testing"

func TestDuplicateSentinel(t *testing.T) {
	if ErrTopupNotPending == nil {
		t.Fatal("missing duplicate/concurrent approve sentinel")
	}
}
