package auction

import (
	"math"
	"testing"
)

func TestProductionRegressionMatrixRouting(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{
		PercentMapDefaultKey:    0.20,
		PercentMapRTBDefaultKey: 0.30,
	})
	service := &AuctionService{percents: store}

	tests := []struct {
		name        string
		campaign    *Campaign
		wantMode    PercentRoutingMode
		wantPercent float64
		wantMin     float64
		wantComplex bool
	}{
		{name: "ordinary simple", campaign: &Campaign{ID: "ordinary-simple", TypeModel: TypeModelSimple}, wantMode: PercentRoutingPercenterFloor, wantPercent: .20, wantMin: .20},
		{name: "ordinary complex", campaign: &Campaign{ID: "ordinary-complex", TypeModel: TypeModelComplex}, wantMode: PercentRoutingPercenterFloor, wantPercent: .20, wantMin: .20, wantComplex: true},
		{name: "ordinary map only", campaign: &Campaign{ID: "ordinary-map", TypeModel: TypeModelMapOnly}, wantMode: PercentRoutingMapOnly, wantPercent: .20, wantMin: .20},
		{name: "rtb simple", campaign: &Campaign{ID: "rtb-simple", TypeModel: TypeModelSimple, RTB: true}, wantMode: PercentRoutingPercenterFloor, wantPercent: .30, wantMin: .30},
		{name: "rtb complex bypass", campaign: &Campaign{ID: "rtb-complex", TypeModel: TypeModelComplex, RTB: true}, wantMode: PercentRoutingRTBComplexFallback, wantPercent: .30, wantMin: .30},
		{name: "rtb map only", campaign: &Campaign{ID: "rtb-map", TypeModel: TypeModelMapOnly, RTB: true}, wantMode: PercentRoutingMapOnly, wantPercent: .30, wantMin: .30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.ResolvePricingDecision(tt.campaign)
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tt.wantMode || math.Abs(got.Percent-tt.wantPercent) > 1e-12 || math.Abs(got.MinMargin-tt.wantMin) > 1e-12 {
				t.Fatalf("routing mismatch: got=%+v want mode=%v percent=%v min=%v", got, tt.wantMode, tt.wantPercent, tt.wantMin)
			}
			if shouldUseComplexPercenter(tt.campaign) != tt.wantComplex {
				t.Fatalf("Complex routing mismatch for %+v", tt.campaign)
			}
			if (got.Mode == PercentRoutingMapOnly || got.Mode == PercentRoutingRTBComplexFallback) && tt.wantComplex {
				t.Fatal("map-only/RTB+Complex bypass must never be routed into Complex state")
			}
		})
	}
}

func TestProductionRegressionMatrixPromoFloor(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{
		PercentMapDefaultKey:    .20,
		PercentMapRTBDefaultKey: .30,
	})
	service := &AuctionService{percents: store}

	for _, model := range []int{TypeModelSimple, TypeModelComplex} {
		active := &Campaign{ID: "promo-active", TypeModel: model, PromoSpendRemaining: 1}
		got, err := service.ResolvePricingDecision(active)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got.MinMargin-.30) > 1e-12 {
			t.Fatalf("type_model=%d active promo floor=%v want .30", model, got.MinMargin)
		}

		exhausted := &Campaign{ID: "promo-exhausted", TypeModel: model, PromoSpendRemaining: 0}
		got, err = service.ResolvePricingDecision(exhausted)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got.MinMargin-.20) > 1e-12 {
			t.Fatalf("type_model=%d exhausted promo floor=%v want .20", model, got.MinMargin)
		}
	}

	mapOnly := &Campaign{ID: "map-only-promo", TypeModel: TypeModelMapOnly, PromoSpendRemaining: 100}
	got, err := service.ResolvePricingDecision(mapOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != PercentRoutingMapOnly || math.Abs(got.Percent-.20) > 1e-12 {
		t.Fatalf("type_model=3 must remain exact map-only despite promo: %+v", got)
	}
}
