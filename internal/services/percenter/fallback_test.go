package percenter

import (
	"testing"
	"time"
)

func TestBuildFallbackDecisionsUsesNarrowestLevelWithFiveImpressions(t *testing.T) {
	base := Segment{CampaignID: "c1", SiteID: "s1", Geo: "US", Device: "mobile", OS: "android"}
	a := base
	a.Browser = "chrome"
	a.SSPDomain = "ssp-a"
	b := base
	b.Browser = "firefox"
	b.SSPDomain = "ssp-b"

	decisions := BuildFallbackDecisions([]SegmentTraffic{
		{Segment: a, Impressions: 3},
		{Segment: b, Impressions: 4},
	}, 5)
	if len(decisions) != 2 {
		t.Fatalf("got %d decisions, want 2", len(decisions))
	}
	wantParent := SegmentHierarchy(a)[1]
	for _, d := range decisions {
		if !d.HasSelection {
			t.Fatalf("missing selection: %+v", d)
		}
		if d.SelectedLevel != SegmentLevelCampaignSiteGeoDevOS || d.SelectedHash != wantParent.Hash {
			t.Fatalf("selected wrong fallback: %+v want=%+v", d, wantParent)
		}
		if d.Impressions != 7 {
			t.Fatalf("parent impressions=%d want=7", d.Impressions)
		}
	}
}

func TestBuildFallbackDecisionsStopsAtExactWhenExactHasFive(t *testing.T) {
	segment := Segment{CampaignID: "c1", SiteID: "s1", Geo: "US", Device: "mobile", OS: "android", Browser: "chrome", SSPDomain: "ssp"}
	decision := BuildFallbackDecisions([]SegmentTraffic{{Segment: segment, Impressions: 5}}, 5)[0]
	if !decision.HasSelection || decision.SelectedLevel != SegmentLevelExact || decision.SelectedHash != HashSegment(segment) {
		t.Fatalf("expected exact selection, got %+v", decision)
	}
}

func TestBuildFallbackDecisionsFallsBackToCampaignAndThenHolds(t *testing.T) {
	a := Segment{CampaignID: "c1", SiteID: "s1", Geo: "US", Device: "mobile", OS: "android", Browser: "chrome", SSPDomain: "ssp-a"}
	b := Segment{CampaignID: "c1", SiteID: "s2", Geo: "DE", Device: "desktop", OS: "windows", Browser: "edge", SSPDomain: "ssp-b"}

	decisions := BuildFallbackDecisions([]SegmentTraffic{
		{Segment: a, Impressions: 2},
		{Segment: b, Impressions: 3},
	}, 5)
	for _, d := range decisions {
		if !d.HasSelection || d.SelectedLevel != SegmentLevelCampaign || d.Impressions != 5 {
			t.Fatalf("expected campaign fallback, got %+v", d)
		}
	}

	decisions = BuildFallbackDecisions([]SegmentTraffic{
		{Segment: a, Impressions: 2},
		{Segment: b, Impressions: 2},
	}, 5)
	for _, d := range decisions {
		if d.HasSelection {
			t.Fatalf("expected no selection below campaign threshold, got %+v", d)
		}
	}
}

func TestSetFallbackTargetPreservesPricingAndAdvancesPointVersion(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	state := BaselineStateForCampaign("exact", "c1", 1, 0.2, 1, TypeModelSimple, ProfitModelImpression, now.Add(-time.Hour))
	state.PointVersion = 7
	state.Margin = 0.4
	state.SSPBid = 0.6

	updated, changed := SetFallbackTarget(state, "parent", now)
	if !changed || updated.FallbackSegmentHash != "parent" {
		t.Fatalf("route not updated: %+v", updated)
	}
	if updated.PointVersion != 8 {
		t.Fatalf("point version=%d want=8", updated.PointVersion)
	}
	if updated.Margin != state.Margin || updated.SSPBid != state.SSPBid || updated.AdvertiserPrice != state.AdvertiserPrice {
		t.Fatalf("routing changed optimizer pricing: before=%+v after=%+v", state, updated)
	}
	if EffectiveStateHash(updated) != "parent" {
		t.Fatalf("effective state hash=%q want parent", EffectiveStateHash(updated))
	}

	back, changed := SetFallbackTarget(updated, updated.SegmentHash, now.Add(5*time.Minute))
	if !changed || back.FallbackSegmentHash != "" || EffectiveStateHash(back) != back.SegmentHash {
		t.Fatalf("failed to return to exact state: %+v", back)
	}
}
