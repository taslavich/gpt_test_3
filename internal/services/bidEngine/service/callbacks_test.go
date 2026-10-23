package bidEngine

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"html"
	"net/url"
	"strings"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestFinalizeADVPopCallbacksDoNotEmbedDSPCallbacks(t *testing.T) {
	adm := "https://creative.example/render?a=1"
	unexpectedNURL := "https://dsp.example/win"
	unexpectedBURL := "https://dsp.example/bill"
	bid := &ortb.Bid{Adm: &adm, Nurl: &unexpectedNURL, Burl: &unexpectedBURL}

	got, ok := FinalizeADVCallbacks(bid, "callbacks.example", "winner-1", "ssp.example", constants.POP)
	if !ok || got == nil {
		t.Fatal("ADV callback finalization failed")
	}
	assertCallbackQuery(t, got.GetAdm(), "/adm", map[string]string{
		"id": "winner-1", "url": adm, "f": constants.FormatToCodes[constants.POP],
	})
	assertCallbackQuery(t, got.GetNurl(), "/nurl", map[string]string{
		"id": "winner-1", "s": "ssp.example", "f": constants.FormatToCodes[constants.POP],
	})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{
		"id": "winner-1", "f": constants.FormatToCodes[constants.POP],
	})
	assertCallbackQuery(t, got.GetExt().GetCwin(), "/clicks_wins", map[string]string{
		"id": "winner-1",
	})
	assertQueryMissing(t, got.GetNurl(), "url")
	assertQueryMissing(t, got.GetBurl(), "url")

	if bid.GetAdm() != adm || bid.GetNurl() != unexpectedNURL || bid.GetBurl() != unexpectedBURL {
		t.Fatal("source bid was mutated")
	}
}

func TestFinalizeADVBannerImageKeepsHTMLAndWrapsOnlyHref(t *testing.T) {
	adm := `<a href="https://landing.example/path?a=1&amp;b=2" target="_blank"><img src="https://cdn.example/banner.png" width="300" height="250"></a>`
	unexpectedNURL := "https://dsp.example/win"
	unexpectedBURL := "https://dsp.example/bill"
	bid := &ortb.Bid{Adm: &adm, Nurl: &unexpectedNURL, Burl: &unexpectedBURL}

	got, ok := FinalizeADVCallbacks(bid, "callbacks.example", "banner-winner", "ban_mc_test", constants.BAN)
	if !ok || got == nil {
		t.Fatal("ADV banner callback finalization failed")
	}
	if !strings.Contains(got.GetAdm(), `<img src="https://cdn.example/banner.png" width="300" height="250">`) {
		t.Fatalf("banner image markup was changed: %s", got.GetAdm())
	}
	href := bannerHrefFromADM(t, got.GetAdm())
	assertCallbackQuery(t, href, "/adm", map[string]string{
		"id": "banner-winner", "url": "https://landing.example/path?a=1&b=2", "f": constants.FormatToCodes[constants.BAN],
	})
	assertCallbackQuery(t, got.GetNurl(), "/nurl", map[string]string{
		"id": "banner-winner", "s": "ban_mc_test", "f": constants.FormatToCodes[constants.BAN],
	})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{
		"id": "banner-winner", "f": constants.FormatToCodes[constants.BAN],
	})
	assertCallbackQuery(t, got.GetExt().GetCwin(), "/clicks_wins", map[string]string{
		"id": "banner-winner",
	})
	if bid.GetAdm() != adm || bid.GetNurl() != unexpectedNURL || bid.GetBurl() != unexpectedBURL {
		t.Fatal("source banner bid was mutated")
	}
}

func TestFinalizeADVBannerIframeKeepsADMUnchanged(t *testing.T) {
	adm := `<iframe src="https://iframe.example/render?a=1&amp;b=2" width="300" height="250"></iframe>`
	bid := &ortb.Bid{Adm: &adm}

	got, ok := FinalizeADVCallbacks(bid, "callbacks.example", "iframe-winner", "ban_mc_test", constants.BAN)
	if !ok || got == nil {
		t.Fatal("ADV iframe banner callback finalization failed")
	}
	if got.GetAdm() != adm {
		t.Fatalf("iframe ADM changed: got %q want %q", got.GetAdm(), adm)
	}
	if strings.Contains(got.GetAdm(), "/adm?") {
		t.Fatalf("iframe ADM must not contain click wrapper: %q", got.GetAdm())
	}
	assertCallbackQuery(t, got.GetNurl(), "/nurl", map[string]string{
		"id": "iframe-winner", "s": "ban_mc_test", "f": constants.FormatToCodes[constants.BAN],
	})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{
		"id": "iframe-winner", "f": constants.FormatToCodes[constants.BAN],
	})
	assertCallbackQuery(t, got.GetExt().GetCwin(), "/clicks_wins", map[string]string{
		"id": "iframe-winner",
	})
}

