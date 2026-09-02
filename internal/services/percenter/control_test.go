package percenter

import (
	"testing"
	"time"
)

func TestRebenchmarkStateUsesAutomaticResetForSimple(t *testing.T) {
	now := time.Date(2026, 8, 31, 20, 0, 0, 0, time.UTC)
	state := BaselineStateForCampaign("hash", "campaign", 1.25, 0.30, 7, TypeModelSimple, ProfitModelClick, now.Add(-time.Hour))
	state.PointVersion = 41
	state.Phase = PhaseSimpleMarginSearch
	state.Margin = 0.50
	state.SSPBid = state.OriginalBid * (1 - state.Margin)

	reset := RebenchmarkState(state, now)
	if reset.Phase != PhaseSimpleBaseline {
		t.Fatalf("phase=%s want %s", reset.Phase, PhaseSimpleBaseline)
	}
	if reset.PointVersion != 42 {
		t.Fatalf("point_version=%d want 42", reset.PointVersion)
	}
	if reset.TypeModel != TypeModelSimple || reset.ProfitModel != ProfitModelClick {
		t.Fatalf("campaign mode changed during rebenchmark: %+v", reset)
	}
	if reset.OriginalBid != state.OriginalBid || reset.MinMargin != state.MinMargin || reset.CampaignVersion != state.CampaignVersion {
		t.Fatalf("campaign identity changed during rebenchmark: %+v", reset)
	}
}

func TestRebenchmarkStateUsesAutomaticResetForSmart(t *testing.T) {
	now := time.Date(2026, 8, 31, 20, 0, 0, 0, time.UTC)
	state := BaselineStateForCampaign("hash", "campaign", 2, 0.20, 3, TypeModelSmart, ProfitModelImpression, now.Add(-time.Hour))
	state.PointVersion = 9
	state.Phase = PhaseMarginSearch
	state.LastSSPReoptimizeAt = now.Add(-6 * time.Hour)

	reset := RebenchmarkState(state, now)
	if reset.Phase != PhaseBenchmark {
		t.Fatalf("phase=%s want %s", reset.Phase, PhaseBenchmark)
	}
	if reset.PointVersion != 10 {
		t.Fatalf("point_version=%d want 10", reset.PointVersion)
	}
	if !reset.LastSSPReoptimizeAt.IsZero() {
		t.Fatalf("manual rebenchmark must start a fresh SSP cycle: %v", reset.LastSSPReoptimizeAt)
	}
}
