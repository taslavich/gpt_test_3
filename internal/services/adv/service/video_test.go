package auction

import (
	"strings"
	"testing"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func i32(v int32) *int32 { return &v }

func testVideoCreative() *Creative {
	skippable := true
	return &Creative{
		ID:           "v1",
		CreativeName: "video-one",
		ADMURL:       "https://advertiser.example/landing?src=twinbid",
		ImageURL:     "https://cdn.example/video.mp4",
		FileFormat:   "video/mp4",
		W:            1920,
		H:            1080,
		VideoFormat:  VideoFormatInstream,
		VideoMetadata: &VideoCreativeMetadata{
			Mimes: []string{"video/mp4"}, Duration: 20, Protocols: []int32{2, 3, 7},
			API: []int32{2}, Bitrate: 1200, Linearity: 1, Skippable: &skippable,
			Width: 1920, Height: 1080, Codec: "h264", FileSize: 5 << 20,
		},
	}
}

func TestVideoFormatMatchesImpression(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		placement *int32
		plcmt     *int32
		want      bool
	}{
		{name: "instream placement", format: VideoFormatInstream, placement: i32(1), want: true},
		{name: "outstream placement", format: VideoFormatOutstream, placement: i32(3), want: true},
		{name: "popup placement", format: VideoFormatPopup, placement: i32(5), want: true},
		{name: "wrong placement", format: VideoFormatInstream, placement: i32(3), want: false},
		{name: "placement has priority over plcmt", format: VideoFormatPopup, placement: i32(5), plcmt: i32(1), want: true},
		{name: "generic plcmt is not a fallback", format: VideoFormatInstream, plcmt: i32(1), want: false},
		{name: "missing classification", format: VideoFormatOutstream, want: false},
		{name: "legacy frontend alias", format: "outstream_slider", placement: i32(4), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			imp := &ortb.Imp{Video: &ortb.Video{Placement: test.placement, Plcmt: test.plcmt}}
			if got := videoFormatMatchesImpression(test.format, imp); got != test.want {
				t.Fatalf("videoFormatMatchesImpression(%q)=%v want=%v", test.format, got, test.want)
			}
		})
	}
}

func TestVideoCreativeCompatibility(t *testing.T) {
	creative := testVideoCreative()
	imp := &ortb.Imp{Video: &ortb.Video{
		Mimes: []string{"video/mp4", "video/webm"}, Minduration: i32(5), Maxduration: i32(30),
		Protocols: []int32{2}, W: i32(400), H: i32(225), Linearity: i32(1),
		Minbitrate: i32(300), Maxbitrate: i32(9600), Api: []int32{2}, Skip: i32(1), Placement: i32(1),
		// These low-level request attributes are deliberately not advertiser filters.
		Startdelay: i32(-1), Playbackmethod: []int32{4}, Pos: i32(7),
	}}
	if ok, reason := videoCreativeMatchesImpression(creative, imp); !ok || reason != diagNone {
		t.Fatalf("compatible VIDEO creative rejected: reason=%v", reason)
	}
	imp.Video.Maxduration = i32(10)
	if ok, reason := videoCreativeMatchesImpression(creative, imp); ok || reason != diagVideoDurationMismatch {
		t.Fatalf("duration mismatch not detected: ok=%v reason=%v", ok, reason)
	}
}

