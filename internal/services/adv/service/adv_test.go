package auction

import (
	"html"
	"math"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestNextAuctionModeUsesExactNinetyFiveFourOneCycle(t *testing.T) {
	service := &AuctionService{}
	wantModeByPosition := func(position int) auctionMode {
		switch {
		case position <= 95:
			return auctionModeMaxBid
		case position <= 99:
			return auctionModeWeightedTop
		default:
			return auctionModeWeightedAll
		}
	}

	for cycle := 0; cycle < 2; cycle++ {
		for position := 1; position <= 100; position++ {
			mode, gotPosition := service.nextAuctionMode()
			if gotPosition != uint64(position) {
				t.Fatalf("cycle %d: position=%d want %d", cycle+1, gotPosition, position)
			}
			if want := wantModeByPosition(position); mode != want {
				t.Fatalf("cycle %d position %d: mode=%s want %s", cycle+1, position, mode, want)
			}
		}
	}
}

func TestNextAuctionModeIsAtomic(t *testing.T) {
	service := &AuctionService{}
	const requests = 100

	positions := make(chan uint64, requests)
	var wg sync.WaitGroup
	wg.Add(requests)
	for range requests {
		go func() {
			defer wg.Done()
			_, position := service.nextAuctionMode()
			positions <- position
		}()
	}
	wg.Wait()
	close(positions)

	seen := make(map[uint64]int, requests)
	for position := range positions {
		seen[position]++
	}
	for position := uint64(1); position <= requests; position++ {
		if seen[position] != 1 {
			t.Fatalf("position %d occurred %d times, want exactly once", position, seen[position])
		}
	}
}

func TestPrepareCandidatePoolMaxBidShufflesOnlyEqualTopPrices(t *testing.T) {
	candidates := []candidate{
		{campaign: &Campaign{ID: "low"}, effectivePrice: 5},
		{campaign: &Campaign{ID: "top-a"}, effectivePrice: 10},
		{campaign: &Campaign{ID: "middle"}, effectivePrice: 8},
		{campaign: &Campaign{ID: "top-b"}, effectivePrice: 10},
		{campaign: &Campaign{ID: "top-c"}, effectivePrice: 10},
	}

	reverseShuffle := func(n int, swap func(i, j int)) {
		for left, right := 0, n-1; left < right; left, right = left+1, right-1 {
			swap(left, right)
		}
	}
	pool := prepareCandidatePool(candidates, auctionModeMaxBid, reverseShuffle)

	gotIDs := make([]string, 0, len(pool))
	for _, cand := range pool {
		gotIDs = append(gotIDs, cand.campaign.ID)
	}
	wantIDs := []string{"top-c", "top-b", "top-a", "middle", "low"}
	for index := range wantIDs {
		if gotIDs[index] != wantIDs[index] {
			t.Fatalf("pool order=%v want %v", gotIDs, wantIDs)
		}
	}

	if candidates[0].campaign.ID != "low" {
		t.Fatal("prepareCandidatePool mutated the original candidate slice")
	}
}

func TestPrepareCandidatePoolWeightedTopDropsPricesBelowEightyPercent(t *testing.T) {
	candidates := []candidate{
		{campaign: &Campaign{ID: "100"}, effectivePrice: 100},
		{campaign: &Campaign{ID: "95"}, effectivePrice: 95},
		{campaign: &Campaign{ID: "80"}, effectivePrice: 80},
		{campaign: &Campaign{ID: "79"}, effectivePrice: 79},
	}

	pool := prepareCandidatePool(candidates, auctionModeWeightedTop, nil)
	if len(pool) != 3 {
		t.Fatalf("weighted top pool has %d candidates, want 3", len(pool))
	}
	for _, cand := range pool {
		if cand.campaign.ID == "79" {
			t.Fatal("candidate below 80% of the maximum remained in the pool")
		}
	}
}

func TestPrepareCandidatePoolWeightedAllKeepsEveryCandidate(t *testing.T) {
	candidates := []candidate{
		{campaign: &Campaign{ID: "a"}, effectivePrice: 100},
		{campaign: &Campaign{ID: "b"}, effectivePrice: 1},
	}

	pool := prepareCandidatePool(candidates, auctionModeWeightedAll, nil)
	if len(pool) != len(candidates) {
		t.Fatalf("weighted all pool has %d candidates, want %d", len(pool), len(candidates))
	}
}

func TestWeightedCandidateIndexUsesEffectivePriceAsWeight(t *testing.T) {
	candidates := []candidate{
		{campaign: &Campaign{ID: "a"}, effectivePrice: 50},
		{campaign: &Campaign{ID: "b"}, effectivePrice: 40},
		{campaign: &Campaign{ID: "c"}, effectivePrice: 10},
	}

	tests := []struct {
		randomUnit float64
		wantIndex  int
	}{
		{randomUnit: 0, wantIndex: 0},
		{randomUnit: 0.499999, wantIndex: 0},
		{randomUnit: 0.50, wantIndex: 1},
		{randomUnit: 0.899999, wantIndex: 1},
		{randomUnit: 0.90, wantIndex: 2},
		{randomUnit: math.Nextafter(1, 0), wantIndex: 2},
	}

	for _, test := range tests {
		if got := weightedCandidateIndex(candidates, test.randomUnit); got != test.wantIndex {
			t.Fatalf("randomUnit=%v: index=%d want %d", test.randomUnit, got, test.wantIndex)
		}
	}
}

func TestTrafficMatches(t *testing.T) {
	tests := []struct {
		campaign string
		request  string
		want     bool
	}{
		{TrafficAdult, TrafficAdult, true},
		{TrafficAdult, TrafficMainstream, false},
		{TrafficMainstream, TrafficMainstream, true},
		{TrafficMainstream, TrafficAdult, false},
		{TrafficMixed, TrafficAdult, true},
		{TrafficMixed, TrafficMainstream, true},
		{TrafficMixed, TrafficMixed, false},
	}

	for _, test := range tests {
		if got := trafficMatches(test.campaign, test.request); got != test.want {
			t.Fatalf("trafficMatches(%q, %q)=%t want %t", test.campaign, test.request, got, test.want)
		}
	}
}

func TestAuctionPriceUsesBasePriceWhileChargePriceUsesBillingRules(t *testing.T) {
	basePrice := 300.0
	deduction := 0.5

	chargePrice := CalculateChargePrice(basePrice, PricingModelCPM, "POP")
	if math.Abs(chargePrice-0.3) > 1e-12 {
		t.Fatalf("charge price got %v want 0.3", chargePrice)
	}

	auctionPrice := CalculateEffectiveAuctionPrice(basePrice, deduction)
	if math.Abs(auctionPrice-150) > 1e-12 {
		t.Fatalf("auction price got %v want 150", auctionPrice)
	}
}

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

func TestMatchingBannerCreativesFiltersImageMIMEAndAllowsIframe(t *testing.T) {
	w, h := int32(300), int32(250)
	imp := &ortb.Imp{Banner: &ortb.Banner{
		W:     &w,
		H:     &h,
		Mimes: []string{"image/png"},
	}}

	png := &Creative{ID: "png", ADMURL: "png", W: 300, H: 250, BannerType: "img", FileFormat: "image/png"}
	gif := &Creative{ID: "gif", ADMURL: "gif", W: 300, H: 250, BannerType: "img", FileFormat: "image/gif"}
	iframe := &Creative{ID: "iframe", ADMURL: "iframe", W: 300, H: 250, BannerType: "iframe", FileFormat: "text/html"}

	matched := matchingCreatives([]*Creative{png, gif, iframe}, imp, "BAN")
	if len(matched) != 2 || matched[0] != png || matched[1] != iframe {
		t.Fatalf("unexpected MIME-filtered creatives: %#v", matched)
	}

	imp.Banner.Mimes = nil
	matched = matchingCreatives([]*Creative{png, gif}, imp, "BAN")
	if len(matched) != 2 {
		t.Fatalf("empty request mimes must not filter image creatives: %#v", matched)
	}
}

func TestEffectivePriceMeetsBidFloor(t *testing.T) {
	floor := float32(0.75)
	imp := &ortb.Imp{Bidfloor: &floor}

	if effectivePriceMeetsBidFloor(0.74, imp) {
		t.Fatal("effective price below bidfloor must be rejected")
	}
	if !effectivePriceMeetsBidFloor(0.75, imp) {
		t.Fatal("effective price equal to bidfloor must be accepted")
	}
	if !effectivePriceMeetsBidFloor(0.76, imp) {
		t.Fatal("effective price above bidfloor must be accepted")
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

func TestBuildBidBannerImageAppendsTrackerMacrosOnlyToAnchorHref(t *testing.T) {
	service := &AuctionService{}
	impID := "imp-1"
	reqID := "request-1"
	siteID := "site id&1"
	req := &ortb.BidRequest{Id: &reqID, Site: &ortb.Site{Id: &siteID}}
	imp := &ortb.Imp{Id: &impID}
	campaign := &Campaign{ID: "campaign-1", Format: "BAN", BasePrice: 2.5}
	adm := `<a href="https://click.example/path?existing=1&amp;source=banner" target="_blank"><img src="https://media.example/image.png"></a>`
	creative := &Creative{
		ID:             "creative-1",
		ADMURL:         adm,
		BannerType:     "img",
		TrackersMacros: map[string]bool{"site_id": true},
		W:              300,
		H:              250,
	}

	bid := service.buildBid(req, imp, campaign, creative, 1)
	if bid == nil {
		t.Fatal("buildBid returned nil")
	}
	result := bid.GetAdm()
	match := bannerAnchorHrefPattern.FindStringSubmatch(result)
	if len(match) != 4 {
		t.Fatalf("banner anchor href not found in ADM: %q", result)
	}
	href := match[2]
	if href == "" {
		href = match[3]
	}
	parsed, err := url.Parse(html.UnescapeString(href))
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query().Get("site_id"); got != siteID {
		t.Fatalf("site_id macro not added to href: got %q in %q", got, result)
	}
	if got := parsed.Query().Get("existing"); got != "1" {
		t.Fatalf("existing href query lost: got %q in %q", got, result)
	}
	if got := parsed.Query().Get("source"); got != "banner" {
		t.Fatalf("HTML-escaped href query lost: got %q in %q", got, result)
	}
	if !strings.Contains(result, `src="https://media.example/image.png"`) {
		t.Fatalf("image src or ADM markup was modified: %q", result)
	}
}

func TestBuildBidBannerIframeDoesNotAppendTrackerMacros(t *testing.T) {
	service := &AuctionService{}
	impID := "imp-1"
	reqID := "request-1"
	siteID := "site-1"
	req := &ortb.BidRequest{Id: &reqID, Site: &ortb.Site{Id: &siteID}}
	imp := &ortb.Imp{Id: &impID}
	campaign := &Campaign{ID: "campaign-1", Format: "BAN", BasePrice: 2.5}
	adm := `<iframe src="https://iframe.example/render?existing=1" width="300" height="250"></iframe>`
	creative := &Creative{
		ID:             "creative-1",
		ADMURL:         adm,
		BannerType:     "iframe",
		TrackersMacros: map[string]bool{"site_id": true},
		W:              300,
		H:              250,
	}

	bid := service.buildBid(req, imp, campaign, creative, 1)
	if bid == nil {
		t.Fatal("buildBid returned nil")
	}
	if got := bid.GetAdm(); got != adm {
		t.Fatalf("iframe ADM must remain unchanged: got %q want %q", got, adm)
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
	_, err := validateAndNormalizePercentMap(PercentMap{"user-id": math.NaN()})
	if err == nil {
		t.Fatal("NaN percent must be rejected")
	}
}

func TestPercentMapUsesCampaignUserIDOnly(t *testing.T) {
	store := &PercentStore{}
	store.value.Store(&percentSnapshot{Values: PercentMap{
		"user-id": 0.25,
	}})

	if got := store.Lookup(" USER-ID "); math.Abs(got-0.25) > 1e-12 {
		t.Fatalf("got %v, want 0.25", got)
	}
	if got := store.Lookup("other-user"); math.Abs(got-DefaultADVPercent) > 1e-12 {
		t.Fatalf("missing user must use default deduction %v, got %v", DefaultADVPercent, got)
	}
}

func TestPercentMapEmptySnapshotUsesThirtyPercentDefault(t *testing.T) {
	store := &PercentStore{}
	store.value.Store(&percentSnapshot{Values: PercentMap{}})

	if got := store.Lookup("any-user"); math.Abs(got-0.30) > 1e-12 {
		t.Fatalf("empty map default=%v, want 0.30", got)
	}
	if got := store.Lookup(""); math.Abs(got-0.30) > 1e-12 {
		t.Fatalf("empty user ID default=%v, want 0.30", got)
	}
}

func TestPercentMapExplicitZeroOverridesDefault(t *testing.T) {
	store := &PercentStore{}
	store.value.Store(&percentSnapshot{Values: PercentMap{"user-id": 0}})

	if got := store.Lookup("user-id"); got != 0 {
		t.Fatalf("explicit zero must override the default, got %v", got)
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

func TestQualityMapsAllowEmptySnapshot(t *testing.T) {
	snapshot, err := buildQualitySnapshot(emptyQualityMaps())
	if err != nil {
		t.Fatalf("empty quality maps must be valid: %v", err)
	}
	store := &QualityStore{}
	store.value.Store(snapshot)
	if store.ContainsAny("mc_example.com") {
		t.Fatal("an empty quality snapshot must not match any SSP domain")
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

	bidPrice := 0.001875
	bid := service.buildBid(req, imp, campaign, creative, bidPrice)
	if bid == nil {
		t.Fatal("buildBid returned nil")
	}
	if got := bid.GetAdm(); got != creative.ADMURL {
		t.Fatalf("ADV must return raw creative ADM for BidEngine finalization: got %q want %q", got, creative.ADMURL)
	}
	if got := float64(bid.GetPrice()); math.Abs(got-bidPrice) > 1e-9 {
		t.Fatalf("ADV bid price must use effective auction price: got %.12f want %.12f", got, bidPrice)
	}
	if bid.GetNurl() != "" || bid.GetBurl() != "" {
		t.Fatalf("ADV must not form callbacks: nurl=%q burl=%q", bid.GetNurl(), bid.GetBurl())
	}
}

func TestExtractRequestFilterValuesUsesLanguageAndUADevice(t *testing.T) {
	language := " RU "
	userAgent := "Mozilla/5.0 (Linux; Android 13; SM-S918B) AppleWebKit/537.36 Chrome/120.0 Mobile Safari/537.36"
	req := &ortb.BidRequest{Device: &ortb.Device{Language: &language, Ua: &userAgent}}

	values := extractRequestFilterValues(req)
	if values.language == nil || *values.language != "ru" {
		t.Fatalf("language must be normalized and preserved: got %v", values.language)
	}
	if values.deviceType == nil || *values.deviceType != "mobile" {
		t.Fatalf("device type must be derived from UA: got %v", values.deviceType)
	}
}
