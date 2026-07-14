package auction

import (
	"math"
	"net/url"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func TestChargePriceRules(t *testing.T) {
	tests := []struct {
		model, format string
		base, want    float64
	}{
		{PricingModelCPM, "BAN", 10, 0.01},
		{PricingModelCPM, "IPP", 10, 0.01},
		{PricingModelCPC, "POP", 10, 0.01},
		{PricingModelCPC, "NAT", 10, 10},
	}
	for _, test := range tests {
		if got := CalculateChargePrice(test.base, test.model, test.format); math.Abs(got-test.want) > 1e-12 {
			t.Fatalf("%s/%s: got %v want %v", test.model, test.format, got, test.want)
		}
	}
	if got := CalculateEffectiveAuctionPrice(1, 0.15); math.Abs(got-0.85) > 1e-12 {
		t.Fatalf("effective price got %v", got)
	}
}

func TestBannerFormatPreferredAndFallback(t *testing.T) {
	w300, h250, w728, h90 := int32(300), int32(250), int32(728), int32(90)
	imp := &ortb.Imp{Banner: &ortb.Banner{W: &w300, H: &h250, Format: []*ortb.Format{{W: &w728, H: &h90}}}}
	sizes := bannerSizes(imp)
	if !sizes[[2]int{728, 90}] || sizes[[2]int{300, 250}] {
		t.Fatalf("banner.format must take precedence: %#v", sizes)
	}
	imp.Banner.Format = nil
	sizes = bannerSizes(imp)
	if !sizes[[2]int{300, 250}] {
		t.Fatalf("w/h fallback missing")
	}
}

func TestMatchingCreativesDoesNotMutateCampaign(t *testing.T) {
	w, h := int32(300), int32(250)
	first := &Creative{ID: "first", W: 300, H: 250}
	second := &Creative{ID: "second", W: 728, H: 90}
	original := []*Creative{first, second}
	matched := matchingCreatives(original, &ortb.Imp{Banner: &ortb.Banner{W: &w, H: &h}}, "BAN")
	if len(matched) != 1 || matched[0] != first {
		t.Fatalf("unexpected match: %#v", matched)
	}
	if len(original) != 2 || original[1] != second {
		t.Fatal("original creatives mutated")
	}
}

func TestWhitelistBlacklistMissingValue(t *testing.T) {
	white := filterV2.NewFilters(true, true, []string{"SE"})
	black := filterV2.NewFilters(true, false, []string{"SE"})
	if white.Allowed(nil) {
		t.Fatal("missing value must fail whitelist")
	}
	if !black.Allowed(nil) {
		t.Fatal("missing value must pass blacklist")
	}
}

func TestActiveSlotsLeftUsesFractionalEdges(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	campaign := &Campaign{StartTS: now, EndTS: now.Add(150 * time.Second)}
	if got := ActiveSlotsLeft(campaign, now); math.Abs(got-0.5) > 1e-12 {
		t.Fatalf("got %v active slots, want 0.5", got)
	}
}

func TestCurrentSlotActiveFractionUsesWholeSlotEdge(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 4, 30, 0, time.UTC)
	campaign := &Campaign{
		StartTS: time.Date(2026, 7, 13, 12, 4, 0, 0, time.UTC),
		EndTS:   time.Date(2026, 7, 13, 12, 15, 0, 0, time.UTC),
	}
	if got := CurrentSlotActiveFraction(campaign, now); math.Abs(got-0.2) > 1e-12 {
		t.Fatalf("got current slot fraction %v, want 0.2", got)
	}
}

func TestPacingSlotTargetScalesPartialCurrentSlot(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 4, 30, 0, time.UTC)
	campaign := &Campaign{
		GoalTotalDollars: 100,
		StartTS:          time.Date(2026, 7, 13, 12, 4, 0, 0, time.UTC),
		EndTS:            time.Date(2026, 7, 13, 12, 15, 0, 0, time.UTC),
	}
	got, err := pacingSlotTarget(campaign, now, 45, 5)
	if err != nil {
		t.Fatal(err)
	}
	// Spend before the current slot is 40, so 60 remains at slot start.
	// Active time from slot start is 2.2 slot-equivalents and the current
	// edge slot contributes 0.2, therefore its total target is 60/2.2*0.2.
	want := 60.0 / 2.2 * 0.2
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got slot target %v, want %v", got, want)
	}
}

