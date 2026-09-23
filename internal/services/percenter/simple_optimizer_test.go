package percenter

import (
	"math"
	"testing"
	"time"
)

func simpleTestPolicy() SimplePolicy {
	return SimplePolicy{
		WinRateRetention:    0.50,
		MinImpressions:      5,
		OptimizeInterval:    5 * time.Minute,
		RebenchmarkInterval: 6 * time.Hour,
		SearchStepsPP:       []float64{5, 2, 1},
		MaxMargin:           0.90,
		StateTTL:            7 * 24 * time.Hour,
	}.Normalize()
}

func assertFloat(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %.12f want %.12f", got, want)
	}
}

func TestSimpleCandidateAtSixtyPercentOfBaselineCanBeAccepted(t *testing.T) {
	policy := simpleTestPolicy()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	state := NewSimpleState("segment", "campaign", 10, 0.20, false, policy, now.Add(-time.Hour))
	state.Phase = SimplePhaseSearch
	state.BaselineWinRate = 0.50
	state.LastConfirmedMargin = 0.20
	state.LastConfirmedProfit = 0.10
	state.Margin = 0.25
	state.SSPBid = priceAfterMargin(10, state.Margin)
	state.PointVersion = 2
	state.LastOptimizeAt = now.Add(-policy.OptimizeInterval)
	state.LastRebenchmarkAt = now.Add(-time.Hour)

	// observed=0.30 = 60% of the 0.50 baseline, above the 50% guard.
	metrics := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 30, TwinBidProfit: 20}
	next, changed := AdvanceSimple(state, metrics, policy, now)
	if !changed {
		t.Fatal("candidate above winrate guard with better profit must be accepted")
	}
	assertFloat(t, next.LastConfirmedMargin, 0.25)
	assertFloat(t, next.LastConfirmedProfit, 0.20)
	assertFloat(t, next.Margin, 0.30) // continue coarse 5pp search
	decision := next.DecisionHistory[len(next.DecisionHistory)-1]
	assertFloat(t, decision.TargetProfitPerReq, 0.10)
	assertFloat(t, decision.ActualProfitPerReq, 0.20)
	if next.PointVersion != 3 {
		t.Fatalf("point version=%d want 3", next.PointVersion)
	}
}

func TestSimpleCandidateAtFortyNinePercentOfBaselineRollsBack(t *testing.T) {
	policy := simpleTestPolicy()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	state := NewSimpleState("segment", "campaign", 10, 0.20, false, policy, now.Add(-time.Hour))
	state.Phase = SimplePhaseSearch
	state.BaselineWinRate = 0.50
	state.LastConfirmedMargin = 0.20
	state.LastConfirmedProfit = 0.10
	state.Margin = 0.25
	state.SSPBid = priceAfterMargin(10, state.Margin)
	state.PointVersion = 2
	state.LastOptimizeAt = now.Add(-policy.OptimizeInterval)
	state.LastRebenchmarkAt = now.Add(-time.Hour)

	// observed=0.245 = 49% of baseline. Profit is better, but guard must win.
	metrics := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 1000, Impressions: 245, TwinBidProfit: 200}
	next, changed := AdvanceSimple(state, metrics, policy, now)
	if !changed {
		t.Fatal("failed candidate must produce rollback state")
	}
	assertFloat(t, next.Margin, 0.20)
	if next.StepIndex != 1 || !next.AwaitingProbe {
		t.Fatalf("rollback must advance to 2pp step: step=%d awaiting=%v", next.StepIndex, next.AwaitingProbe)
	}
	if got := next.DecisionHistory[len(next.DecisionHistory)-1].Reason; got != "winrate_guard_rollback" {
		t.Fatalf("reason=%q", got)
	}
}

