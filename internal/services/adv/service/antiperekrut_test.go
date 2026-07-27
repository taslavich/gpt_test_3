package auction

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTrafficHashPassIsDeterministic(t *testing.T) {
	first := trafficHashPass("request-1", "campaign-1", 250_000)
	for i := 0; i < 100; i++ {
		if got := trafficHashPass("request-1", "campaign-1", 250_000); got != first {
			t.Fatal("hash gate is not deterministic")
		}
	}
	if !trafficHashPass("request-1", "campaign-1", TrafficLimitFull) {
		t.Fatal("100% limit must always pass")
	}
	if trafficHashPass("", "campaign-1", TrafficLimitInitial) {
		t.Fatal("empty request hash id must fail closed")
	}
	if trafficHashPass("request-1", "", TrafficLimitInitial) {
		t.Fatal("empty campaign id must fail closed")
	}
}

func TestCalculateTrafficLimits(t *testing.T) {
	resetAt := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	spendAt := resetAt.Add(time.Minute)
	c1 := &Campaign{ID: "c1", UserID: "u1", Status: CampaignStatusActive}
	c2 := &Campaign{ID: "c2", UserID: "u1", Status: CampaignStatusActive}
	snapshot := &Snapshot{Campaigns: []*Campaign{c2, c1}, UserGoals: map[string]float64{"u1": 100}}
	state := &AntiPerekrutState{
		CampaignSpend: map[string]SpendPoint{
			"c1": {Spend: 1, CreatedAt: spendAt},
			"c2": {Spend: 1, CreatedAt: spendAt},
		},
		CampaignAuctionAllowed: map[string]bool{"c1": true, "c2": true},
		UserRemainingBalance:   map[string]float64{"u1": 100},
		TrafficLimit:           map[string]uint32{"c1": TrafficLimitInitial, "c2": TrafficLimitInitial},
		CampaignResetAppliedAt: map[string]time.Time{"c1": resetAt, "c2": resetAt},
	}
	calculateTrafficLimits(state, snapshot, nil)
	if state.TrafficLimit["c1"] != 300 || state.TrafficLimit["c2"] != 300 {
		t.Fatalf("limits=%v, both campaigns must increase exactly x3", state.TrafficLimit)
	}
}

func TestCalculateTrafficLimitsRequiresCompletePostResetSpend(t *testing.T) {
	resetAt := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	c1 := &Campaign{ID: "c1", UserID: "u1", Status: CampaignStatusActive}
	c2 := &Campaign{ID: "c2", UserID: "u1", Status: CampaignStatusActive}
	snapshot := &Snapshot{Campaigns: []*Campaign{c1, c2}, UserGoals: map[string]float64{"u1": 100}}
	state := &AntiPerekrutState{
		CampaignSpend: map[string]SpendPoint{
			"c1": {Spend: 1, CreatedAt: resetAt.Add(time.Minute)},
			// c2 is deliberately absent.
		},
		CampaignAuctionAllowed: map[string]bool{"c1": true, "c2": true},
		UserRemainingBalance:   map[string]float64{"u1": 100},
		TrafficLimit:           map[string]uint32{"c1": TrafficLimitInitial, "c2": TrafficLimitInitial},
		CampaignResetAppliedAt: map[string]time.Time{"c1": resetAt, "c2": resetAt},
	}
	calculateTrafficLimits(state, snapshot, nil)
	if state.TrafficLimit["c1"] != TrafficLimitInitial || state.TrafficLimit["c2"] != TrafficLimitInitial {
		t.Fatalf("incomplete spend must keep all user campaigns unchanged: %v", state.TrafficLimit)
	}
}

func TestCalculateTrafficLimitsStopsWhenBalanceIsInsufficient(t *testing.T) {
	resetAt := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	spendAt := resetAt.Add(time.Minute)
	c1 := &Campaign{ID: "c1", UserID: "u1", Status: CampaignStatusActive}
	c2 := &Campaign{ID: "c2", UserID: "u1", Status: CampaignStatusActive}
	snapshot := &Snapshot{Campaigns: []*Campaign{c1, c2}, UserGoals: map[string]float64{"u1": 5}}
	state := &AntiPerekrutState{
		CampaignSpend: map[string]SpendPoint{
			"c1": {Spend: 1, CreatedAt: spendAt}, "c2": {Spend: 1, CreatedAt: spendAt},
		},
		CampaignAuctionAllowed: map[string]bool{"c1": true, "c2": true},
		UserRemainingBalance:   map[string]float64{"u1": 5},
		TrafficLimit:           map[string]uint32{"c1": TrafficLimitInitial, "c2": TrafficLimitInitial},
		CampaignResetAppliedAt: map[string]time.Time{"c1": resetAt, "c2": resetAt},
	}
	calculateTrafficLimits(state, snapshot, nil)
	if state.TrafficLimit["c1"] != TrafficLimitInitial || state.TrafficLimit["c2"] != TrafficLimitInitial {
		t.Fatalf("insufficient balance must not increase current or following campaign: %v", state.TrafficLimit)
	}
}

