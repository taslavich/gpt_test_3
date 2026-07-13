package auction

import (
	"math"
	"testing"
)

func TestValidUserGoalRejectsMissingAndInvalidGoals(t *testing.T) {
	cases := []struct {
		name string
		id   string
		goal float64
		want bool
	}{
		{name: "valid", id: "user-1", goal: 1, want: true},
		{name: "missing user", id: " ", goal: 1, want: false},
		{name: "zero", id: "user-1", goal: 0, want: false},
		{name: "negative", id: "user-1", goal: -1, want: false},
		{name: "nan", id: "user-1", goal: math.NaN(), want: false},
		{name: "inf", id: "user-1", goal: math.Inf(1), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validUserGoal(tc.id, tc.goal); got != tc.want {
				t.Fatalf("validUserGoal(%q, %v)=%v, want %v", tc.id, tc.goal, got, tc.want)
			}
		})
	}
}

func TestCampaignsWithoutValidUserGoalAreExcluded(t *testing.T) {
	campaigns := map[string]*Campaign{
		"valid":   {ID: "valid", UserID: "user-valid", GoalTotalDollars: 50},
		"missing": {ID: "missing", UserID: "user-missing", GoalTotalDollars: 50},
	}
	goals := map[string]float64{"user-valid": 10}
	pruneCampaignsWithoutUserGoals(campaigns, goals)
	if _, ok := campaigns["valid"]; !ok {
		t.Fatal("campaign with valid user goal was removed")
	}
	if _, ok := campaigns["missing"]; ok {
		t.Fatal("campaign without valid user goal was not removed")
	}
}