func TestFinalizeDSPCallbacksAddsCwinAndPreservesBidExt(t *testing.T) {
	adm := "https://creative.example/render"
	nurl := "https://dsp.example/win"
	btype := int32(1)
	verticalID := int32(42)
	source := &ortb.Bid{
		Adm:  &adm,
		Nurl: &nurl,
		Ext: &ortb.BidExt{
			Btype:      &btype,
			VerticalId: &verticalID,
		},
	}

	got, ok := FinalizeBidCallbacks(
		source,
		"callbacks.example",
		"dsp-winner",
		"ssp.example",
		constants.POP,
		true,
		true,
	)
	if !ok || got == nil {
		t.Fatal("DSP callback finalization failed")
	}
	assertCallbackQuery(t, got.GetNurl(), "/nurl", map[string]string{
		"id": "dsp-winner", "s": "ssp.example", "url": nurl, "f": constants.FormatToCodes[constants.POP],
	})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{
		"id": "dsp-winner", "url": nurl, "f": constants.FormatToCodes[constants.POP],
	})
	assertCallbackQuery(t, got.GetExt().GetCwin(), "/clicks_wins", map[string]string{
		"id": "dsp-winner",
	})
	if got.GetExt().GetBtype() != btype || got.GetExt().GetVerticalId() != verticalID {
		t.Fatalf("existing bid ext was not preserved: %+v", got.GetExt())
	}
	if source.GetExt().GetCwin() != "" {
		t.Fatal("source bid ext was mutated")
	}
}

func bannerHrefFromADM(t *testing.T, adm string) string {
	t.Helper()
	match := bannerAnchorHrefPattern.FindStringSubmatch(adm)
	if match == nil {
		t.Fatalf("banner href not found in ADM: %s", adm)
	}
	href := match[2]
	if href == "" {
		href = match[3]
	}
	return html.UnescapeString(href)
}

func TestFinalizeADVNativeCallbacksWrapOnlyLinkAndAddBURL(t *testing.T) {
	adm := `{"native":{"ver":"1.2","link":{"url":"https://creative.example/render?a=1"},"assets":[{"id":100,"title":{"text":"Title"}}]}}`
	unexpectedNURL := "https://dsp.example/win"
	unexpectedBURL := "https://dsp.example/bill"
	bid := &ortb.Bid{Adm: &adm, Nurl: &unexpectedNURL, Burl: &unexpectedBURL}
	got, ok := FinalizeADVCallbacks(bid, "callbacks.example", "winner-2", "ssp.example", constants.IPP)
	if !ok || got == nil {
		t.Fatal("ADV native callback finalization failed")
	}
	linkURL := nativeLinkURLFromADM(t, got.GetAdm())
	assertCallbackQuery(t, linkURL, "/adm", map[string]string{
		"id": "winner-2", "url": "https://creative.example/render?a=1", "f": constants.FormatToCodes[constants.IPP],
	})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{
		"id": "winner-2", "f": constants.FormatToCodes[constants.IPP],
	})
	assertCallbackQuery(t, got.GetExt().GetCwin(), "/clicks_wins", map[string]string{
		"id": "winner-2",
	})
	if got.GetNurl() != "" {
		t.Fatalf("native ADV response must not contain nurl: %q", got.GetNurl())
	}
	if bid.GetAdm() != adm || bid.GetNurl() != unexpectedNURL || bid.GetBurl() != unexpectedBURL {
		t.Fatal("source native bid was mutated")
	}
}

func nativeLinkURLFromADM(t *testing.T, adm string) string {
	t.Helper()
	var payload struct {
		Native struct {
			Link struct {
				URL string `json:"url"`
			} `json:"link"`
			Assets []json.RawMessage `json:"assets"`
		} `json:"native"`
	}
	if err := json.Unmarshal([]byte(adm), &payload); err != nil {
		t.Fatalf("parse native adm: %v", err)
	}
	if len(payload.Native.Assets) != 1 {
		t.Fatalf("native assets were not preserved: %s", adm)
	}
	return payload.Native.Link.URL
}

