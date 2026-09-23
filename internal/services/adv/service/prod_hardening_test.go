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

func TestPromoRuntimeShadowAppliesUserWideWithoutAuctionIO(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{
		PercentMapDefaultKey:    .18,
		PercentMapRTBDefaultKey: .24,
		"ordinary-high":         .45,
		"rtb-specific":          .25,
	})
	service := &AuctionService{percents: store}

	campaigns := []*Campaign{
		{ID: "ordinary-simple", UserID: "user-1", TypeModel: TypeModelSimple, PromoSpendRemaining: 5, PromoRevision: 10},
		{ID: "ordinary-high", UserID: "user-1", TypeModel: TypeModelMapOnly, PromoSpendRemaining: 5, PromoRevision: 10},
		{ID: "rtb-specific", UserID: "user-1", RTB: true, TypeModel: TypeModelComplex, PromoSpendRemaining: 5, PromoRevision: 10},
	}
	for _, campaign := range campaigns {
		decision, err := service.ResolvePricingDecision(campaign)
		if err != nil {
			t.Fatal(err)
		}
		if decision.Percent < PromoPercenterMinMargin {
			t.Fatalf("promo floor missing before runtime update: campaign=%s decision=%+v", campaign.ID, decision)
		}
	}

	if err := service.ApplyPromoSpendRemaining("user-1", 0, 11); err != nil {
		t.Fatal(err)
	}
	wants := map[string]float64{
		"ordinary-simple": .18,
		"ordinary-high":   .45,
		"rtb-specific":    .25,
	}
	for _, campaign := range campaigns {
		decision, err := service.ResolvePricingDecision(campaign)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(decision.Percent-wants[campaign.ID]) > 1e-12 {
			t.Fatalf("runtime promo exhaustion did not apply user-wide: campaign=%s decision=%+v want=%v", campaign.ID, decision, wants[campaign.ID])
		}
	}

	// A lower revision is stale even if its remaining value is higher.
	if err := service.ApplyPromoSpendRemaining("user-1", 3, 10); err != nil {
		t.Fatal(err)
	}
	if got := service.effectivePromoSpendRemaining(campaigns[0]); got != 0 {
		t.Fatalf("stale promo update resurrected remaining: got=%v", got)
	}
}

func TestPromoRevisionStaleSnapshotDoesNotRollbackBillingUpdate(t *testing.T) {
	service := NewAuctionService(nil, nil, nil, nil, nil)
	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 5, 10)); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyPromoSpendRemaining("user-1", 0, 11); err != nil {
		t.Fatal(err)
	}
	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 5, 10)); err != nil {
		t.Fatal(err)
	}

	state, ok := promoStateFromSnapshot(service.currentSnapshot(), "user-1")
	if !ok || state.Remaining != 0 || state.Revision != 11 {
		t.Fatalf("stale snapshot rolled promo state back: state=%+v present=%t", state, ok)
	}
	if raw, ok := service.promoRemainingOverrides.Load("user-1"); !ok || raw.(promoRuntimeState).Revision != 11 {
		t.Fatalf("newer billing shadow was not retained after stale snapshot: shadow=%#v present=%t", raw, ok)
	}
}

func TestPromoRevisionNewGrantAfterExhaustionApplies(t *testing.T) {
	service := NewAuctionService(nil, nil, nil, nil, nil)
	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 5, 10)); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyPromoSpendRemaining("user-1", 0, 11); err != nil {
		t.Fatal(err)
	}
	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 100, 12)); err != nil {
		t.Fatal(err)
	}

	state, ok := promoStateFromSnapshot(service.currentSnapshot(), "user-1")
	if !ok || state.Remaining != 100 || state.Revision != 12 {
		t.Fatalf("new promo grant was not applied: state=%+v present=%t", state, ok)
	}
	if _, ok := service.promoRemainingOverrides.Load("user-1"); ok {
		t.Fatal("newer authoritative snapshot should replace and clear the older runtime shadow")
	}
}

