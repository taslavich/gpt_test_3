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
	calculateTrafficLimits(state, snapshot, nil, spendAt)
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
	calculateTrafficLimits(state, snapshot, nil, resetAt.Add(time.Minute))
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
	calculateTrafficLimits(state, snapshot, nil, spendAt)
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
	calculateTrafficLimits(state, snapshot, nil, resetAt.Add(time.Minute))
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
	if got := manager.EffectiveTrafficLimit(state, campaign, time.Now().UTC()); got != TrafficLimitInitial {
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

func TestScheduleBreakFreezesTrafficLimit(t *testing.T) {
	blockStart := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	campaign := &Campaign{
		ID: "c1", UserID: "u1", Status: CampaignStatusActive,
		StartTS: blockStart.Add(-time.Hour), EndTS: blockStart.Add(12 * time.Hour),
		ActiveIntervals: []TimeRange{{Start: blockStart, End: blockStart.Add(2 * time.Hour)}},
	}
	snapshot := &Snapshot{Campaigns: []*Campaign{campaign}, UserGoals: map[string]float64{"u1": 100}}
	state := &AntiPerekrutState{
		CampaignSpend:               map[string]SpendPoint{"c1": {Spend: 0, CreatedAt: blockStart.Add(3 * time.Hour)}},
		CampaignAuctionAllowed:      map[string]bool{"c1": true},
		CampaignActiveIntervalStart: map[string]time.Time{"c1": blockStart},
		UserRemainingBalance:        map[string]float64{"u1": 100},
		TrafficLimit:                map[string]uint32{"c1": 560_000},
		CampaignResetAppliedAt:      map[string]time.Time{"c1": blockStart},
	}

	calculateTrafficLimits(state, snapshot, nil, blockStart.Add(3*time.Hour))
	if state.TrafficLimit["c1"] != 560_000 {
		t.Fatalf("schedule break changed traffic limit: %d", state.TrafficLimit["c1"])
	}
}

func TestNewScheduleBlockResetsAndMayGrowOnSameTick(t *testing.T) {
	firstStart := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	secondStart := time.Date(2026, 7, 28, 7, 0, 0, 0, time.UTC)
	tickAt := secondStart.Add(8 * time.Second)
	campaign := &Campaign{
		ID: "c1", UserID: "u1", Status: CampaignStatusActive,
		StartTS: firstStart.Add(-time.Hour), EndTS: secondStart.Add(4 * time.Hour),
		ActiveIntervals: []TimeRange{
			{Start: firstStart, End: firstStart.Add(2 * time.Hour)},
			{Start: secondStart, End: secondStart.Add(3 * time.Hour)},
		},
	}
	snapshot := &Snapshot{Campaigns: []*Campaign{campaign}, UserGoals: map[string]float64{"u1": 100}}
	state := &AntiPerekrutState{
		CampaignSpend:               map[string]SpendPoint{"c1": {Spend: 0, CreatedAt: secondStart.Add(5 * time.Second)}},
		CampaignAuctionAllowed:      map[string]bool{"c1": true},
		CampaignActiveIntervalStart: map[string]time.Time{"c1": firstStart},
		UserRemainingBalance:        map[string]float64{"u1": 100},
		TrafficLimit:                map[string]uint32{"c1": 560_000},
		AppliedCampaignResetVersion: map[string]int64{},
		CampaignResetAppliedAt:      map[string]time.Time{"c1": firstStart},
	}

	resetCampaigns := applyCampaignStateTransitions(state, snapshot, tickAt)
	if got := state.CampaignActiveIntervalStart["c1"]; !got.Equal(secondStart) {
		t.Fatalf("active interval start=%s, want %s", got, secondStart)
	}
	if state.TrafficLimit["c1"] != TrafficLimitInitial {
		t.Fatalf("new block did not reset to initial: %d", state.TrafficLimit["c1"])
	}
	if _, blockedForTick := resetCampaigns["c1"]; blockedForTick {
		t.Fatal("schedule reset must not suppress growth on the +8s tick")
	}

	calculateTrafficLimits(state, snapshot, resetCampaigns, tickAt)
	if state.TrafficLimit["c1"] != 300 {
		t.Fatalf("same-tick limit=%d, want 300", state.TrafficLimit["c1"])
	}
}

func TestEffectiveTrafficLimitUsesInitialBeforeScheduleTick(t *testing.T) {
	firstStart := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	secondStart := time.Date(2026, 7, 28, 7, 0, 0, 0, time.UTC)
	campaign := &Campaign{
		ID: "c1",
		ActiveIntervals: []TimeRange{
			{Start: firstStart, End: firstStart.Add(2 * time.Hour)},
			{Start: secondStart, End: secondStart.Add(3 * time.Hour)},
		},
	}
	state := &AntiPerekrutState{
		TrafficLimit:                map[string]uint32{"c1": 560_000},
		AppliedCampaignResetVersion: map[string]int64{},
		CampaignActiveIntervalStart: map[string]time.Time{"c1": firstStart},
	}
	manager := &AntiPerekrutManager{}

	if got := manager.EffectiveTrafficLimit(state, campaign, secondStart); got != TrafficLimitInitial {
		t.Fatalf("first request of new block limit=%d, want %d", got, TrafficLimitInitial)
	}
	state.CampaignActiveIntervalStart["c1"] = secondStart
	state.TrafficLimit["c1"] = 300
	if got := manager.EffectiveTrafficLimit(state, campaign, secondStart.Add(8*time.Second)); got != 300 {
		t.Fatalf("post-tick limit=%d, want 300", got)
	}
}

func TestNormalizeActiveIntervalsMergesTouchingRanges(t *testing.T) {
	start := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	got := normalizeActiveIntervals([]TimeRange{
		{Start: start.Add(2 * time.Hour), End: start.Add(4 * time.Hour)},
		{Start: start, End: start.Add(2 * time.Hour)},
	})
	if len(got) != 1 || !got[0].Start.Equal(start) || !got[0].End.Equal(start.Add(4*time.Hour)) {
		t.Fatalf("touching ranges were not merged: %#v", got)
	}
}
