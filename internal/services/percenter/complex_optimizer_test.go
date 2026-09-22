package percenter

import (
	"math"
	"testing"
	"time"
)

func complexTestPolicy() ComplexPolicy {
	return ComplexPolicy{
		BuyoutRetention:       0.80,
		EfficiencyRetention:   0.80,
		MinImpressions:        5,
		OptimizeInterval:      5 * time.Minute,
		RebenchmarkInterval:   6 * time.Hour,
		SSPSearchStepsPercent: []float64{10, 5, 2, 1},
		MarginSearchStepsPP:   []float64{10, 5, 2, 1},
		MaxMargin:             0.90,
		StateTTL:              7 * 24 * time.Hour,
	}.Normalize()
}

func TestComplexStateMachineBenchmarkToSSPToMarginBaselineToMarginSearch(t *testing.T) {
	policy := complexTestPolicy()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	state := NewComplexState("segment", "campaign", 10, 0.20, policy, now.Add(-10*time.Minute))

	baseline := ComplexMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 50, AdvertiserSpend: 0.5, TwinBidProfit: 0.10}
	initialVersion := state.PointVersion
	state, changed := AdvanceComplex(state, baseline, policy, now)
	if !changed || state.Phase != ComplexPhaseSSPSearch || state.PointVersion != nextPointVersion(initialVersion) {
		t.Fatalf("benchmark transition failed: %+v changed=%t", state, changed)
	}
	if !approximatelyEqual(state.OriginalBid, 10) {
		t.Fatalf("SSP phase must keep original bid fixed: %+v", state)
	}
	if !approximatelyEqual(state.Margin, state.EffectiveMin) {
		t.Fatalf("SSP phase must keep margin fixed at effective_min: %+v", state)
	}
	if want := advertiserPriceForMargin(state.SSPBid, state.Margin); !approximatelyEqual(state.AdvertiserPrice, want) {
		t.Fatalf("SSP phase advertiser price=%v want SSP/(1-margin)=%v: %+v", state.AdvertiserPrice, want, state)
	}

	// Force the current SSP probe to be the final 1% step, then violate buyout
	// so the state machine must lock the last good SSP and enter margin baseline.
	state.SSPStepIndex = len(policy.SSPSearchStepsPercent) - 1
	state.LastGoodSSPBid = 8
	state.SSPBid = 7.92
	state.BaselineBuyout = 0.50
	state.LastConfirmedProfit = 0.001
	state.LastOptimizeAt = now.Add(-policy.OptimizeInterval)
	sspMetrics := ComplexMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 39, AdvertiserSpend: 0.5, TwinBidProfit: 0.20}
	state, changed = AdvanceComplex(state, sspMetrics, policy, now.Add(policy.OptimizeInterval))
	if !changed || state.Phase != ComplexPhaseMarginBaseline {
		t.Fatalf("SSP -> margin baseline transition failed: %+v changed=%t", state, changed)
	}
	if !approximatelyEqual(state.SSPBid, 8) || state.Margin < state.EffectiveMin-1e-12 {
		t.Fatalf("margin baseline must use last good SSP and effective floor: %+v", state)
	}
	if !approximatelyEqual(state.OriginalBid, 10) || !approximatelyEqual(state.AdvertiserPrice, advertiserPriceForMargin(state.SSPBid, state.Margin)) {
		t.Fatalf("margin-baseline transition must preserve original bid and derived advertiser price: %+v", state)
	}

	marginBaseline := ComplexMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 50, AdvertiserSpend: 0.5, TwinBidProfit: 0.10}
	state, changed = AdvanceComplex(state, marginBaseline, policy, now.Add(2*policy.OptimizeInterval))
	if !changed || state.Phase != ComplexPhaseMarginSearch {
		t.Fatalf("margin baseline -> margin search transition failed: %+v changed=%t", state, changed)
	}
	if !approximatelyEqual(state.SSPBid, 8) {
		t.Fatalf("margin search must keep SSP fixed: %+v", state)
	}
}