func TestPromoRevisionDuplicateUpdateIsIdempotent(t *testing.T) {
	service := NewAuctionService(nil, nil, nil, nil, nil)
	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 0, 11)); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyPromoSpendRemaining("user-1", 100, 12); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyPromoSpendRemaining("user-1", 100, 12); err != nil {
		t.Fatal(err)
	}

	raw, ok := service.promoRemainingOverrides.Load("user-1")
	if !ok {
		t.Fatal("versioned runtime promo shadow is missing")
	}
	state := raw.(promoRuntimeState)
	if state.Remaining != 100 || state.Revision != 12 {
		t.Fatalf("duplicate update changed state: %+v", state)
	}
}

func TestPromoRevisionOutOfOrderUpdatesUseRevisionNotRemaining(t *testing.T) {
	service := NewAuctionService(nil, nil, nil, nil, nil)
	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 0, 11)); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyPromoSpendRemaining("user-1", 100, 12); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyPromoSpendRemaining("user-1", 0, 11); err != nil {
		t.Fatal(err)
	}

	raw, ok := service.promoRemainingOverrides.Load("user-1")
	if !ok {
		t.Fatal("runtime promo shadow is missing")
	}
	state := raw.(promoRuntimeState)
	if state.Remaining != 100 || state.Revision != 12 {
		t.Fatalf("lower-revision update won because of remaining magnitude: %+v", state)
	}

	if err := service.ApplyPromoSpendRemaining("user-1", 1, 13); err != nil {
		t.Fatal(err)
	}
	state = mustPromoShadowForTest(t, service, "user-1")
	if state.Remaining != 1 || state.Revision != 13 {
		t.Fatalf("newer revision did not win when remaining decreased: %+v", state)
	}
}

func TestPromoRevisionSnapshotRefreshReconcilesShadowByRevision(t *testing.T) {
	service := NewAuctionService(nil, nil, nil, nil, nil)
	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 5, 10)); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyPromoSpendRemaining("user-1", 0, 11); err != nil {
		t.Fatal(err)
	}

	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 5, 10)); err != nil {
		t.Fatal(err)
	}
	if state := mustPromoShadowForTest(t, service, "user-1"); state.Revision != 11 || state.Remaining != 0 {
		t.Fatalf("stale snapshot incorrectly replaced runtime shadow: %+v", state)
	}

	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 0, 11)); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.promoRemainingOverrides.Load("user-1"); ok {
		t.Fatal("equal-revision snapshot did not clear caught-up runtime shadow")
	}

	if err := service.ApplyPromoSpendRemaining("user-1", 25, 12); err != nil {
		t.Fatal(err)
	}
	if err := service.PublishSnapshot(promoSnapshotForTest("user-1", 100, 13)); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.promoRemainingOverrides.Load("user-1"); ok {
		t.Fatal("newer snapshot did not replace older runtime shadow")
	}
	state, ok := promoStateFromSnapshot(service.currentSnapshot(), "user-1")
	if !ok || state.Remaining != 100 || state.Revision != 13 {
		t.Fatalf("newer snapshot state not published: state=%+v present=%t", state, ok)
	}
}

func promoSnapshotForTest(userID string, remaining float64, revision int64) *Snapshot {
	return &Snapshot{
		UserGoals:               map[string]float64{userID: 1},
		UserPromoSpendRemaining: map[string]float64{userID: remaining},
		UserPromoRevision:       map[string]int64{userID: revision},
	}
}

func mustPromoShadowForTest(t *testing.T, service *AuctionService, userID string) promoRuntimeState {
	t.Helper()
	raw, ok := service.promoRemainingOverrides.Load(userID)
	if !ok {
		t.Fatalf("promo runtime shadow for %s is missing", userID)
	}
	state, ok := raw.(promoRuntimeState)
	if !ok {
		t.Fatalf("promo runtime shadow for %s has unexpected type %T", userID, raw)
	}
	return state
}
