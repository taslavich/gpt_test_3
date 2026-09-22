package auction

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestCallRTBCampaignChoosesMaxBidAndKeepsPerImpStatus(t *testing.T) {
	requestID := "req-1"
	imp1, imp2, imp3 := "imp-1", "imp-2", "imp-3"
	bidfloor := float32(0.75)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var got ortb.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode RTB request: %v", err)
		}
		if len(got.GetImp()) != 3 {
			t.Fatalf("impressions=%d want 3", len(got.GetImp()))
		}
		if got.GetImp()[0].GetBidfloor() != bidfloor {
			t.Fatalf("bidfloor=%v want %v; RTB request must preserve incoming bidfloor", got.GetImp()[0].GetBidfloor(), bidfloor)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp","seatbid":[{"bid":[` +
			`{"id":"low","impid":"imp-1","price":1.00,"adm":"https://buyer.example/low"},` +
			`{"id":"high","impid":"imp-1","price":1.20,"adm":"https://buyer.example/high"},` +
			`{"id":"bad-adm","impid":"imp-2","price":2.00,"adm":""}` +
			`]}]}`))
	}))
	defer server.Close()

	s := &AuctionService{rtbHTTPClient: server.Client()}
	campaign := &Campaign{ID: "campaign-rtb", RTB: true, DSPLink: server.URL}
	source := &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{
		{Id: &imp1, Bidfloor: &bidfloor},
		{Id: &imp2},
		{Id: &imp3},
	}}

	result := s.callRTBCampaign(context.Background(), source, campaign, source.GetImp(), constants.POP, func(string, ...any) {})
	if got := result.bids[imp1]; got == nil || got.GetId() != "high" || got.GetPrice() != 1.20 {
		t.Fatalf("imp1 selected bid=%+v want max valid bid high/1.20", got)
	}
	if _, ok := result.bids[imp2]; ok {
		t.Fatal("imp2 bid without ADM must not participate")
	}
	if _, ok := result.bids[imp3]; ok {
		t.Fatal("imp3 no-bid must not participate")
	}
	if got := result.codes[imp1]; got != "200" {
		t.Fatalf("imp1 status=%q want 200", got)
	}
	if got := result.codes[imp2]; got != "800" {
		t.Fatalf("imp2 status=%q want 800", got)
	}
	if got := result.codes[imp3]; got != "200" {
		t.Fatalf("imp3 no-bid status=%q want 200", got)
	}
}

func TestCallRTBCampaignPreservesOriginalBidfloorInOutboundOpenRTB(t *testing.T) {
	impID := "imp-bidfloor"
	bidfloor := float32(0.83)
	bidfloorcur := "EUR"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var got ortb.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode outbound RTB request: %v", err)
		}
		if len(got.GetImp()) != 1 {
			t.Fatalf("outbound impressions=%d want 1", len(got.GetImp()))
		}
		gotImp := got.GetImp()[0]
		if gotImp.GetBidfloor() != bidfloor {
			t.Fatalf("outbound bidfloor=%v want original %v", gotImp.GetBidfloor(), bidfloor)
		}
		if gotImp.GetBidfloorcur() != bidfloorcur {
			t.Fatalf("outbound bidfloorcur=%q want original %q", gotImp.GetBidfloorcur(), bidfloorcur)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	s := &AuctionService{rtbHTTPClient: server.Client()}
	campaign := &Campaign{ID: "campaign-bidfloor-passthrough", RTB: true, DSPLink: server.URL}
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{
		Id:          &impID,
		Bidfloor:    &bidfloor,
		Bidfloorcur: &bidfloorcur,
	}}}

	result := s.callRTBCampaign(context.Background(), source, campaign, source.GetImp(), constants.POP, func(string, ...any) {})
	if got := result.codes[impID]; got != "204" {
		t.Fatalf("status=%q want 204", got)
	}
	if source.GetImp()[0].GetBidfloor() != bidfloor || source.GetImp()[0].GetBidfloorcur() != bidfloorcur {
		t.Fatalf("source request was mutated: bidfloor=%v bidfloorcur=%q", source.GetImp()[0].GetBidfloor(), source.GetImp()[0].GetBidfloorcur())
	}
}