func TestComplexSSPBuyoutSeventyNinePercentRejectsAndEightyPercentAllows(t *testing.T) {
	policy := complexTestPolicy()
	now := time.Now().UTC()
	base := NewComplexState("segment", "campaign", 10, 0.20, policy, now.Add(-time.Hour))
	base.Phase = ComplexPhaseSSPSearch
	base.BaselineBuyout = 0.50
	base.LastGoodSSPBid = 8
	base.SSPBid = 7.2
	base.LastConfirmedProfit = 0.001
	base.PointVersion = 10

	rejectMetrics := ComplexMetrics{SegmentHash: base.SegmentHash, PointVersion: base.PointVersion, Requests: 1000, Impressions: 395, AdvertiserSpend: 1, TwinBidProfit: 2}
	rejected, changed := AdvanceComplex(base, rejectMetrics, policy, now)
	if !changed || !approximatelyEqual(rejected.SSPBid, 8) || !rejected.AwaitingProbe {
		t.Fatalf("79%% of baseline must roll back SSP candidate: %+v", rejected)
	}
	if !approximatelyEqual(rejected.Margin, rejected.EffectiveMin) || !approximatelyEqual(rejected.AdvertiserPrice, advertiserPriceForMargin(rejected.SSPBid, rejected.Margin)) {
		t.Fatalf("SSP rollback must preserve fixed margin and recompute advertiser price: %+v", rejected)
	}
	if got := rejected.DecisionHistory[len(rejected.DecisionHistory)-1]; got.BuyoutThresholdPassed || got.Reason != "ssp_buyout_guard_rollback" {
		t.Fatalf("unexpected 79%% decision: %+v", got)
	}

	retryMetrics := ComplexMetrics{SegmentHash: rejected.SegmentHash, PointVersion: rejected.PointVersion, Requests: 1000, Impressions: 500, AdvertiserSpend: 1, TwinBidProfit: 1}
	retry, retryChanged := AdvanceComplex(rejected, retryMetrics, policy, now.Add(policy.OptimizeInterval))
	if !retryChanged || !approximatelyEqual(retry.SSPBid, 7.6) {
		t.Fatalf("rollback must continue with the next smaller SSP step: %+v changed=%t", retry, retryChanged)
	}
	if !approximatelyEqual(retry.Margin, retry.EffectiveMin) || !approximatelyEqual(retry.AdvertiserPrice, advertiserPriceForMargin(retry.SSPBid, retry.Margin)) {
		t.Fatalf("smaller-step SSP probe must recompute advertiser price at fixed margin: %+v", retry)
	}

	allow := base
	allow.PointVersion = 20
	allow.LastConfirmedProfit = 0.01
	allow.AdvertiserPrice = advertiserPriceForMargin(allow.SSPBid, allow.EffectiveMin)
	allowMetrics := ComplexMetrics{SegmentHash: allow.SegmentHash, PointVersion: allow.PointVersion, Requests: 1000, Impressions: 400, AdvertiserSpend: 1, TwinBidProfit: 5}
	accepted, changed := AdvanceComplex(allow, allowMetrics, policy, now)
	if !changed || !(accepted.LastGoodSSPBid < 8) || !(accepted.SSPBid < accepted.LastGoodSSPBid) {
		t.Fatalf("80%% of baseline must allow SSP candidate even when normalized profit regresses: %+v", accepted)
	}
	if !approximatelyEqual(accepted.OriginalBid, allow.OriginalBid) || !approximatelyEqual(accepted.Margin, accepted.EffectiveMin) {
		t.Fatalf("accepted SSP candidate must preserve original bid and effective_min margin: %+v", accepted)
	}
	if want := advertiserPriceForMargin(accepted.SSPBid, accepted.Margin); !approximatelyEqual(accepted.AdvertiserPrice, want) {
		t.Fatalf("accepted SSP probe advertiser price=%v want=%v: %+v", accepted.AdvertiserPrice, want, accepted)
	}
	if got := accepted.DecisionHistory[len(accepted.DecisionHistory)-1]; !got.BuyoutThresholdPassed || got.ProfitImproved {
		t.Fatalf("80%% buyout threshold must pass independently of regressed profit: %+v", got)
	}
}

func TestComplexMarginEfficiencySeventyNineRejectsEightyAllows(t *testing.T) {
	policy := complexTestPolicy()
	now := time.Now().UTC()
	base := NewComplexState("segment", "campaign", 10, 0.20, policy, now.Add(-time.Hour))
	base.Phase = ComplexPhaseMarginSearch
	base.BaselineEfficiency = 100
	base.LastGoodSSPBid = 8
	base.SSPBid = 8
	base.LastGoodMargin = 0.20
	base.Margin = 0.30
	base.AdvertiserPrice = advertiserPriceForMargin(8, 0.30)
	base.LastConfirmedProfit = 0.001
	base.PointVersion = 10

	rejectMetrics := ComplexMetrics{SegmentHash: base.SegmentHash, PointVersion: base.PointVersion, Requests: 100, Impressions: 79, AdvertiserSpend: 1, TwinBidProfit: 1}
	rejected, changed := AdvanceComplex(base, rejectMetrics, policy, now)
	if !changed || !approximatelyEqual(rejected.Margin, 0.20) {
		t.Fatalf("79%% efficiency must roll back margin: %+v", rejected)
	}
	if got := rejected.DecisionHistory[len(rejected.DecisionHistory)-1]; got.EfficiencyThresholdPassed || got.Reason != "margin_efficiency_guard_rollback" {
		t.Fatalf("unexpected 79%% efficiency decision: %+v", got)
	}

	allow := base
	allow.PointVersion = 20
	allowMetrics := ComplexMetrics{SegmentHash: allow.SegmentHash, PointVersion: allow.PointVersion, Requests: 100, Impressions: 80, AdvertiserSpend: 1, TwinBidProfit: 1}
	accepted, changed := AdvanceComplex(allow, allowMetrics, policy, now)
	if !changed || accepted.LastGoodMargin < 0.30-1e-12 {
		t.Fatalf("80%% efficiency with improved profit must accept margin candidate: %+v", accepted)
	}
	if got := accepted.DecisionHistory[len(accepted.DecisionHistory)-1]; !got.EfficiencyThresholdPassed {
		t.Fatalf("80%% efficiency threshold must pass: %+v", got)
	}
}

