package notifications

import "testing"

func TestLowBalanceExpression(t *testing.T) {
	goal, spent, threshold := 10.0, 9.5, 1.0
	if !(goal-spent < threshold) {
		t.Fatal("expected low balance from goal-spent")
	}
}