func TestCallRTBCampaignDoesNotApplyDSPChangers(t *testing.T) {
	const dspDomain = "rtb-regression-dsp"
	impID := "imp-no-changer"
	originalSiteID := "publisher-site"
	changedSiteID := "changer-site"

	// Prove the request is one the existing DSP changer would actually modify
	// in the ordinary router flow. The RTB ADV flow must bypass that changer.
	changer := &filter.ChangersBoxChanger{Changers: map[string]*filter.ChangersChanger{
		dspDomain: {
			Apply: true,
			SiteIdBoxes: map[string]*filter.SiteIdBoxChanger{
				originalSiteID: {ToChange: true, SiteId: changedSiteID},
			},
		},
	}}
	probe := &ortb.BidRequest{
		Site: &ortb.Site{Id: &originalSiteID},
		Imp:  []*ortb.Imp{{Id: &impID}},
	}
	changed, ok := changer.Change(probe, dspDomain)
	if !ok || changed.GetSite().GetId() != changedSiteID {
		t.Fatalf("test setup invalid: DSP changer did not change site id, changed=%v site_id=%q", ok, changed.GetSite().GetId())
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var got ortb.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode outbound RTB request: %v", err)
		}
		if got.GetSite().GetId() != originalSiteID {
			t.Fatalf("RTB outbound site.id=%q want untouched %q; DSP changers must not run in ADV RTB flow", got.GetSite().GetId(), originalSiteID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	s := &AuctionService{rtbHTTPClient: server.Client()}
	campaign := &Campaign{ID: "campaign-no-changer", RTB: true, DSPLink: server.URL}
	source := &ortb.BidRequest{
		Site: &ortb.Site{Id: &originalSiteID},
		Imp:  []*ortb.Imp{{Id: &impID}},
	}
	result := s.callRTBCampaign(context.Background(), source, campaign, source.GetImp(), constants.POP, func(string, ...any) {})
	if got := result.codes[impID]; got != "204" {
		t.Fatalf("status=%q want 204", got)
	}
	if source.GetSite().GetId() != originalSiteID {
		t.Fatalf("source request was mutated: site.id=%q want %q", source.GetSite().GetId(), originalSiteID)
	}
}

func TestCallRTBCampaignTimeoutDropsCampaignForAllSentImpressions(t *testing.T) {
	imp1, imp2 := "imp-1", "imp-2"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	s := &AuctionService{rtbHTTPClient: server.Client()}
	campaign := &Campaign{ID: "campaign-timeout", RTB: true, DSPLink: server.URL}
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp1}, {Id: &imp2}}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	result := s.callRTBCampaign(ctx, source, campaign, source.GetImp(), constants.POP, func(string, ...any) {})
	if len(result.bids) != 0 {
		t.Fatalf("timeout returned bids: %+v", result.bids)
	}
	if result.codes[imp1] != "1" || result.codes[imp2] != "1" {
		t.Fatalf("timeout statuses=%v want both 1", result.codes)
	}
}

func TestBuildExternalADVBidUsesLocalCampaignAndDSPCallbackSemantics(t *testing.T) {
	price := float32(1.25)
	adm := "https://buyer.example/adm"
	nurl := "https://buyer.example/nurl"
	burl := "https://buyer.example/burl"
	externalCID := "external-cid"
	crid := "external-crid"
	adid := "external-adid"
	remote := &ortb.Bid{Price: &price, Adm: &adm, Nurl: &nurl, Burl: &burl, Cid: &externalCID, Crid: &crid, Adid: &adid}

	bid := buildExternalADVBid(candidate{
		campaign:       &Campaign{ID: "local-campaign"},
		originalBid:    1.25,
		effectivePrice: 0.875,
		externalBid:    remote,
	})
	if bid == nil {
		t.Fatal("external ADV bid is nil")
	}
	if bid.GetCid() != "local-campaign" {
		t.Fatalf("cid=%q want local campaign id", bid.GetCid())
	}
	if bid.GetCrid() != crid {
		t.Fatalf("crid=%q want external crid %q", bid.GetCrid(), crid)
	}
	if bid.GetNurl() != nurl {
		t.Fatalf("nurl=%q want preserved downstream nurl", bid.GetNurl())
	}
	if bid.GetBurl() != "" {
		t.Fatalf("external burl=%q must be discarded like ordinary DSP burl", bid.GetBurl())
	}
	if bid.GetAdid() != adid {
		t.Fatalf("external adid=%q want preserved %q", bid.GetAdid(), adid)
	}
	if bid.GetExt().GetCwin() != constants.ExternalADVBidMarker {
		t.Fatalf("internal marker=%q want %q", bid.GetExt().GetCwin(), constants.ExternalADVBidMarker)
	}
}

