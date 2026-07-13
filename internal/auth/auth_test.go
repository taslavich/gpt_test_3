package auth

import (
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/models"
	"testing"
)

func TestAuthBalanceComputedFromGoalSpent(t *testing.T) {
	u := models.User{Goal: 3, Spent: 1}
	u.ComputeBalance()
	if u.Balance != 2 {
		t.Fatal(u.Balance)
	}
}
