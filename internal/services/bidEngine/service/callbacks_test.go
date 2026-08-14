package bidEngine

import (
	"context"
	"encoding/json"
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
		"id": uuid, "f": constants.FormatToCodes[constants.BAN],
	})
	assertQueryMissing(t, got.GetBurl(), "url")
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
