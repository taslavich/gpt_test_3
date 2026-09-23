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
		PercentMapDefaultKey:    .18,
		PercentMapRTBDefaultKey: .26,
		"ordinary-high":         .45,
		"rtb-high":              .50,
	})
	service := &AuctionService{percents: store}

	tests := []struct {
		name        string
		campaign    *Campaign
		wantMode    PercentRoutingMode
		wantPercent float64
		wantMin     float64
	}{
		{name: "ordinary simple promo lifts ALL", campaign: &Campaign{ID: "ordinary-simple", TypeModel: TypeModelSimple, PromoSpendRemaining: 1}, wantMode: PercentRoutingPercenterFloor, wantPercent: .30, wantMin: .30},
		{name: "ordinary complex promo lifts ALL", campaign: &Campaign{ID: "ordinary-complex", TypeModel: TypeModelComplex, PromoSpendRemaining: 1}, wantMode: PercentRoutingPercenterFloor, wantPercent: .30, wantMin: .30},
		{name: "ordinary map-only promo lifts ALL", campaign: &Campaign{ID: "ordinary-map", TypeModel: TypeModelMapOnly, PromoSpendRemaining: 1}, wantMode: PercentRoutingMapOnly, wantPercent: .30, wantMin: .30},
		{name: "ordinary map above promo wins", campaign: &Campaign{ID: "ordinary-high", TypeModel: TypeModelMapOnly, PromoSpendRemaining: 1}, wantMode: PercentRoutingMapOnly, wantPercent: .45, wantMin: .45},
		{name: "ordinary promo exhausted returns to ALL", campaign: &Campaign{ID: "ordinary-simple", TypeModel: TypeModelSimple}, wantMode: PercentRoutingPercenterFloor, wantPercent: .18, wantMin: .18},
		{name: "rtb simple promo lifts ALL_RTB", campaign: &Campaign{ID: "rtb-simple", RTB: true, TypeModel: TypeModelSimple, PromoSpendRemaining: 1}, wantMode: PercentRoutingPercenterFloor, wantPercent: .30, wantMin: .30},
		{name: "rtb complex fallback promo lifts ALL_RTB", campaign: &Campaign{ID: "rtb-complex", RTB: true, TypeModel: TypeModelComplex, PromoSpendRemaining: 1}, wantMode: PercentRoutingRTBComplexFallback, wantPercent: .30, wantMin: .30},
		{name: "rtb map-only promo lifts ALL_RTB", campaign: &Campaign{ID: "rtb-map", RTB: true, TypeModel: TypeModelMapOnly, PromoSpendRemaining: 1}, wantMode: PercentRoutingMapOnly, wantPercent: .30, wantMin: .30},
		{name: "rtb map above promo wins", campaign: &Campaign{ID: "rtb-high", RTB: true, TypeModel: TypeModelMapOnly, PromoSpendRemaining: 1}, wantMode: PercentRoutingMapOnly, wantPercent: .50, wantMin: .50},
		{name: "rtb promo exhausted returns to ALL_RTB", campaign: &Campaign{ID: "rtb-simple", RTB: true, TypeModel: TypeModelSimple}, wantMode: PercentRoutingPercenterFloor, wantPercent: .26, wantMin: .26},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.ResolvePricingDecision(tt.campaign)
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tt.wantMode || math.Abs(got.Percent-tt.wantPercent) > 1e-12 || math.Abs(got.MinMargin-tt.wantMin) > 1e-12 {
				t.Fatalf("promo routing mismatch: got=%+v want mode=%v percent=%v min=%v", got, tt.wantMode, tt.wantPercent, tt.wantMin)
			}
		})
	}
}