func TestVideoFilterTraceReportsExactDecision(t *testing.T) {
	creative := testVideoCreative()
	imp := &ortb.Imp{Video: &ortb.Video{
		Mimes: []string{"video/mp4"}, Minduration: i32(5), Maxduration: i32(30),
		Protocols: []int32{2, 3, 7}, W: i32(1920), H: i32(1080), Placement: i32(1),
	}}
	trace := evaluateVideoCreativeCompatibility(creative, imp)
	if !trace.Matched || trace.Reason != diagNone {
		t.Fatalf("expected matching trace, got matched=%v reason=%v", trace.Matched, trace.Reason)
	}
	if trace.RequestFormat != VideoFormatInstream || trace.CreativeFormat != VideoFormatInstream {
		t.Fatalf("unexpected format trace: request=%q creative=%q", trace.RequestFormat, trace.CreativeFormat)
	}
	for name, got := range map[string]string{
		"format": trace.FormatCheck, "source": trace.SourceCheck, "protocol": trace.ProtocolCheck,
		"mime": trace.MimeCheck, "duration": trace.DurationCheck, "api": trace.APICheck,
		"battr": trace.BattrCheck, "bitrate": trace.BitrateCheck, "linearity": trace.LinearityCheck,
		"skip": trace.SkipCheck, "size": trace.SizeCheck,
	} {
		if got != "pass" {
			t.Fatalf("%s check=%q want pass", name, got)
		}
	}

	imp.Video.Placement = i32(3)
	trace = evaluateVideoCreativeCompatibility(creative, imp)
	if trace.Matched || trace.Reason != diagVideoFormatMismatch || trace.FormatCheck != "fail" {
		t.Fatalf("format mismatch trace is wrong: %+v", trace)
	}
	if trace.MimeCheck != "not_checked" {
		t.Fatalf("later checks must remain not_checked after format reject: %+v", trace)
	}
}

func TestVideoCreativeRejectsMissingMIMEAllowList(t *testing.T) {
	creative := testVideoCreative()
	imp := &ortb.Imp{Video: &ortb.Video{Protocols: []int32{3}, Placement: i32(1)}}
	if ok, reason := videoCreativeMatchesImpression(creative, imp); ok || reason != diagVideoMimeMismatch {
		t.Fatalf("request without VIDEO mimes accepted: ok=%v reason=%v", ok, reason)
	}
}

func TestVideoCreativeRejectsWrapperOnlyProtocol(t *testing.T) {
	creative := testVideoCreative()
	imp := &ortb.Imp{Video: &ortb.Video{Mimes: []string{"video/mp4"}, Protocols: []int32{5, 6, 8}, Placement: i32(1)}}
	if ok, reason := videoCreativeMatchesImpression(creative, imp); ok || reason != diagVideoProtocolMismatch {
		t.Fatalf("wrapper-only request accepted: ok=%v reason=%v", ok, reason)
	}
}

func TestVideoCreativeFormatMismatch(t *testing.T) {
	creative := testVideoCreative()
	creative.VideoFormat = VideoFormatInstream
	imp := &ortb.Imp{Video: &ortb.Video{Mimes: []string{"video/mp4"}, Protocols: []int32{3}, Placement: i32(3)}}
	if ok, reason := videoCreativeMatchesImpression(creative, imp); ok || reason != diagVideoFormatMismatch {
		t.Fatalf("VIDEO format mismatch not detected: ok=%v reason=%v", ok, reason)
	}
}

func TestVideoCreativeRejectsInvalidSourceURL(t *testing.T) {
	creative := testVideoCreative()
	creative.ADMURL = "javascript:alert(1)"
	imp := &ortb.Imp{Video: &ortb.Video{Mimes: []string{"video/mp4"}, Protocols: []int32{3}, Placement: i32(1)}}
	if ok, reason := videoCreativeMatchesImpression(creative, imp); ok || reason != diagVideoSourceInvalid {
		t.Fatalf("invalid advertiser URL accepted: ok=%v reason=%v", ok, reason)
	}
}

func TestBuildOwnVideoVASTSelectsHighestSupportedInlineVersion(t *testing.T) {
	creative := testVideoCreative()
	imp := &ortb.Imp{Video: &ortb.Video{Protocols: []int32{2, 3, 7}, Placement: i32(1)}}
	vast, ok := buildOwnVideoVAST(imp, creative, creative.ADMURL)
	if !ok {
		t.Fatal("failed to build own VIDEO VAST")
	}
	for _, want := range []string{`<VAST xmlns="http://www.iab.com/VAST" version="4.0">`, `<UniversalAdId idRegistry="twinbidexchange.com" idValue="v1">v1</UniversalAdId>`, videoImpressionPlaceholder, creative.ADMURL, `<![CDATA[` + creative.ImageURL + `]]>`, `<Duration>00:00:20</Duration>`} {
		if !strings.Contains(vast, want) {
			t.Fatalf("generated VAST missing %q: %s", want, vast)
		}
	}
	if strings.Index(vast, "<MediaFiles>") > strings.Index(vast, "<VideoClicks>") {
		t.Fatalf("VAST 4 Linear order must be Duration -> MediaFiles -> VideoClicks: %s", vast)
	}
	if strings.Index(vast, "<Impression>") > strings.Index(vast, "<AdTitle>") {
		t.Fatalf("VAST 4 InLine order must place Impression before AdTitle: %s", vast)
	}
	if strings.Contains(vast, "ClickTracking") || strings.Contains(vast, "TrackingEvents") || strings.Contains(vast, "Wrapper") {
		t.Fatalf("generated VAST contains forbidden tracking/wrapper markup: %s", vast)
	}
}

