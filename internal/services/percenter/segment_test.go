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