func TestCallRTBCampaignStripsOnlyInternalADVFormatMarker(t *testing.T) {
	impID := "imp-1"
	marker := constants.ADVImpressionFormatMarkerPrefix + constants.BAN
	userExt := "publisher-value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var got ortb.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode RTB request: %v", err)
		}
		ext := got.GetImp()[0].GetBanner().GetExt()
		if len(ext) != 1 || ext[0] != userExt {
			t.Fatalf("banner.ext=%v want only user value %q", ext, userExt)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	s := &AuctionService{rtbHTTPClient: server.Client()}
	campaign := &Campaign{ID: "campaign-rtb", RTB: true, DSPLink: server.URL}
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{Ext: []string{userExt, marker}}}}}
	result := s.callRTBCampaign(context.Background(), source, campaign, source.GetImp(), constants.BAN, func(string, ...any) {})
	if result.codes[impID] != "204" {
		t.Fatalf("status=%q want 204", result.codes[impID])
	}
	if got := source.GetImp()[0].GetBanner().GetExt(); len(got) != 2 {
		t.Fatalf("source request was mutated: %v", got)
	}
}

func TestUnsafeRTBIPBlocksInternalRanges(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.1.2.3", "100.64.1.1", "169.254.169.254", "192.168.1.10", "::1", "fd00::1", "fe80::1"}
	for _, raw := range blocked {
		if !unsafeRTBIP(net.ParseIP(raw)) {
			t.Fatalf("%s must be blocked", raw)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"}
	for _, raw := range allowed {
		if unsafeRTBIP(net.ParseIP(raw)) {
			t.Fatalf("%s must be allowed", raw)
		}
	}
}

func Test54AdsRTBRequestNormalizationForPop(t *testing.T) {
	requestID := "req-54ads"
	imp1ID, imp2ID := "imp-1", "imp-2"
	subID := "publisher-subid"
	deviceType := int32(4)
	country := "CN"
	domain := "https://example.com/some/path"
	page := "/landing"
	tmax := int32(100)
	bannerW := int32(320)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode 54Ads RTB request: %v", err)
		}
		if got["tmax"] != float64(500) {
			t.Fatalf("tmax=%v want 500", got["tmax"])
		}

		device, _ := got["device"].(map[string]any)
		if _, exists := device["deviceType"]; exists {
			t.Fatalf("deviceType must not be sent to 54Ads: %+v", device)
		}
		if device["devicetype"] != float64(4) {
			t.Fatalf("devicetype=%v want 4", device["devicetype"])
		}
		geo, _ := device["geo"].(map[string]any)
		if geo["country"] != "CHN" {
			t.Fatalf("device.geo.country=%v want CHN", geo["country"])
		}

		site, _ := got["site"].(map[string]any)
		if site["domain"] != "example.com" {
			t.Fatalf("site.domain=%v want example.com", site["domain"])
		}
		if site["page"] != "https://example.com/landing" {
			t.Fatalf("site.page=%v want full URL", site["page"])
		}

		imps, _ := got["imp"].([]any)
		if len(imps) != 2 {
			t.Fatalf("impressions=%d want 2", len(imps))
		}
		for i, rawImp := range imps {
			imp, _ := rawImp.(map[string]any)
			if _, exists := imp["banner"]; exists {
				t.Fatalf("imp[%d].banner must be removed for POP: %+v", i, imp)
			}
			ext, _ := imp["ext"].(map[string]any)
			if ext["type"] != "pop" {
				t.Fatalf("imp[%d].ext.type=%v want pop", i, ext["type"])
			}
		}
		firstImp := imps[0].(map[string]any)
		firstExt := firstImp["ext"].(map[string]any)
		if firstExt["subid"] != subID {
			t.Fatalf("imp[0].ext.subid=%v want %q", firstExt["subid"], subID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	s := &AuctionService{rtbHTTPClient: server.Client()}
	campaign := &Campaign{ID: "campaign-54ads", UserID: rtb54AdsUserID, RTB: true, DSPLink: server.URL}
	source := &ortb.BidRequest{
		Id:   &requestID,
		Tmax: &tmax,
		Imp: []*ortb.Imp{
			{Id: &imp1ID, Ext: &ortb.Imp_Ext{Subid: &subID}, Banner: &ortb.Banner{W: &bannerW}},
			{Id: &imp2ID, Banner: &ortb.Banner{W: &bannerW}},
		},
		Device: &ortb.Device{DeviceType: &deviceType, Geo: &ortb.Geo{Country: &country}},
		Site:   &ortb.Site{Domain: &domain, Page: &page},
	}

	result := s.callRTBCampaign(context.Background(), source, campaign, source.GetImp(), constants.POP, func(string, ...any) {})
	if result.codes[imp1ID] != "204" || result.codes[imp2ID] != "204" {
		t.Fatalf("statuses=%v want both 204", result.codes)
	}
	if source.GetTmax() != 100 || source.GetDevice().GetGeo().GetCountry() != "CN" || source.GetImp()[0].GetBanner() == nil {
		t.Fatal("source request must not be mutated")
	}
}

func Test54AdsRTBRequestKeepsBannerOutsidePop(t *testing.T) {
	impID := "imp-ban"
	bannerW := int32(300)
	campaign := &Campaign{ID: "campaign-54ads", UserID: rtb54AdsUserID, RTB: true}
	req := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{W: &bannerW}}}}

	body, err := marshalRTBRequestForCampaign(req, campaign, constants.BAN)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	imps := got["imp"].([]any)
	imp := imps[0].(map[string]any)
	if _, exists := imp["banner"]; !exists {
		t.Fatal("banner must be preserved outside POP")
	}
	ext := imp["ext"].(map[string]any)
	if ext["type"] != "pop" {
		t.Fatalf("imp.ext.type=%v want pop", ext["type"])
	}
}