func TestTrackerMacrosUnknownAndEncoded(t *testing.T) {
	siteID := "site id&1"
	req := &ortb.BidRequest{Site: &ortb.Site{Id: &siteID}}
	result := appendTrackerMacros("https://example.test/a?existing=1", map[string]bool{"site_id": true, "browser": true}, "c", "cr", req)
	parsed, err := url.Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("site_id") != siteID {
		t.Fatalf("site_id not encoded/decoded correctly: %s", result)
	}
	if parsed.Query().Get("browser") != unknownTrackerValue {
		t.Fatalf("missing browser should be unknown")
	}
	if parsed.Query().Get("existing") != "1" {
		t.Fatal("existing query lost")
	}
}

func TestPercentMapRejectsNaN(t *testing.T) {
	_, err := validateAndNormalizePercentMap(PercentMap{"ssp.test": {"SE": {"user": &types.PercentAndBidfloor{Percent: float32(math.NaN())}}}})
	if err == nil {
		t.Fatal("NaN percent must be rejected")
	}
}

func TestSnapshotDeepClone(t *testing.T) {
	filter := filterV2.NewFilters(true, true, []string{"SE"})
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	creative := &Creative{ID: "cr", CampaignID: "c", ADMURL: "https://example.test/creative", W: 300, H: 250, TrackersMacros: map[string]bool{"device": true}}
	campaign := &Campaign{
		ID: "c", UserID: "u", Status: CampaignStatusActive, PricingModel: PricingModelCPM,
		Format: "BAN", TrafficType: TrafficMainstream, QualitySegment: "usual",
		BasePrice: 1, GoalTotalDollars: 100, StartTS: now, EndTS: now.Add(time.Hour),
		CountryFilter: filter, Creatives: []*Creative{creative},
	}
	source := &Snapshot{UserGoals: map[string]float64{"u": 10}, Campaigns: []*Campaign{campaign}}
	clone, err := cloneAndValidateSnapshot(source)
	if err != nil {
		t.Fatal(err)
	}
	source.UserGoals["u"] = 0
	filter.Objects["SE"] = false
	creative.TrackersMacros["device"] = false
	if clone.UserGoals["u"] != 10 || !clone.Campaigns[0].CountryFilter.Objects["SE"] || !clone.Campaigns[0].Creatives[0].TrackersMacros["device"] {
		t.Fatal("published snapshot shares mutable state")
	}
}

func TestBannerAndIPPFormatsRequireDistinctRouterMarker(t *testing.T) {
	banner := &ortb.Banner{Ext: []string{constants.ADVImpressionFormatMarkerPrefix + constants.BAN}}
	imp := &ortb.Imp{Banner: banner}
	if !impressionMatchesFormat(imp, constants.BAN) {
		t.Fatal("BAN marker must match BAN")
	}
	if impressionMatchesFormat(imp, constants.IPP) {
		t.Fatal("BAN marker must not match IPP")
	}
	banner.Ext = []string{constants.ADVImpressionFormatMarkerPrefix + constants.IPP}
	if !impressionMatchesFormat(imp, constants.IPP) {
		t.Fatal("IPP marker must match IPP")
	}
	if impressionMatchesFormat(imp, constants.BAN) {
		t.Fatal("IPP marker must not match BAN")
	}
}

func TestQualityMapsRejectEmptySnapshot(t *testing.T) {
	if _, err := buildQualitySnapshot(emptyQualityMaps()); err == nil {
		t.Fatal("empty quality maps must be rejected")
	}
}