func TestComplexMarginRollsBackWhenNormalizedProfitRegresses(t *testing.T) {
	policy := complexTestPolicy()
	now := time.Now().UTC()
	state := NewComplexState("segment", "campaign", 10, 0.20, policy, now.Add(-time.Hour))
	state.Phase = ComplexPhaseMarginSearch
	state.BaselineEfficiency = 50
	state.LastGoodSSPBid = 8
	state.SSPBid = 8
	state.LastGoodMargin = 0.20
	state.Margin = 0.30
	state.AdvertiserPrice = advertiserPriceForMargin(8, 0.30)
	state.LastConfirmedProfit = 0.02
	state.PointVersion = 7

	metrics := ComplexMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 50, AdvertiserSpend: 1, TwinBidProfit: 1}
	next, changed := AdvanceComplex(state, metrics, policy, now)
	if !changed || !approximatelyEqual(next.Margin, 0.20) {
		t.Fatalf("normalized profit regression must roll back: %+v", next)
	}
	if got := next.DecisionHistory[len(next.DecisionHistory)-1]; got.Reason != "margin_profit_regressed_rollback" || got.ProfitImproved {
		t.Fatalf("unexpected profit rollback history: %+v", got)
	}
}

func TestComplexGuardCadenceFloorCapAndRebenchmark(t *testing.T) {
	policy := complexTestPolicy()
	now := time.Now().UTC()
	state := NewComplexState("segment", "campaign", 10, 0.35, policy, now.Add(-time.Hour))
	if state.Margin < 0.35-1e-12 || state.Margin > 0.90+1e-12 {
		t.Fatalf("initial margin violates floor/cap: %+v", state)
	}

	tooSparse := ComplexMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 4, AdvertiserSpend: 1, TwinBidProfit: 1}
	if next, changed := AdvanceComplex(state, tooSparse, policy, now); changed || next.PointVersion != state.PointVersion {
		t.Fatalf("<5 impressions must not move state: %+v", next)
	}

	five := tooSparse
	five.Impressions = 5
	moved, changed := AdvanceComplex(state, five, policy, now)
	if !changed {
		t.Fatal("5 impressions must allow a decision")
	}
	if again, changed := AdvanceComplex(moved, ComplexMetrics{SegmentHash: moved.SegmentHash, PointVersion: moved.PointVersion, Requests: 100, Impressions: 50, AdvertiserSpend: 1, TwinBidProfit: 2}, policy, now.Add(time.Minute)); changed || again.PointVersion != moved.PointVersion {
		t.Fatalf("5m cadence must block early decision: %+v", again)
	}

	moved.LastRebenchmarkAt = now.Add(-policy.RebenchmarkInterval)
	moved.LastOptimizeAt = now.Add(-policy.OptimizeInterval)
	rebenchMetrics := ComplexMetrics{SegmentHash: moved.SegmentHash, PointVersion: moved.PointVersion, Requests: 100, Impressions: 50, AdvertiserSpend: 1, TwinBidProfit: 2}
	rebench, changed := AdvanceComplex(moved, rebenchMetrics, policy, now)
	if !changed || rebench.Phase != ComplexPhaseBenchmark || rebench.Margin < rebench.EffectiveMin-1e-12 || rebench.Margin > 0.90+1e-12 {
		t.Fatalf("6h rebenchmark must return to bounded benchmark: %+v", rebench)
	}
}