func TestBidFlyRTBNURLUsesRawBidPrice(t *testing.T) {
	requestID := "req-bidfly"
	impID := "imp-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp","seatbid":[{"bid":[{"id":"b1","impid":"imp-1","price":1.2,"adm":"https://creative.example/render","nurl":"https://dsp.bidfly.net/rtb/win?rid=20985490157&cid=259&price=${AUCTION_PRICE}&click_id=abc&eid=47"}]}]}`))
	}))
	defer server.Close()

	s := &AuctionService{rtbHTTPClient: server.Client()}
	campaign := &Campaign{ID: "campaign-bidfly", UserID: rtbBidFlyUserID, RTB: true, DSPLink: server.URL}
	source := &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &impID}}}

	result := s.callRTBCampaign(context.Background(), source, campaign, source.GetImp(), constants.POP, func(string, ...any) {})
	bid := result.bids[impID]
	if bid == nil {
		t.Fatal("expected selected RTB bid")
	}
	parsed, err := url.Parse(bid.GetNurl())
	if err != nil {
		t.Fatalf("parse prepared nurl: %v", err)
	}
	if got := parsed.Query().Get("price"); got != "1.2" {
		t.Fatalf("nurl price=%q want raw RTB bid price 1.2; nurl=%q", got, bid.GetNurl())
	}
	if bid.GetPrice() != 1.2 {
		t.Fatalf("raw bid price changed: %v", bid.GetPrice())
	}
}

func TestBidFlyRTBNURLAddsMissingPrice(t *testing.T) {
	got := rtbNURLWithRawBidPrice("https://dsp.bidfly.net/rtb/win?rid=1&cid=2", 0.75)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("price") != "0.75" {
		t.Fatalf("missing price was not added: %q", got)
	}
	if parsed.Query().Get("rid") != "1" || parsed.Query().Get("cid") != "2" {
		t.Fatalf("existing query parameters changed: %q", got)
	}
}

func TestBannerPartnerBANRequestRequirements(t *testing.T) {
	impID := "imp-banner"
	ua := "Mozilla/5.0"
	ip := "203.0.113.10"
	w := int32(300)
	req := &ortb.BidRequest{
		Device: &ortb.Device{Ua: &ua, Ip: &ip},
		Imp:    []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{W: &w}}},
	}
	if reason := bannerPartnerBANRequestRejectReason(req, req.GetImp()[0]); reason != "" {
		t.Fatalf("valid banner request rejected: %q", reason)
	}

	noBanner := &ortb.Imp{Id: &impID}
	if reason := bannerPartnerBANRequestRejectReason(req, noBanner); reason != "banner_partner_banner_missing" {
		t.Fatalf("missing banner reason=%q", reason)
	}

	reqNoUA := &ortb.BidRequest{Device: &ortb.Device{Ip: &ip}, Imp: req.GetImp()}
	if reason := bannerPartnerBANRequestRejectReason(reqNoUA, reqNoUA.GetImp()[0]); reason != "banner_partner_device_ua_missing" {
		t.Fatalf("missing ua reason=%q", reason)
	}

	reqNoIP := &ortb.BidRequest{Device: &ortb.Device{Ua: &ua}, Imp: req.GetImp()}
	if reason := bannerPartnerBANRequestRejectReason(reqNoIP, reqNoIP.GetImp()[0]); reason != "banner_partner_device_ip_missing" {
		t.Fatalf("missing ip reason=%q", reason)
	}
}

func TestBannerPartnerNormalizationOnlyForBAN(t *testing.T) {
	requestID := "req-banner-partner"
	impID := "imp-banner"
	ua := "Mozilla/5.0"
	ip := "203.0.113.10"
	w := int32(300)
	h := int32(250)
	nativeRequest := `{"native":{"ver":"1.2"}}`
	campaign := &Campaign{ID: "campaign-banner-partner", UserID: rtbBannerPartnerUserID, RTB: true}
	req := &ortb.BidRequest{
		Id:     &requestID,
		Device: &ortb.Device{Ua: &ua, Ip: &ip},
		Imp: []*ortb.Imp{{
			Id:     &impID,
			Banner: &ortb.Banner{W: &w, H: &h},
			Native: &ortb.Native{Request: &nativeRequest},
		}},
	}

	banBody, err := marshalRTBRequestForCampaign(req, campaign, constants.BAN)
	if err != nil {
		t.Fatalf("marshal BAN request: %v", err)
	}
	var ban map[string]any
	if err := json.Unmarshal(banBody, &ban); err != nil {
		t.Fatalf("decode BAN request: %v", err)
	}
	banImp := ban["imp"].([]any)[0].(map[string]any)
	if _, ok := banImp["banner"]; !ok {
		t.Fatal("BAN request must preserve banner")
	}
	if _, ok := banImp["native"]; ok {
		t.Fatal("BAN partner request must omit parallel native payload")
	}

	popBody, err := marshalRTBRequestForCampaign(req, campaign, constants.POP)
	if err != nil {
		t.Fatalf("marshal non-BAN request: %v", err)
	}
	var pop map[string]any
	if err := json.Unmarshal(popBody, &pop); err != nil {
		t.Fatalf("decode non-BAN request: %v", err)
	}
	popImp := pop["imp"].([]any)[0].(map[string]any)
	if _, ok := popImp["native"]; !ok {
		t.Fatal("non-BAN campaign must keep generic RTB request unchanged")
	}
	if req.GetImp()[0].GetNative() == nil {
		t.Fatal("source request must not be mutated")
	}
}
