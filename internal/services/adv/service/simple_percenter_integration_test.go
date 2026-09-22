package auction

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

func TestRTBSimpleFallbackUsesALLRTBAsEffectiveMinimum(t *testing.T) {
	store, err := NewPercentStore(filepath.Join(t.TempDir(), "percent_map.json"))
	if err != nil {
		t.Fatalf("NewPercentStore: %v", err)
	}
	service := &AuctionService{percents: store}
	campaign := &Campaign{ID: "rtb-simple-without-explicit-map", RTB: true, TypeModel: TypeModelSimple}

	decision, err := service.ResolvePricingDecision(campaign)
	if err != nil {
		t.Fatalf("ResolvePricingDecision: %v", err)
	}
	if math.Abs(decision.MapPercent-0.30) > 1e-12 || math.Abs(decision.MinMargin-0.30) > 1e-12 {
		t.Fatalf("RTB Simple fallback must resolve ALL_RTB=30%%: %+v", decision)
	}

	policy := (percenter.SimplePolicy{}).Normalize()
	t0 := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	state := percenter.NewSimpleState("segment-rtb", campaign.ID, 10, decision.MinMargin, true, policy, t0)
	if math.Abs(state.EffectiveMin-0.30) > 1e-12 || state.Margin < 0.30-1e-12 {
		t.Fatalf("initial Simple state escaped ALL_RTB floor: %+v", state)
	}

	// Baseline starts a legal upward probe. Search must never cross below the floor.
	baseline := percenter.SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 50, TwinBidProfit: 10}
	state, changed := percenter.AdvanceSimple(state, baseline, policy, t0.Add(5*time.Minute))
	if !changed || state.Margin < 0.30-1e-12 {
		t.Fatalf("Simple search crossed below ALL_RTB floor: %+v", state)
	}

	// A failed probe rolls back, but only to the confirmed 30%% floor.
	failedProbe := percenter.SimpleMetrics{SegmentHash: state.SegmentHash, PointVersion: state.PointVersion, Requests: 100, Impressions: 20, TwinBidProfit: 9}
	state, changed = percenter.AdvanceSimple(state, failedProbe, policy, t0.Add(10*time.Minute))
	if !changed || state.Margin < 0.30-1e-12 || state.LastConfirmedMargin < 0.30-1e-12 {
		t.Fatalf("Simple rollback crossed below ALL_RTB floor: %+v", state)
	}

	// Persisted/corrupt values below the floor are repaired back to 30%%.
	state.Margin = 0.10
	state.LastConfirmedMargin = 0.10
	repaired, changed := percenter.RepairSimpleState(state, 100, decision.MinMargin, true, policy, t0.Add(11*time.Minute))
	if !changed || repaired.Margin < 0.30-1e-12 || repaired.LastConfirmedMargin < 0.30-1e-12 || repaired.EffectiveMin < 0.30-1e-12 {
		t.Fatalf("Simple repair crossed below ALL_RTB floor: %+v", repaired)
	}
}
