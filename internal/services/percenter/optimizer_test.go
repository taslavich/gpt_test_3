package percenter

import (
	"math"
	"testing"
	"time"
)

func testPolicy() Policy {
	return Policy{
		BuyoutRetention:        0.80,
		EfficiencyRetention:    0.80,
		DefaultMinMargin:       0.20,
		PromoMinMargin:         0.30,
		MaxMargin:              0.90,
		SSPSearchPrecision:     0.01,
		MarginSearchStepsPP:    []float64{10, 5, 2, 1},
		SSPReoptimizeInterval:  6 * time.Hour,
		MarginOptimizeInterval: 5 * time.Minute,
	}
}

func TestAdvanceWaitsFullDecisionInterval(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	state := BaselineState("hash", "campaign", 1, 0.30, 1, now)
	metrics := Metrics{SegmentHash: "hash", Requests: 100, Wins: 10}
	if _, changed := Advance(state, metrics, testPolicy(), now.Add(4*time.Minute)); changed {
		t.Fatal("state changed before the configured five-minute decision interval")
	}
	updated, changed := Advance(state, metrics, testPolicy(), now.Add(5*time.Minute))
	if !changed {
		t.Fatal("state did not advance after the configured decision interval")
	}
	if updated.PointVersion <= state.PointVersion {
		t.Fatalf("point version did not advance: %d -> %d", state.PointVersion, updated.PointVersion)
	}
}

func TestSparseSSPSearchIsNotResetBeforeItFinishes(t *testing.T) {
	policy := testPolicy()
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	state := BaselineState("hash", "campaign", 1, 0.30, 1, now)

	if !state.LastSSPReoptimizeAt.IsZero() {
		t.Fatalf("unfinished baseline must not start the 6h reoptimization clock: %v", state.LastSSPReoptimizeAt)
	}

	// The segment is extremely sparse and receives its first usable traffic a day
	// later. It must still record the benchmark and enter the SSP search instead
	// of being reset just because six hours elapsed since state creation.
	state, changed := Advance(state, Metrics{Requests: 1, Wins: 1}, policy, now.Add(24*time.Hour))
	if !changed || state.Phase != PhaseSSPSearch {
		t.Fatalf("sparse baseline did not advance into SSP search: %+v", state)
	}
	if !state.LastSSPReoptimizeAt.IsZero() {
		t.Fatalf("SSP reoptimization timestamp must remain zero while search is in progress: %v", state.LastSSPReoptimizeAt)
	}

	// Another rare sample can arrive well beyond six hours. The in-progress
	// binary search must continue rather than restart from the benchmark.
	previousPoint := state.PointVersion
	state, changed = Advance(state, Metrics{Requests: 1, Wins: 1}, policy, state.LastChangeAt.Add(24*time.Hour))
	if !changed {
		t.Fatalf("sparse SSP search stalled: %+v", state)
	}
	if state.PointVersion <= previousPoint {
		t.Fatalf("sparse SSP search did not move to the next test point: %d -> %d", previousPoint, state.PointVersion)
	}
	if state.Phase == PhaseBenchmark {
		t.Fatalf("sparse SSP search was incorrectly reset to benchmark: %+v", state)
	}
}

func TestCompletedSSPOptimizationStillReoptimizesAfterSixHours(t *testing.T) {
	policy := testPolicy()
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	state := State{
		SegmentHash:         "hash",
		CampaignID:          "campaign",
		CampaignVersion:     1,
		PointVersion:        9,
		OriginalBid:         1,
		MinMargin:           0.30,
		AdvertiserPrice:     0.80,
		SSPBid:              0.50,
		Margin:              0.375,
		Phase:               PhaseMarginSearch,
		LastChangeAt:        now.Add(-time.Hour),
		LastSSPReoptimizeAt: now.Add(-6 * time.Hour),
	}

	reset, changed := Advance(state, Metrics{Requests: 1, Wins: 1, AdvertiserSpend: 0.0008, TwinBidProfit: 0.0003}, policy, now)
	if !changed || reset.Phase != PhaseBenchmark {
		t.Fatalf("completed SSP cycle was not reset after six hours: %+v", reset)
	}
	if !reset.LastSSPReoptimizeAt.IsZero() {
		t.Fatalf("new SSP cycle must be marked unfinished after reset: %v", reset.LastSSPReoptimizeAt)
	}
	if reset.PointVersion <= state.PointVersion {
		t.Fatalf("reset did not advance point version: %d -> %d", state.PointVersion, reset.PointVersion)
	}
}