func TestSimpleStatisticalGuardAndFiveMinuteCadence(t *testing.T) {
	policy := simpleTestPolicy()
	t0 := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	state := NewSimpleState("segment", "campaign", 10, 0.20, false, policy, t0)

	for _, impressions := range []uint64{0, 1, 4} {
		metrics := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: impressions, TwinBidProfit: 10}
		if next, changed := AdvanceSimple(state, metrics, policy, t0.Add(5*time.Minute)); changed || next.PointVersion != state.PointVersion || next.Margin != state.Margin {
			t.Fatalf("%d impressions must not change state: %+v", impressions, next)
		}
	}

	metrics := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 5, TwinBidProfit: 10}
	next, changed := AdvanceSimple(state, metrics, policy, t0.Add(5*time.Minute))
	if !changed {
		t.Fatal("5 impressions must allow a decision")
	}
	assertFloat(t, next.Margin, 0.25)

	probeMetrics := SimpleMetrics{SegmentHash: next.SegmentHash, PointVersion: next.PointVersion, Requests: 100, Impressions: 5, TwinBidProfit: 11}
	if after, changed := AdvanceSimple(next, probeMetrics, policy, t0.Add(9*time.Minute)); changed || after.PointVersion != next.PointVersion {
		t.Fatal("state must not move before the 5m cadence elapses")
	}
}

func TestSimpleSearchUsesFiveTwoOneSteps(t *testing.T) {
	policy := simpleTestPolicy()
	t0 := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	state := NewSimpleState("segment", "campaign", 10, 0.20, false, policy, t0)

	baseline := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 50, TwinBidProfit: 10}
	state, changed := AdvanceSimple(state, baseline, policy, t0.Add(5*time.Minute))
	if !changed {
		t.Fatal("baseline window must start first probe")
	}
	assertFloat(t, state.Margin, 0.25)

	fail5 := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 20, TwinBidProfit: 9}
	state, changed = AdvanceSimple(state, fail5, policy, t0.Add(10*time.Minute))
	if !changed || state.StepIndex != 1 || !state.AwaitingProbe {
		t.Fatalf("5pp failure must roll back and select 2pp next: %+v", state)
	}
	assertFloat(t, state.Margin, 0.20)

	confirmedWindow := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 50, TwinBidProfit: 10}
	state, changed = AdvanceSimple(state, confirmedWindow, policy, t0.Add(15*time.Minute))
	if !changed {
		t.Fatal("2pp probe must be launched")
	}
	assertFloat(t, state.Margin, 0.22)

	fail2 := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 20, TwinBidProfit: 9}
	state, changed = AdvanceSimple(state, fail2, policy, t0.Add(20*time.Minute))
	if !changed || state.StepIndex != 2 || !state.AwaitingProbe {
		t.Fatalf("2pp failure must roll back and select 1pp next: %+v", state)
	}
	assertFloat(t, state.Margin, 0.20)

	confirmedWindow = SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 50, TwinBidProfit: 10}
	state, changed = AdvanceSimple(state, confirmedWindow, policy, t0.Add(25*time.Minute))
	if !changed {
		t.Fatal("1pp probe must be launched")
	}
	assertFloat(t, state.Margin, 0.21)
}

func TestSimpleSixHourRebenchmarkReturnsToFloor(t *testing.T) {
	policy := simpleTestPolicy()
	t0 := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	state := NewSimpleState("segment", "campaign", 10, 0.30, false, policy, t0)
	state.Phase = SimplePhaseSearch
	state.Margin = 0.45
	state.LastConfirmedMargin = 0.40
	state.BaselineWinRate = 0.5
	state.PointVersion = 7
	state.LastRebenchmarkAt = t0
	state.LastOptimizeAt = t0

	metrics := SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 50, TwinBidProfit: 10}
	next, changed := AdvanceSimple(state, metrics, policy, t0.Add(6*time.Hour))
	if !changed {
		t.Fatal("6h window must rebenchmark")
	}
	assertFloat(t, next.Margin, 0.30)
	if next.Phase != SimplePhaseBaseline || next.BaselineWinRate != 0 || next.PointVersion != 8 {
		t.Fatalf("unexpected rebenchmark state: %+v", next)
	}
}

