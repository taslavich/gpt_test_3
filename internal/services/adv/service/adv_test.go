package auction

import (
	"math"
	"net/url"
	"testing"
	"time"

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