func TestSSPBinarySearchKeepsMarginFixedAndFindsPassingBoundary(t *testing.T) {
	policy := testPolicy()
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	state := BaselineState("hash", "campaign", 1, 0.30, 1, now)

	var changed bool
	state, changed = Advance(state, Metrics{Requests: 1000, Wins: 100}, policy, now.Add(5*time.Minute))
	if !changed || state.Phase != PhaseSSPSearch {
		t.Fatalf("benchmark did not start SSP search: %+v", state)
	}
	if math.Abs(state.Margin-0.30) > 1e-12 {
		t.Fatalf("SSP search changed min margin: %v", state.Margin)
	}
	// Drive a synthetic monotonic market: buyout passes at SSP >= 0.50 and fails below it.
	for i := 0; i < 20 && state.Phase == PhaseSSPSearch; i++ {
		wins := uint64(79)
		if state.SSPBid >= 0.50 {
			wins = 81
		}
		state, changed = Advance(state, Metrics{Requests: 1000, Wins: wins}, policy, state.LastChangeAt.Add(5*time.Minute))
		if !changed {
			t.Fatalf("SSP search stalled at iteration %d: %+v", i, state)
		}
		if state.Phase == PhaseSSPSearch && math.Abs(state.Margin-0.30) > 1e-12 {
			t.Fatalf("SSP search changed min margin: %v", state.Margin)
		}
	}
	if state.Phase != PhaseMarginBaseline {
		t.Fatalf("SSP search did not converge: %+v", state)
	}
	if state.SSPBid < 0.49 || state.SSPBid > 0.51 {
		t.Fatalf("SSP boundary=%v want approximately 0.50", state.SSPBid)
	}
}

func TestMarginSearchHonorsEfficiencyAndOriginalBid(t *testing.T) {
	policy := testPolicy()
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	state := State{
		SegmentHash:         "hash",
		CampaignID:          "campaign",
		CampaignVersion:     1,
		PointVersion:        1,
		OriginalBid:         1,
		MinMargin:           0.30,
		AdvertiserPrice:     0.7142857142857143,
		SSPBid:              0.50,
		Margin:              0.30,
		Phase:               PhaseMarginBaseline,
		LastChangeAt:        now,
		LastSSPReoptimizeAt: now,
	}
	// 1000 impressions at $0.714 CPM -> efficiency about 1400 impressions / dollar.
	baseline := Metrics{Requests: 10000, Wins: 1000, AdvertiserSpend: 1000 * state.AdvertiserPrice / 1000, TwinBidProfit: 0.2}
	state, changed := Advance(state, baseline, policy, now.Add(5*time.Minute))
	if !changed || state.Phase != PhaseMarginSearch {
		t.Fatalf("margin search did not start: %+v", state)
	}
	if state.Margin <= 0.30 {
		t.Fatalf("first hill-climb point did not increase margin: %v", state.Margin)
	}
	if state.AdvertiserPrice > state.OriginalBid+1e-12 {
		t.Fatalf("advertiser price exceeded original bid: %v > %v", state.AdvertiserPrice, state.OriginalBid)
	}

	// A point below the 80% efficiency guard cannot become the best point.
	bestBefore := state.BestMargin
	bad := Metrics{Requests: 10000, Wins: 1000, AdvertiserSpend: 1.1, TwinBidProfit: 1.0}
	state, changed = Advance(state, bad, policy, state.LastChangeAt.Add(5*time.Minute))
	if !changed {
		t.Fatal("hill-climb did not react to an invalid-efficiency point")
	}
	if state.BestMargin != bestBefore {
		t.Fatalf("invalid-efficiency point became best margin: %v -> %v", bestBefore, state.BestMargin)
	}
	if state.Margin > policy.MaxMargin+1e-12 || state.AdvertiserPrice > state.OriginalBid+1e-12 {
		t.Fatalf("margin search violated hard bounds: %+v", state)
	}
}