func assertCallbackQuery(t *testing.T, raw, path string, expected map[string]string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse callback %q: %v", raw, err)
	}
	if parsed.Path != path {
		t.Fatalf("callback path=%q want %q", parsed.Path, path)
	}
	for key, want := range expected {
		if got := parsed.Query().Get(key); got != want {
			t.Fatalf("callback %s=%q want %q in %q", key, got, want, raw)
		}
	}
}

func assertQueryMissing(t *testing.T, raw, key string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse callback %q: %v", raw, err)
	}
	if _, exists := parsed.Query()[key]; exists {
		t.Fatalf("callback %q unexpectedly contains query key %q", raw, key)
	}
}

func TestDSPBannerKeepsRawADMAndUsesExchangeBURL(t *testing.T) {
	requestID, impID, uuid := "req-ban", "imp-ban", "uuid-ban"
	price := float32(1.0)
	adm := `<a href="https://landing.example"><img src="https://cdn.example/a.png"></a>`
	nurl := "https://dsp.example/win?x=1"
	dspBurl := "https://dsp.example/bill"

	req := &bidEngineGrpc.BidEngineRequest_V2_5{
		BidRequest: &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{}}}},
		BidResponses: map[string]*ortb.BidResponse{
			"dsp-banner": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &impID, Price: &price, Adm: &adm, Nurl: &nurl, Burl: &dspBurl}}}}},
		},
		ImpIdUuid: map[string]string{impID: uuid},
		SspDomain: "ssp.example",
		Format:    constants.BAN,
		Logged:    true,
	}

	response, _, burlUUIDs, admUUIDs := GetWinnerBidInternalWithRoutes_V_2_5(
		context.Background(), req, 0.1, req.ImpIdUuid, nil, true, "ADULT", "callbacks.example",
	)
	bids := response.GetSeatbid()[0].GetBid()
	if len(bids) != 1 {
		t.Fatalf("banner DSP winners=%d want 1", len(bids))
	}
	got := bids[0]
	if got.GetAdm() != adm {
		t.Fatalf("banner ADM changed: got %q want %q", got.GetAdm(), adm)
	}
	assertCallbackQuery(t, got.GetNurl(), "/nurl", map[string]string{
		"id": uuid, "s": "ssp.example", "url": nurl, "f": constants.FormatToCodes[constants.BAN],
	})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{
		"id": uuid, "url": nurl, "f": constants.FormatToCodes[constants.BAN],
	})
	if len(admUUIDs) != 0 {
		t.Fatalf("raw banner ADM must not allocate /adm UUIDs: %v", admUUIDs)
	}
	if len(burlUUIDs) != 1 || burlUUIDs[0] != uuid {
		t.Fatalf("banner BURL UUIDs=%v want [%s]", burlUUIDs, uuid)
	}
}

func TestDSPNativeAndIPPKeepsValidRawJSONADM(t *testing.T) {
	for _, format := range []string{constants.NAT, constants.IPP} {
		t.Run(format, func(t *testing.T) {
			requestID, impID, uuid := "req-"+format, "imp-"+format, "uuid-"+format
			price := float32(1.0)
			adm := `{"native":{"ver":"1.2","link":{"url":"https://landing.example"},"assets":[{"id":100,"title":{"text":"x"}}]}}`
			req := &bidEngineGrpc.BidEngineRequest_V2_5{
				BidRequest: &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &impID, Native: &ortb.Native{}}}},
				BidResponses: map[string]*ortb.BidResponse{
					"dsp-native": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &impID, Price: &price, Adm: &adm}}}}},
				},
				ImpIdUuid: map[string]string{impID: uuid},
				SspDomain: "ssp.example",
				Format:    format,
				Logged:    true,
			}
			response, _, burlUUIDs, admUUIDs := GetWinnerBidInternalWithRoutes_V_2_5(
				context.Background(), req, 0.1, req.ImpIdUuid, nil, true, "MAINSTREAM", "callbacks.example",
			)
			bids := response.GetSeatbid()[0].GetBid()
			if len(bids) != 1 || bids[0].GetAdm() != adm {
				t.Fatalf("%s raw Native ADM was not preserved: %+v", format, bids)
			}
			if len(admUUIDs) != 0 || len(burlUUIDs) != 1 || burlUUIDs[0] != uuid {
				t.Fatalf("%s callback UUIDs: burl=%v adm=%v", format, burlUUIDs, admUUIDs)
			}
		})
	}
}