func TestComplexSearchStepsAndPointVersionAttribution(t *testing.T) {
	policy := complexTestPolicy()
	state := NewComplexState("segment", "campaign", 10, 0.20, policy, time.Now().UTC())
	wantSSP := []float64{7.2, 7.6, 7.84, 7.92}
	for i, want := range wantSSP {
		state.SSPStepIndex = i
		state.LastGoodSSPBid = 8
		got, ok := nextComplexSSPProbe(state, policy)
		if !ok || math.Abs(got-want) > 1e-12 {
			t.Fatalf("SSP step %d got=%v want=%v ok=%t", i, got, want, ok)
		}
	}
	wantMargin := []float64{0.30, 0.25, 0.22, 0.21}
	for i, want := range wantMargin {
		state.MarginStepIndex = i
		state.LastGoodMargin = 0.20
		got, ok := nextComplexMarginProbe(state, policy)
		if !ok || math.Abs(got-want) > 1e-12 {
			t.Fatalf("margin step %d got=%v want=%v ok=%t", i, got, want, ok)
		}
	}

	metrics := ComplexMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion + 1, Requests: 100, Impressions: 50, AdvertiserSpend: 1, TwinBidProfit: 1}
	if next, changed := AdvanceComplex(state, metrics, policy, time.Now().UTC()); changed || next.PointVersion != state.PointVersion {
		t.Fatal("statistics from another point_version must never drive Complex state")
	}
}

func TestComplexSSPProbeDoesNotUseMaxMarginAsLowerBound(t *testing.T) {
	policy := complexTestPolicy()
	state := NewComplexState("segment", "campaign", 10, 0.20, policy, time.Now().UTC())
	state.LastGoodSSPBid = 1.05
	state.SSPStepIndex = 0 // 10% relative step. MaxMargin must not clamp SSP search.

	got, ok := nextComplexSSPProbe(state, policy)
	if !ok {
		t.Fatal("SSP search must continue below the price implied by MaxMargin")
	}
	if !approximatelyEqual(got, 0.945) {
		t.Fatalf("unexpected SSP probe: got=%v want=0.945", got)
	}

	state.SSPBid = got
	state.Margin = state.EffectiveMin
	state.AdvertiserPrice = advertiserPriceForMargin(state.SSPBid, state.Margin)

	if !approximatelyEqual(state.OriginalBid, 10) {
		t.Fatalf("original bid changed during SSP search: got=%v want=10", state.OriginalBid)
	}
	if !approximatelyEqual(state.Margin, 0.20) {
		t.Fatalf("SSP search must keep margin at effective_min: got=%v want=0.20", state.Margin)
	}
	wantAdvertiserPrice := 0.945 / 0.8
	if !approximatelyEqual(state.AdvertiserPrice, wantAdvertiserPrice) {
		t.Fatalf("unexpected advertiser price: got=%v want=%v", state.AdvertiserPrice, wantAdvertiserPrice)
	}
}

func TestComplexMarginProbeClampsToMaxBoundary(t *testing.T) {
	policy := complexTestPolicy()
	state := NewComplexState("segment", "campaign", 10, 0.20, policy, time.Now().UTC())
	state.LastGoodMargin = 0.85
	state.MarginStepIndex = 0 // 10 percentage points would nominally produce 95%.

	got, ok := nextComplexMarginProbe(state, policy)
	if !ok || !approximatelyEqual(got, 0.90) {
		t.Fatalf("margin search must probe reachable max boundary: got=%v ok=%t", got, ok)
	}
}

func TestComplexFloorChangeReinitializesBelowOldPoints(t *testing.T) {
	policy := complexTestPolicy()
	now := time.Now().UTC()
	state := NewComplexState("segment", "campaign", 10, 0.20, policy, now.Add(-time.Hour))
	state.Phase = ComplexPhaseMarginSearch
	state.Margin = 0.25
	state.LastGoodMargin = 0.25
	state.PointVersion = 9

	repaired, changed := RepairComplexState(state, 10, 0.40, policy, now)
	if !changed || repaired.Phase != ComplexPhaseBenchmark || repaired.Margin < 0.40-1e-12 || repaired.PointVersion <= 9 {
		t.Fatalf("new percent-map/promo floor must invalidate old below-floor point: %+v", repaired)
	}
}

func TestComplexMetricsUseNormalizedProfit(t *testing.T) {
	metrics := ComplexMetrics{Requests: 200, Impressions: 50, AdvertiserSpend: 2, TwinBidProfit: 10}
	if got := metrics.ProfitPerRelevantOpportunity(); math.Abs(got-0.05) > 1e-12 {
		t.Fatalf("profit per relevant opportunity=%v want 0.05", got)
	}
	if got := metrics.Efficiency(); math.Abs(got-25) > 1e-12 {
		t.Fatalf("efficiency=%v want 25", got)
	}
}