func TestSelectVASTVersionPreference(t *testing.T) {
	tests := []struct {
		protocols []int32
		version   string
		ok        bool
	}{
		{[]int32{2}, "2.0", true},
		{[]int32{2, 3}, "3.0", true},
		{[]int32{2, 3, 7}, "4.0", true},
		{[]int32{5, 6, 8}, "", false},
	}
	for _, tc := range tests {
		got, _, ok := selectVASTVersion(tc.protocols)
		if ok != tc.ok || got != tc.version {
			t.Fatalf("protocols=%v got version=%q ok=%v; want %q/%v", tc.protocols, got, ok, tc.version, tc.ok)
		}
	}
}

func TestParseVideoMetadataCabinetContract(t *testing.T) {
	metadata, err := parseVideoCreativeMetadataJSONB([]byte(`{
		"mimes":["video/mp4"],
		"duration":20,
		"protocols":[2,3,7],
		"api":[2],
		"battr":[8,9],
		"bitrate":1200,
		"linearity":1,
		"skippable":true,
		"width":1920,
		"height":1080,
		"codec":"H264",
		"file_size":5242880
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if metadata == nil || metadata.Duration != 20 || metadata.Bitrate != 1200 || metadata.Linearity != 1 || metadata.Width != 1920 || metadata.Height != 1080 || metadata.Codec != "h264" || metadata.FileSize != 5242880 {
		t.Fatalf("unexpected cabinet VIDEO metadata: %#v", metadata)
	}
	if len(metadata.Mimes) != 1 || metadata.Mimes[0] != "video/mp4" || len(metadata.Protocols) != 3 || len(metadata.Attributes) != 2 {
		t.Fatalf("cabinet VIDEO metadata lists not parsed: %#v", metadata)
	}
	if metadata.Skippable == nil || !*metadata.Skippable {
		t.Fatalf("skippable flag not parsed: %#v", metadata)
	}
}

func TestBuildBidGeneratesPrivateVASTAndAppliesMacrosOnlyToAdvertiserURL(t *testing.T) {
	creative := testVideoCreative()
	creative.TrackersMacros = map[string]string{"campaign_id": "cid", "creative_id": "crid", "click_id": "subid"}
	campaign := &Campaign{ID: "campaign-1", Format: "VID"}
	impID := "imp-1"
	imp := &ortb.Imp{Id: &impID, Video: &ortb.Video{Mimes: []string{"video/mp4"}, Protocols: []int32{3}, Placement: i32(1)}}
	siteID := "site-1"
	req := &ortb.BidRequest{Site: &ortb.Site{Id: &siteID}, Imp: []*ortb.Imp{imp}}

	bid := (&AuctionService{}).buildBid(req, imp, campaign, creative, 1.25)
	if bid == nil {
		t.Fatal("VIDEO buildBid returned nil")
	}
	adm := bid.GetAdm()
	if !strings.Contains(adm, `<VAST version="3.0">`) || !strings.Contains(adm, videoImpressionPlaceholder) {
		t.Fatalf("private VAST was not generated: %s", adm)
	}
	if strings.Contains(adm, "/adm?") {
		t.Fatalf("ADV must not wrap the whole VAST or ClickThrough before BidEngine: %s", adm)
	}
	if !strings.Contains(adm, "cid=campaign-1") || !strings.Contains(adm, "crid=v1") {
		t.Fatalf("business macros missing from advertiser destination: %s", adm)
	}
	if strings.Contains(adm, "subid=") {
		t.Fatalf("click_id must be generated at real click time, not auction time: %s", adm)
	}
}
