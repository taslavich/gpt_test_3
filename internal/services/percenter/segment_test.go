package percenter

import "testing"

func TestHashSegmentIsStableAndKeepsEmptyPositions(t *testing.T) {
	base := Segment{
		SSPDomain:  "Rubicon",
		Geo:        "us",
		Browser:    "",
		Device:     "Mobile",
		OS:         "Android",
		SiteID:     "12345",
		CampaignID: "777",
	}
	got := HashSegment(base)
	if got == "" {
		t.Fatal("empty segment hash")
	}
	if got != HashSegment(base) {
		t.Fatal("same segment must produce the same hash")
	}
	withBrowser := base
	withBrowser.Browser = "chrome"
	if got == HashSegment(withBrowser) {
		t.Fatal("empty browser must remain a distinct segment value")
	}
	shifted := base
	shifted.Browser = base.Device
	shifted.Device = ""
	if got == HashSegment(shifted) {
		t.Fatal("empty field position must not be collapsed")
	}
}

func TestSegmentHierarchyMatchesApprovedOrderAndAvoidsExactCollisions(t *testing.T) {
	segment := Segment{
		SSPDomain:  "ssp.example",
		Geo:        "us",
		Browser:    "Chrome",
		Device:     "Mobile",
		OS:         "Android",
		SiteID:     "site-1",
		CampaignID: "campaign-1",
	}
	h := SegmentHierarchy(segment)
	wantLevels := []SegmentLevel{
		SegmentLevelExact,
		SegmentLevelCampaignSiteGeoDevOS,
		SegmentLevelCampaignSiteGeoDev,
		SegmentLevelCampaignSite,
		SegmentLevelCampaign,
	}
	if len(h) != len(wantLevels) {
		t.Fatalf("unexpected hierarchy size: got=%d want=%d", len(h), len(wantLevels))
	}
	for i, want := range wantLevels {
		if h[i].Level != want {
			t.Fatalf("hierarchy[%d] level=%q want=%q", i, h[i].Level, want)
		}
		if h[i].Hash == "" {
			t.Fatalf("hierarchy[%d] has empty hash", i)
		}
	}
	if h[0].Hash != HashSegment(segment) {
		t.Fatalf("exact hierarchy hash changed: got=%s want=%s", h[0].Hash, HashSegment(segment))
	}

	blankExact := segment
	blankExact.Browser = ""
	blankExact.SSPDomain = ""
	if HashSegment(blankExact) == h[1].Hash {
		t.Fatal("parent hierarchy hash collided with an exact segment containing blank browser/ssp")
	}
}
