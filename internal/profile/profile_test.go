package profile

import (
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/models"
	"testing"
)

func TestBalanceComputedFromGoalSpent(t *testing.T) {
	u := models.User{Goal: 25, Spent: 7.5}
	u.ComputeBalance()
	if u.Balance != 17.5 {
		t.Fatalf("balance=%v", u.Balance)
	}
}
func TestPatchRequestFinancialFieldsAreOptional(t *testing.T) {
	req := PatchUserRequest{}
	if req.Balance != nil || req.Goal != nil || req.Spent != nil {
		t.Fatal("zero patch should not require financial fields")
	}
}