func TestCalculateCampaignAuctionAllowedUsesUserGuardOncePerUser(t *testing.T) {
	snapshot := &Snapshot{
		Campaigns: []*Campaign{
			{ID: "c1", UserID: "u1", Status: CampaignStatusActive},
			{ID: "c2", UserID: "u1", Status: CampaignStatusActive},
			{ID: "c3", UserID: "u2", Status: CampaignStatusActive},
		},
		UserGoals: map[string]float64{"u1": 10, "u2": 10},
	}
	userSpend := map[string]SpendPoint{
		"u1": {Spend: 3},
		"u2": {Spend: 4},
	}
	runtimeSpent := map[string]float64{"u1": 4, "u2": 3}

	remaining, allowed := calculateCampaignAuctionAllowed(snapshot, userSpend, runtimeSpent)
	if remaining["u1"] != 6 || remaining["u2"] != 7 {
		t.Fatalf("unexpected remaining balances: %v", remaining)
	}
	if !allowed["c1"] || !allowed["c2"] {
		t.Fatalf("u1 campaigns must be allowed because 3*2 <= 6: %v", allowed)
	}
	if allowed["c3"] {
		t.Fatalf("u2 campaign must be blocked because 4*2 > 7: %v", allowed)
	}
}

func TestCalculateTrafficLimitsFreezesBlockedUser(t *testing.T) {
	resetAt := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	campaign := &Campaign{ID: "c1", UserID: "u1", Status: CampaignStatusActive}
	snapshot := &Snapshot{Campaigns: []*Campaign{campaign}, UserGoals: map[string]float64{"u1": 100}}
	state := &AntiPerekrutState{
		CampaignSpend:          map[string]SpendPoint{},
		CampaignAuctionAllowed: map[string]bool{"c1": false},
		UserRemainingBalance:   map[string]float64{"u1": 100},
		TrafficLimit:           map[string]uint32{"c1": 900},
		CampaignResetAppliedAt: map[string]time.Time{"c1": resetAt},
	}
	calculateTrafficLimits(state, snapshot, nil)
	if state.TrafficLimit["c1"] != 900 {
		t.Fatalf("blocked user traffic limit changed: %d", state.TrafficLimit["c1"])
	}
}

func TestCampaignAllowedFailsClosedWhenFlagIsMissing(t *testing.T) {
	manager := &AntiPerekrutManager{}
	campaign := &Campaign{ID: "c1"}
	state := &AntiPerekrutState{CampaignAuctionAllowed: map[string]bool{}}
	if manager.CampaignAllowed(state, campaign) {
		t.Fatal("missing campaign flag must fail closed")
	}
	state.CampaignAuctionAllowed["c1"] = true
	if !manager.CampaignAllowed(state, campaign) {
		t.Fatal("explicit true campaign flag must pass")
	}
}

func TestEffectiveTrafficLimitFailsClosedForUnappliedVersion(t *testing.T) {
	manager := &AntiPerekrutManager{}
	campaign := &Campaign{ID: "c1", TrafficResetVersion: 2}
	state := &AntiPerekrutState{
		TrafficLimit:                map[string]uint32{"c1": 90_000},
		AppliedCampaignResetVersion: map[string]int64{"c1": 1},
	}
	if got := manager.EffectiveTrafficLimit(state, campaign); got != TrafficLimitInitial {
		t.Fatalf("unapplied reset version limit=%d, want %d", got, TrafficLimitInitial)
	}
}

func TestApplyGlobalResetIsIdempotentAndResetsAllActiveCampaigns(t *testing.T) {
	now := time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)
	snapshot := &Snapshot{
		Campaigns: []*Campaign{
			{ID: "c1", UserID: "u1", Status: CampaignStatusActive, TrafficResetVersion: 4},
			{ID: "c2", UserID: "u2", Status: CampaignStatusActive, TrafficResetVersion: 2},
		},
		UserGoals: map[string]float64{"u1": 10, "u2": 10},
	}
	manager := &AntiPerekrutManager{snapshot: func() *Snapshot { return snapshot }}
	manager.state.Store(&AntiPerekrutState{
		UserSpend: map[string]SpendPoint{}, CampaignSpend: map[string]SpendPoint{},
		TrafficLimit:                map[string]uint32{"c1": 50_000, "c2": TrafficLimitFull},
		AppliedCampaignResetVersion: map[string]int64{"c1": 3, "c2": 2},
		CampaignResetAppliedAt:      map[string]time.Time{}, GlobalResetGeneration: 7, Generation: 10,
	})

	manager.applyGlobalReset(8, now)
	state := manager.State()
	if state.GlobalResetGeneration != 8 || state.Generation != 11 {
		t.Fatalf("generation state=%+v", state)
	}
	if state.TrafficLimit["c1"] != TrafficLimitInitial || state.TrafficLimit["c2"] != TrafficLimitInitial {
		t.Fatalf("global reset limits=%v", state.TrafficLimit)
	}
	if state.AppliedCampaignResetVersion["c1"] != 4 {
		t.Fatalf("campaign reset version=%d, want 4", state.AppliedCampaignResetVersion["c1"])
	}

	manager.applyGlobalReset(8, now.Add(time.Minute))
	afterDuplicate := manager.State()
	if afterDuplicate.Generation != state.Generation {
		t.Fatalf("same global generation must be idempotent: before=%d after=%d", state.Generation, afterDuplicate.Generation)
	}
}

func TestNotificationQueueHasNoFixedCapacityOrSilentDrop(t *testing.T) {
	manager := &AntiPerekrutManager{
		notify:     func(context.Context, string) error { return nil },
		notifyWake: make(chan struct{}, 1),
	}
	const total = 1024
	for i := 0; i < total; i++ {
		manager.NotifyAuctionError("test", errors.New("failure"))
	}
	manager.notifyMu.Lock()
	queued := len(manager.notifyQueue)
	manager.notifyMu.Unlock()
	if queued != total {
		t.Fatalf("queued notifications=%d, want %d", queued, total)
	}
}
