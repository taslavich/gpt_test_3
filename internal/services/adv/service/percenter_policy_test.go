package auction

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func newPercentStoreForPolicyTest(t *testing.T, values PercentMap) *PercentStore {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "adv_percent_map.json")
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewPercentStore(filename)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestPercentStorePersistsDefaultsAndExpandsGroupedCampaignKeys(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{"123, 456,789": 0.25})

	saved, err := store.Saved()
	if err != nil {
		t.Fatal(err)
	}
	if saved[PercentMapDefaultKey] != DefaultADVPercent {
		t.Fatalf("ALL=%v want %v", saved[PercentMapDefaultKey], DefaultADVPercent)
	}
	if saved[PercentMapRTBDefaultKey] != DefaultADVRTBPercent {
		t.Fatalf("ALL_RTB=%v want %v", saved[PercentMapRTBDefaultKey], DefaultADVRTBPercent)
	}
	if saved["123, 456,789"] != 0.25 {
		t.Fatalf("grouped key was not preserved: %#v", saved)
	}
	for _, id := range []string{"123", "456", "789"} {
		if got := store.LookupForCampaign(id, false); got != 0.25 {
			t.Fatalf("campaign %s percent=%v want 0.25", id, got)
		}
	}

	raw, err := os.ReadFile(store.filename)
	if err != nil {
		t.Fatal(err)
	}
	var persisted PercentMap
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted[PercentMapDefaultKey] != 0.20 || persisted[PercentMapRTBDefaultKey] != 0.30 {
		t.Fatalf("defaults were not physically persisted: %#v", persisted)
	}
}

func TestPercentStoreKeepsExplicitDefaults(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{
		PercentMapDefaultKey:    0.17,
		PercentMapRTBDefaultKey: 0.34,
	})
	saved, err := store.Saved()
	if err != nil {
		t.Fatal(err)
	}
	if saved[PercentMapDefaultKey] != 0.17 || saved[PercentMapRTBDefaultKey] != 0.34 {
		t.Fatalf("explicit defaults were overwritten: %#v", saved)
	}
	if got := store.LookupForCampaign("missing", false); got != 0.17 {
		t.Fatalf("ordinary explicit ALL=%v want 0.17", got)
	}
	if got := store.LookupForCampaign("missing", true); got != 0.34 {
		t.Fatalf("RTB explicit ALL_RTB=%v want 0.34", got)
	}
}