func TestDSPNativeRejectsMalformedADMJSON(t *testing.T) {
	requestID, impID, uuid := "req-nat-bad", "imp-nat-bad", "uuid-nat-bad"
	price := float32(1.0)
	adm := `{"native":`
	req := &bidEngineGrpc.BidEngineRequest_V2_5{
		BidRequest: &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &impID, Native: &ortb.Native{}}}},
		BidResponses: map[string]*ortb.BidResponse{
			"dsp-native": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &impID, Price: &price, Adm: &adm}}}}},
		},
		ImpIdUuid: map[string]string{impID: uuid},
		SspDomain: "ssp.example",
		Format:    constants.NAT,
		Logged:    true,
	}
	response, _, burlUUIDs, admUUIDs := GetWinnerBidInternalWithRoutes_V_2_5(
		context.Background(), req, 0.1, req.ImpIdUuid, nil, true, "ADULT", "callbacks.example",
	)
	if len(response.GetSeatbid()[0].GetBid()) != 0 {
		t.Fatal("malformed Native ADM must be rejected")
	}
	if len(burlUUIDs) != 0 || len(admUUIDs) != 0 {
		t.Fatalf("malformed Native ADM created callback UUIDs: burl=%v adm=%v", burlUUIDs, admUUIDs)
	}
}

func TestFinalizeADVVideoRewritesOnlyOwnVASTCallbacks(t *testing.T) {
	adm := `<?xml version="1.0"?><VAST version="3.0"><Ad id="x"><InLine><AdSystem>TwinBid</AdSystem><AdTitle>video</AdTitle><Impression>https://invalid.twinbid.local/video-impression</Impression><Creatives><Creative><Linear><Duration>00:00:20</Duration><VideoClicks><ClickThrough>https://advertiser.example/landing?a=1&amp;b=2</ClickThrough></VideoClicks><MediaFiles><MediaFile delivery="progressive" type="video/mp4" width="1920" height="1080">https://cdn.example/video.mp4</MediaFile></MediaFiles></Linear></Creative></Creatives></InLine></Ad></VAST>`
	dspNURL := "https://should-be-discarded.example/win"
	dspBURL := "https://should-be-discarded.example/bill"
	bid := &ortb.Bid{Adm: &adm, Nurl: &dspNURL, Burl: &dspBURL}

	got, ok := FinalizeADVCallbacks(bid, "callbacks.example", "video-winner", "vid_mc_test", constants.VID)
	if !ok || got == nil {
		t.Fatal("VIDEO callback finalization failed")
	}
	if got.GetAdm() == adm {
		t.Fatal("private ADV VAST was not finalized")
	}
	if !validVASTADM(got.GetAdm()) {
		t.Fatalf("finalized VAST is invalid: %s", got.GetAdm())
	}
	if strings.Contains(got.GetAdm(), "invalid.twinbid.local") {
		t.Fatalf("private VIDEO impression placeholder leaked: %s", got.GetAdm())
	}
	if strings.Contains(got.GetAdm(), "ClickTracking") || strings.Contains(got.GetAdm(), "TrackingEvents") {
		t.Fatalf("unexpected VIDEO tracking added: %s", got.GetAdm())
	}
	if !strings.Contains(got.GetAdm(), "https://callbacks.example/video_impression") {
		t.Fatalf("no-op impression URL missing: %s", got.GetAdm())
	}

	var parsed advVideoClickVAST
	if err := xml.Unmarshal([]byte(got.GetAdm()), &parsed); err != nil {
		t.Fatal(err)
	}
	clickThrough := parsed.Ad.Inline.Creatives.Creative.Linear.VideoClicks.ClickThrough
	parsedClick, err := url.Parse(clickThrough)
	if err != nil {
		t.Fatal(err)
	}
	if parsedClick.Path != "/adm" || parsedClick.Query().Get("id") != "video-winner" || parsedClick.Query().Get("f") != constants.FormatToCodes[constants.VID] {
		t.Fatalf("unexpected VIDEO ClickThrough: %s", clickThrough)
	}
	if gotURL := parsedClick.Query().Get("url"); gotURL != "https://advertiser.example/landing?a=1&b=2" {
		t.Fatalf("advertiser destination changed: %q", gotURL)
	}
	if !strings.Contains(got.GetAdm(), "https://cdn.example/video.mp4") {
		t.Fatalf("media URL was changed: %s", got.GetAdm())
	}
	assertCallbackQuery(t, got.GetNurl(), "/nurl", map[string]string{"id": "video-winner", "s": "vid_mc_test", "f": constants.FormatToCodes[constants.VID]})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{"id": "video-winner", "f": constants.FormatToCodes[constants.VID]})
	if bid.GetNurl() != dspNURL || bid.GetBurl() != dspBURL || bid.GetAdm() != adm {
		t.Fatal("source VIDEO bid was mutated")
	}
}