func TestQualityMapsAllowDomainInMultipleSegments(t *testing.T) {
	sharedDomain := "MC_Moblivion.COM."
	highOnlyDomain := "high-only.example"
	snapshot, err := buildQualitySnapshot(QualityMaps{
		"usual": {sharedDomain: true},
		"high":  {sharedDomain: true, highOnlyDomain: true},
		"ultra": {},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &QualityStore{}
	store.value.Store(snapshot)
	if !store.Contains("usual", sharedDomain) {
		t.Fatal("shared domain must be allowed for usual campaigns")
	}
	if !store.Contains("high", sharedDomain) {
		t.Fatal("shared domain must also be allowed for high campaigns")
	}
	if store.Contains("ultra", sharedDomain) {
		t.Fatal("shared domain must not be allowed for ultra until it is added there")
	}
	if !store.Contains("high", highOnlyDomain) || store.Contains("usual", highOnlyDomain) {
		t.Fatal("high-only domain membership is incorrect")
	}
	if !store.ContainsAny(sharedDomain) || store.ContainsAny("missing.example") {
		t.Fatal("ContainsAny returned an incorrect result")
	}
}

func TestQualityStoreReplacesOneMapAndPreservesOverlappingDomains(t *testing.T) {
	sharedDomain := "MC_Moblivion.COM."
	highOnlyDomain := "high-only.example"
	path := t.TempDir() + "/quality.json"
	if err := writeJSONAtomic(path, QualityMaps{
		"usual": {sharedDomain: true},
		"high":  {},
		"ultra": {},
	}); err != nil {
		t.Fatal(err)
	}
	store, err := NewQualityStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("high", QualityDomainMap{sharedDomain: true, highOnlyDomain: true}); err != nil {
		t.Fatal(err)
	}
	if !store.Contains("usual", sharedDomain) || !store.Contains("high", sharedDomain) {
		t.Fatal("replacing high must preserve the same domain in usual")
	}
	if err := store.Update("ultra", QualityDomainMap{sharedDomain: true}); err != nil {
		t.Fatal(err)
	}
	if !store.Contains("usual", sharedDomain) || !store.Contains("high", sharedDomain) || !store.Contains("ultra", sharedDomain) {
		t.Fatal("the same domain must be allowed in all three quality maps")
	}
	if err := store.UpdateAll(QualityMaps{
		"usual": {sharedDomain: true},
		"high":  {sharedDomain: true, highOnlyDomain: true},
		"ultra": {sharedDomain: true},
	}); err != nil {
		t.Fatal(err)
	}
	if !store.Contains("usual", sharedDomain) || !store.Contains("high", sharedDomain) || !store.Contains("ultra", sharedDomain) {
		t.Fatal("atomic all-map update lost overlapping memberships")
	}
}

func TestCreativeDimensionsRequiredOnlyForBanner(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	newSnapshot := func(format string) *Snapshot {
		return &Snapshot{
			UserGoals: map[string]float64{"u": 100},
			Campaigns: []*Campaign{{
				ID: "c", UserID: "u", Status: CampaignStatusActive,
				PricingModel: PricingModelCPM, Format: format,
				TrafficType: TrafficMainstream, QualitySegment: "usual",
				BasePrice: 1, GoalTotalDollars: 100,
				StartTS: now, EndTS: now.Add(time.Hour),
				Creatives: []*Creative{{
					ID: "cr", CampaignID: "c", ADMURL: "https://example.test/creative",
				}},
			}},
		}
	}

	for _, format := range []string{"NAT", "IPP", "POP"} {
		if _, err := cloneAndValidateSnapshot(newSnapshot(format)); err != nil {
			t.Fatalf("%s creative without dimensions must be accepted: %v", format, err)
		}
	}
	if _, err := cloneAndValidateSnapshot(newSnapshot("BAN")); err == nil {
		t.Fatal("banner creative without dimensions must be rejected")
	}
}

func TestMatchingCreativesIgnoresDimensionsOutsideBanner(t *testing.T) {
	creative := &Creative{ID: "cr", ADMURL: "https://example.test/creative"}
	for _, format := range []string{"NAT", "IPP", "POP"} {
		matched := matchingCreatives([]*Creative{creative}, &ortb.Imp{}, format)
		if len(matched) != 1 || matched[0] != creative {
			t.Fatalf("%s creative without dimensions must remain eligible", format)
		}
	}
}

func TestBuildBidLeavesCallbackFinalizationToBidEngine(t *testing.T) {
	service := &AuctionService{}
	impID := "imp-1"
	reqID := "request-1"
	req := &ortb.BidRequest{Id: &reqID}
	imp := &ortb.Imp{Id: &impID}
	campaign := &Campaign{ID: "campaign-1", BasePrice: 2.5}
	creative := &Creative{ID: "creative-1", ADMURL: "https://creative.example/render", W: 300, H: 250}

	bid := service.buildBid(req, imp, campaign, creative)
	if bid == nil {
		t.Fatal("buildBid returned nil")
	}
	if got := bid.GetAdm(); got != creative.ADMURL {
		t.Fatalf("ADV must return raw creative ADM for BidEngine finalization: got %q want %q", got, creative.ADMURL)
	}
	if bid.GetNurl() != "" || bid.GetBurl() != "" {
		t.Fatalf("ADV must not form callbacks: nurl=%q burl=%q", bid.GetNurl(), bid.GetBurl())
	}
}
