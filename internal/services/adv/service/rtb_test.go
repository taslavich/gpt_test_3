package auction

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
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