func TestPercentStoreRejectsMarginAboveNinetyPercent(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "adv_percent_map.json")
	if err := os.WriteFile(filename, []byte(`{"campaign-1":0.91}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPercentStore(filename); err == nil {
		t.Fatal("percent above 90% must be rejected")
	}
}

func TestPricingDecisionFloorsAndMapOnly(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{
		PercentMapDefaultKey:    0.20,
		PercentMapRTBDefaultKey: 0.30,
		"simple":                0.27,
		"smart-low":             0.20,
		"smart-high":            0.40,
		"map-only":              0.10,
		"rtb-complex":           0.25,
	})
	service := &AuctionService{percents: store}

	tests := []struct {
		name        string
		campaign    *Campaign
		wantMode    PercentRoutingMode
		wantMin     float64
		wantPercent float64
	}{
		{name: "simple map floor", campaign: &Campaign{ID: "simple", TypeModel: TypeModelSimple}, wantMode: PercentRoutingPercenterFloor, wantMin: 0.27, wantPercent: 0.27},
		{name: "simple promo floor", campaign: &Campaign{ID: "missing-simple", TypeModel: TypeModelSimple, PromoSpendRemaining: 100}, wantMode: PercentRoutingPercenterFloor, wantMin: 0.30, wantPercent: 0.30},
		{name: "smart promo floor", campaign: &Campaign{ID: "smart-low", TypeModel: TypeModelComplex, PromoSpendRemaining: 1}, wantMode: PercentRoutingPercenterFloor, wantMin: 0.30, wantPercent: 0.30},
		{name: "smart map above promo", campaign: &Campaign{ID: "smart-high", TypeModel: TypeModelComplex, PromoSpendRemaining: 1}, wantMode: PercentRoutingPercenterFloor, wantMin: 0.40, wantPercent: 0.40},
		{name: "smart promo exhausted", campaign: &Campaign{ID: "smart-low", TypeModel: TypeModelComplex, PromoSpendRemaining: 0}, wantMode: PercentRoutingPercenterFloor, wantMin: 0.20, wantPercent: 0.20},
		{name: "map only exact ignores promo floor", campaign: &Campaign{ID: "map-only", TypeModel: TypeModelMapOnly, PromoSpendRemaining: 100}, wantMode: PercentRoutingMapOnly, wantMin: 0.10, wantPercent: 0.10},
		{name: "rtb simple ALL_RTB floor", campaign: &Campaign{ID: "rtb-simple", TypeModel: TypeModelSimple, RTB: true}, wantMode: PercentRoutingPercenterFloor, wantMin: 0.30, wantPercent: 0.30},
		{name: "rtb complex exact fallback", campaign: &Campaign{ID: "rtb-complex", TypeModel: TypeModelComplex, RTB: true, PromoSpendRemaining: 10}, wantMode: PercentRoutingRTBComplexFallback, wantMin: 0.25, wantPercent: 0.25},
		{name: "rtb map only exact", campaign: &Campaign{ID: "rtb-map", TypeModel: TypeModelMapOnly, RTB: true}, wantMode: PercentRoutingMapOnly, wantMin: 0.30, wantPercent: 0.30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.ResolvePricingDecision(tt.campaign)
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tt.wantMode || math.Abs(got.MinMargin-tt.wantMin) > 1e-12 || math.Abs(got.Percent-tt.wantPercent) > 1e-12 {
				t.Fatalf("decision=%+v want mode=%v min=%v percent=%v", got, tt.wantMode, tt.wantMin, tt.wantPercent)
			}
			if got.MaxMargin != 0.90 {
				t.Fatalf("MaxMargin=%v want 0.90", got.MaxMargin)
			}
		})
	}
}

func TestPercenterSegmentHashIsStableAndCampaignAware(t *testing.T) {
	country, osName, uaValue, siteID := "US", "Android", "Mozilla/5.0 (Linux; Android 14) Chrome/120.0 Mobile", "site-1"
	req := &ortb.BidRequest{
		Device: &ortb.Device{Geo: &ortb.Geo{Country: &country}, Os: &osName, Ua: &uaValue},
		Site:   &ortb.Site{Id: &siteID},
	}
	first := BuildPercenterSegmentHash(req, "ssp.example", "campaign-a")
	second := BuildPercenterSegmentHash(req, "ssp.example", "campaign-a")
	otherCampaign := BuildPercenterSegmentHash(req, "ssp.example", "campaign-b")
	if first == "" || first != second {
		t.Fatalf("segment hash must be stable: first=%q second=%q", first, second)
	}
	if first == otherCampaign {
		t.Fatal("final segment hash must include campaign_id")
	}

	missing := BuildPercenterSegment(nil, "", "campaign-a")
	if missing.SSPDomain != "" || missing.Geo != UnknownSegmentValue || missing.Browser != UnknownSegmentValue || missing.Device != UnknownSegmentValue || missing.OS != UnknownSegmentValue || missing.SiteID != UnknownSegmentValue {
		t.Fatalf("nil request fields must use %q while explicit string arguments preserve empty: %+v", UnknownSegmentValue, missing)
	}
}

func TestPercenterSegmentHashDistinguishesMissingFromExplicitEmpty(t *testing.T) {
	empty := ""
	missingReq := &ortb.BidRequest{
		Device: &ortb.Device{Geo: &ortb.Geo{}},
		Site:   &ortb.Site{},
	}
	emptyReq := &ortb.BidRequest{
		Device: &ortb.Device{
			Geo: &ortb.Geo{Country: &empty},
			Ua:  &empty,
			Os:  &empty,
		},
		Site: &ortb.Site{Id: &empty},
	}

	missing := BuildPercenterSegment(missingReq, "ssp.example", "campaign-a")
	explicitEmpty := BuildPercenterSegment(emptyReq, "ssp.example", "campaign-a")
	if missing.Geo != UnknownSegmentValue || missing.Browser != UnknownSegmentValue || missing.Device != UnknownSegmentValue || missing.OS != UnknownSegmentValue || missing.SiteID != UnknownSegmentValue {
		t.Fatalf("missing/nil fields must use %q: %+v", UnknownSegmentValue, missing)
	}
	if explicitEmpty.Geo != "" || explicitEmpty.Browser != "" || explicitEmpty.Device != "" || explicitEmpty.OS != "" || explicitEmpty.SiteID != "" {
		t.Fatalf("explicit empty fields must remain empty: %+v", explicitEmpty)
	}
	if missing.Hash() == explicitEmpty.Hash() {
		t.Fatalf("missing and explicit-empty segment hashes must differ: %q", missing.Hash())
	}
}

func TestRTBWinnerUsesRawOriginalBidWhenEffectivePriceOrderIsReversed(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{
		PercentMapDefaultKey:    0.20,
		PercentMapRTBDefaultKey: 0.30,
		"rtb-high-raw":          0.60,
		"rtb-high-effective":    0.10,
	})
	service := &AuctionService{percents: store}
	highRawCampaign := &Campaign{ID: "rtb-high-raw", RTB: true, TypeModel: TypeModelMapOnly}
	highEffectiveCampaign := &Campaign{ID: "rtb-high-effective", RTB: true, TypeModel: TypeModelMapOnly}

	highRawPricing, err := service.ResolvePricingDecision(highRawCampaign)
	if err != nil {
		t.Fatal(err)
	}
	highEffectivePricing, err := service.ResolvePricingDecision(highEffectiveCampaign)
	if err != nil {
		t.Fatal(err)
	}

	highRaw := candidate{
		campaign:       highRawCampaign,
		basePrice:      10,
		originalBid:    10,
		effectivePrice: CalculateEffectiveAuctionPrice(10, highRawPricing.Percent),
	}
	highEffective := candidate{
		campaign:       highEffectiveCampaign,
		basePrice:      9,
		originalBid:    9,
		effectivePrice: CalculateEffectiveAuctionPrice(9, highEffectivePricing.Percent),
	}
	if highRaw.effectivePrice >= highEffective.effectivePrice {
		t.Fatalf("test setup must reverse effective-price order: high-raw=%v high-effective=%v", highRaw.effectivePrice, highEffective.effectivePrice)
	}

	pool := prepareCandidatePool([]candidate{highEffective, highRaw}, auctionModeMaxBid, nil)
	if len(pool) != 2 || candidateCampaignID(pool[0]) != highRawCampaign.ID {
		t.Fatalf("RTB max-bid winner must use raw original bid, pool=%+v", pool)
	}
	if got := weightedCandidateIndex([]candidate{highRaw, highEffective}, 0.50); got != 0 {
		t.Fatalf("weighted selection must use raw original bids; index=%d want 0", got)
	}
}

func TestCandidateSelectionUsesOriginalBidNotOptimizedPrice(t *testing.T) {
	higherOriginal := candidate{campaign: &Campaign{ID: "higher-original", BasePrice: 10}, basePrice: 10, originalBid: 10, effectivePrice: 6}
	higherEffective := candidate{campaign: &Campaign{ID: "higher-effective", BasePrice: 9}, basePrice: 9, originalBid: 9, effectivePrice: 8}
	pool := prepareCandidatePool([]candidate{higherEffective, higherOriginal}, auctionModeMaxBid, nil)
	if len(pool) != 2 || candidateCampaignID(pool[0]) != "higher-original" {
		t.Fatalf("max-bid winner must follow original bid: %+v", pool)
	}
}

func TestWeightedSelectionUsesOriginalBid(t *testing.T) {
	candidates := []candidate{
		{campaign: &Campaign{ID: "high-original", BasePrice: 100}, basePrice: 100, originalBid: 100, effectivePrice: 1},
		{campaign: &Campaign{ID: "high-effective", BasePrice: 10}, basePrice: 10, originalBid: 10, effectivePrice: 100},
	}

	top := prepareCandidatePool(candidates, auctionModeWeightedTop, nil)
	if len(top) != 1 || candidateCampaignID(top[0]) != "high-original" {
		t.Fatalf("weighted-top pool must be based on original bid: %+v", top)
	}
	if got := weightedCandidateIndex(candidates, 0.50); got != 0 {
		t.Fatalf("weighted-all selection must weight original bids, index=%d", got)
	}
}

func TestBuildBidDoesNotFilterOrClampBelowBidfloor(t *testing.T) {
	impID := "imp-1"
	floor := float32(100)
	imp := &ortb.Imp{Id: &impID, Bidfloor: &floor}
	req := &ortb.BidRequest{Imp: []*ortb.Imp{imp}}
	campaign := &Campaign{ID: "campaign-1", Format: "POP"}
	creative := &Creative{ID: "creative-1", CampaignID: campaign.ID, ADMURL: "https://example.test/click"}

	bid := (&AuctionService{}).buildBid(req, imp, campaign, creative, 1.25)
	if bid == nil {
		t.Fatal("bid below request bidfloor must still be returned by ADV")
	}
	if math.Abs(float64(bid.GetPrice())-1.25) > 1e-6 {
		t.Fatalf("bid price=%v want 1.25; bidfloor must not clamp it", bid.GetPrice())
	}
}

func TestRTBComplexWarningIsAsyncAndDeduplicated(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{
		PercentMapDefaultKey:    0.20,
		PercentMapRTBDefaultKey: 0.30,
		"rtb-smart":             0.25,
	})
	service := NewAuctionService(nil, nil, store, nil, nil)

	started := make(chan string, 3)
	release := make(chan struct{})
	service.SetSnapshotWarningNotifier(func(ctx context.Context, message string) error {
		started <- message
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	warning := snapshotLoadWarning{
		campaignID: "rtb-smart",
		key:        "rtb_complex:rtb-smart:rev-1",
		message:    "[ADV][RTB_SMART_PERCENTER_SKIPPED] campaign_id=rtb-smart fallback=percent_map",
	}

	returned := make(chan struct{})
	go func() {
		service.reportSnapshotWarnings(context.Background(), []snapshotLoadWarning{warning})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(250 * time.Millisecond):
		close(release)
		t.Fatal("RTB+Complex warning blocked snapshot reporting")
	}

	select {
	case message := <-started:
		if !strings.Contains(message, "percent=0.250000") {
			close(release)
			t.Fatalf("warning must include resolved exact map fallback, got %q", message)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("RTB+Complex warning was not delivered asynchronously")
	}

	service.reportSnapshotWarnings(context.Background(), []snapshotLoadWarning{warning})
	select {
	case duplicate := <-started:
		close(release)
		t.Fatalf("same snapshot revision warning was duplicated: %q", duplicate)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	warning.key = "rtb_complex:rtb-smart:rev-2"
	service.reportSnapshotWarnings(context.Background(), []snapshotLoadWarning{warning})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("new snapshot revision must be allowed to emit a new warning")
	}
}

func TestComplexPercenterRoutingExcludesRTBTypeModel2(t *testing.T) {
	store := newPercentStoreForPolicyTest(t, PercentMap{
		PercentMapDefaultKey:    0.20,
		PercentMapRTBDefaultKey: 0.30,
	})
	service := &AuctionService{percents: store}

	ordinary := &Campaign{ID: "ordinary-complex", TypeModel: TypeModelComplex}
	if !shouldUseComplexPercenter(ordinary) {
		t.Fatal("ordinary type_model=2 must use Complex percenter")
	}

	rtb := &Campaign{ID: "rtb-complex-no-explicit-key", RTB: true, TypeModel: TypeModelComplex}
	if shouldUseComplexPercenter(rtb) {
		t.Fatal("RTB + type_model=2 must never create/read Complex state")
	}
	decision, err := service.ResolvePricingDecision(rtb)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Mode != PercentRoutingRTBComplexFallback || math.Abs(decision.Percent-0.30) > 1e-12 || math.Abs(decision.MinMargin-0.30) > 1e-12 {
		t.Fatalf("RTB + type_model=2 must use exact ALL_RTB fallback: %+v", decision)
	}
}

func TestComplexCandidateKeepsOriginalBidSeparateFromAdvertiserPrice(t *testing.T) {
	const originalBid = 1.00
	const sspBid = 0.50
	const margin = 0.30
	advertiserPrice := sspBid / (1 - margin)

	complex := candidate{
		campaign:       &Campaign{ID: "complex", BasePrice: originalBid, TypeModel: TypeModelComplex},
		basePrice:      advertiserPrice,
		originalBid:    originalBid,
		effectivePrice: sspBid,
	}
	other := candidate{
		campaign:       &Campaign{ID: "other", BasePrice: 0.90, TypeModel: TypeModelSimple},
		basePrice:      0.90,
		originalBid:    0.90,
		effectivePrice: 0.80,
	}

	if got := candidateOriginalBid(complex); math.Abs(got-originalBid) > 1e-12 {
		t.Fatalf("candidateOriginalBid=%v want %v", got, originalBid)
	}
	if got := candidateBasePrice(complex); math.Abs(got-advertiserPrice) > 1e-12 {
		t.Fatalf("candidateBasePrice=%v want advertiser price %v", got, advertiserPrice)
	}
	pool := prepareCandidatePool([]candidate{other, complex}, auctionModeMaxBid, nil)
	if len(pool) == 0 || pool[0].campaign.ID != "complex" {
		t.Fatalf("winner selection must use original bid: pool=%+v", pool)
	}
}

func TestComplexResolvedBalanceUsesDynamicAdvertiserPrice(t *testing.T) {
	const originalBid = 1.00
	originalCharge := CalculateChargePrice(originalBid, PricingModelCPM, "POP")

	lowAdvertiserPrice := 0.50 / (1 - 0.30)
	lowCharge := CalculateChargePrice(lowAdvertiserPrice, PricingModelCPM, "POP")
	if !(lowCharge < originalCharge) {
		t.Fatalf("test setup invalid: low charge=%v original charge=%v", lowCharge, originalCharge)
	}
	remainingBetween := (lowCharge + originalCharge) / 2
	if got := resolvedComplexBalanceReason(remainingBetween, remainingBetween, lowCharge, true); got != diagNone {
		t.Fatalf("dynamic advertiser price below original must use lower resolved charge: reason=%s", diagnosticReasonName(got))
	}

	highAdvertiserPrice := 1.40
	highCharge := CalculateChargePrice(highAdvertiserPrice, PricingModelCPM, "POP")
	if !(highCharge > originalCharge) {
		t.Fatalf("test setup invalid: high charge=%v original charge=%v", highCharge, originalCharge)
	}
	remainingBetweenHigh := (originalCharge + highCharge) / 2
	if got := resolvedComplexBalanceReason(remainingBetweenHigh, highCharge*2, highCharge, true); got != diagCampaignBalanceInsufficient {
		t.Fatalf("campaign balance must be checked against resolved higher charge: reason=%s", diagnosticReasonName(got))
	}
	if got := resolvedComplexBalanceReason(highCharge*2, remainingBetweenHigh, highCharge, true); got != diagUserBalanceInsufficient {
		t.Fatalf("user balance must be checked against resolved higher charge: reason=%s", diagnosticReasonName(got))
	}
}