func TestSimpleRepairRespectsFloorAndNinetyPercentCap(t *testing.T) {
	policy := simpleTestPolicy()
	now := time.Now().UTC()

	low := NewSimpleState("segment-low", "campaign", 10, 0.30, false, policy, now)
	low.Margin = 0.10
	low.LastConfirmedMargin = 0.10
	low.SSPBid = priceAfterMargin(10, low.Margin)
	low, changed := RepairSimpleState(low, 10, 0.30, false, policy, now.Add(time.Minute))
	if !changed {
		t.Fatal("margin below effective floor must be repaired")
	}
	assertFloat(t, low.Margin, 0.30)
	assertFloat(t, low.LastConfirmedMargin, 0.30)

	high := NewSimpleState("segment-high", "campaign", 10, 0.20, false, policy, now)
	high.Margin = 0.95
	high.LastConfirmedMargin = 0.95
	high.SSPBid = priceAfterMargin(10, high.Margin)
	high, changed = RepairSimpleState(high, 10, 0.20, false, policy, now.Add(time.Minute))
	if !changed {
		t.Fatal("margin above cap must be repaired")
	}
	assertFloat(t, high.Margin, 0.90)
	assertFloat(t, high.LastConfirmedMargin, 0.90)
}

func TestSimpleProbeClampsToNinetyPercentBoundary(t *testing.T) {
	policy := simpleTestPolicy()
	state := SimpleState{
		LastConfirmedMargin: 0.88,
		EffectiveMin:        0.20,
		MaxMargin:           0.90,
		StepIndex:           0, // 5pp would otherwise overshoot to 93%.
	}

	probe, ok := nextSimpleProbe(state, policy)
	if !ok {
		t.Fatal("90% boundary must be probed before Simple search settles")
	}
	assertFloat(t, probe, 0.90)

	state.LastConfirmedMargin = 0.90
	if probe, ok := nextSimpleProbe(state, policy); ok {
		t.Fatalf("search at the 90%% cap must settle, got extra probe %.12f", probe)
	}
}

func TestRTBSimpleStateIgnoresDynamicExternalRawBid(t *testing.T) {
	policy := simpleTestPolicy()
	now := time.Now().UTC()
	state := NewSimpleState("segment", "rtb-campaign", 10, 0.30, true, policy, now)
	state.Margin = 0.35

	if !state.Compatible("segment", "rtb-campaign", 100, 0.30, true, policy) {
		t.Fatal("RTB Simple state must remain compatible when only external raw bid changes")
	}
	pricing := state.PricingForOriginalBid(100)
	assertFloat(t, pricing.Margin, 0.35)
	assertFloat(t, pricing.SSPBid, 65)

	ordinary := NewSimpleState("segment2", "ordinary", 10, 0.20, false, policy, now)
	if ordinary.Compatible("segment2", "ordinary", 100, 0.20, false, policy) {
		t.Fatal("ordinary Simple state must remain tied to campaign original BasePrice")
	}
}

func TestSimpleExactSegmentDoesNotBorrowSiblingStatistics(t *testing.T) {
	policy := simpleTestPolicy()
	t0 := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	segmentA := NewSimpleState("segment-a", "campaign", 10, 0.20, false, policy, t0)
	segmentB := NewSimpleState("segment-b", "campaign", 10, 0.20, false, policy, t0)

	index := NewSimpleMetricsIndex([]SimpleMetrics{
		{SegmentHash: segmentA.SegmentHash, PointVersion: segmentA.PointVersion, Requests: 10, Impressions: 2, TwinBidProfit: 1},
		{SegmentHash: segmentB.SegmentHash, PointVersion: segmentB.PointVersion, Requests: 100, Impressions: 100, TwinBidProfit: 100},
	})

	metricA, ok := index.ForState(segmentA)
	if !ok {
		t.Fatal("exact metrics for segment A must be present")
	}
	if metricA.Impressions != 2 || metricA.Requests != 10 {
		t.Fatalf("segment A metrics were polluted by sibling statistics: %+v", metricA)
	}
	next, changed := AdvanceSimple(segmentA, metricA, policy, t0.Add(5*time.Minute))
	if changed || next.PointVersion != segmentA.PointVersion || next.Margin != segmentA.Margin {
		t.Fatalf("segment A state changed using only 2 exact impressions: before=%+v after=%+v", segmentA, next)
	}
}