func TestVideoDSPADMValidation(t *testing.T) {
	if !validRawDSPADM(constants.VID, `<VAST version="3.0"></VAST>`) {
		t.Fatal("valid VAST rejected")
	}
	if validRawDSPADM(constants.VID, `<html></html>`) {
		t.Fatal("non-VAST markup accepted")
	}
	if validRawDSPADM(constants.VID, `<VAST version="3.0"></VAST><broken`) {
		t.Fatal("VAST with malformed trailing XML must be rejected")
	}
	if validRawDSPADM(constants.VID, `<VAST version="3.0"></VAST><Other/>`) {
		t.Fatal("VAST with a second top-level element must be rejected")
	}
	if validRawDSPADM(constants.VID, `<VAST version="3.0"></VAST>garbage`) {
		t.Fatal("VAST with non-whitespace trailing text must be rejected")
	}
	if shouldWrapDSPADM(constants.VID) {
		t.Fatal("VIDEO DSP ADM must not be POP-wrapped")
	}
}

func TestFinalizeADVVideoVAST4PreservesNamespaceAndMediaCDATA(t *testing.T) {
	adm := `<?xml version="1.0" encoding="UTF-8"?><VAST xmlns="http://www.iab.com/VAST" version="4.0"><Ad id="v1"><InLine><AdSystem version="1.0">TwinBid</AdSystem><Impression>https://invalid.twinbid.local/video-impression</Impression><AdTitle>video</AdTitle><Creatives><Creative><UniversalAdId idRegistry="twinbidexchange.com" idValue="v1">v1</UniversalAdId><Linear><Duration>00:00:20</Duration><MediaFiles><MediaFile delivery="progressive" type="video/mp4" width="1920" height="1080"><![CDATA[https://cdn.example/video.mp4?a=1&b=2]]></MediaFile></MediaFiles><VideoClicks><ClickThrough><![CDATA[https://advertiser.example/landing?a=1&b=2]]></ClickThrough></VideoClicks></Linear></Creative></Creatives></InLine></Ad></VAST>`
	bid := &ortb.Bid{Adm: &adm}

	got, ok := FinalizeADVCallbacks(bid, "callbacks.example", "video-v4", "vid_mc_test", constants.VID)
	if !ok || got == nil {
		t.Fatal("VAST 4 callback finalization failed")
	}
	finalADM := got.GetAdm()
	if !strings.Contains(finalADM, `<VAST xmlns="http://www.iab.com/VAST" version="4.0">`) {
		t.Fatalf("VAST 4 namespace changed: %s", finalADM)
	}
	if strings.Count(finalADM, `xmlns="http://www.iab.com/VAST"`) != 1 {
		t.Fatalf("VAST 4 namespace duplicated: %s", finalADM)
	}
	if !strings.Contains(finalADM, `<![CDATA[https://cdn.example/video.mp4?a=1&b=2]]>`) {
		t.Fatalf("MediaFile CDATA was not preserved: %s", finalADM)
	}
	if !strings.Contains(finalADM, `<ClickThrough><![CDATA[https://callbacks.example/adm?`) {
		t.Fatalf("ClickThrough was not rewritten as CDATA: %s", finalADM)
	}
	if strings.Index(finalADM, "<MediaFiles>") > strings.Index(finalADM, "<VideoClicks>") {
		t.Fatalf("VAST 4 Linear order changed: %s", finalADM)
	}
	if !validVASTADM(finalADM) {
		t.Fatalf("finalized VAST 4 is not well-formed: %s", finalADM)
	}
}
