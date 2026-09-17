package bidEngineWeb

import (
	"context"
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	bidEngineService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
)

func TestReadyADVResponseUsesUnifiedInternalPathAndPreservesBehavior(t *testing.T) {
	requestID := "request-1"
	impID := "imp-1"
	bidID := "bid-1"
	adm := `<a href="https://creative.example/render?a=1&amp;b=2" target="_blank"><img src="https://cdn.example/banner.png" width="300" height="250"></a>`
	price := float32(1.25)

	readyBid := &ortb.Bid{
		Id:    &bidID,
		Impid: &impID,
		Price: &price,
		Adm:   &adm,
	}
	currency := "USD"
	bidResponseID := "adv-response-id"
	readyResponse := &ortb.BidResponse{
		Id:    &requestID,
		Bidid: &bidResponseID,
		Cur:   &currency,
		Seatbid: []*ortb.SeatBid{
			{Bid: []*ortb.Bid{readyBid}},
		},
	}
	request := &bidEngineGrpc.BidEngineRequest_V2_5{
		BidRequest: &ortb.BidRequest{
			Id:  &requestID,
			Imp: []*ortb.Imp{{Id: &impID}},
		},
		ReadyBidResponse: readyResponse,
		ImpIdUuid:        map[string]string{impID: "winner-uuid"},
		WinnerUserIds:    map[string]string{impID: "user-1"},
		WinnerBasePrices: map[string]float64{impID: 2.5},
		SspDomain:        "ssp.example",
		Format:           constants.BAN,
		Logged:           false,
		Rekl:             true,
	}

	server := &Server{
		admDomain:                  "callbacks.example",
		GetWinnerBidInternal_V_2_5: bidEngineService.GetWinnerBidInternal_V_2_5,
	}
	response, err := server.GetWinnerBid_V2_5(context.Background(), request)
	if err != nil {
		t.Fatalf("GetWinnerBid_V2_5 returned error: %v", err)
	}
	if response.GetCode() != 200 || response.GetBidResponse() == nil {
		t.Fatalf("unexpected ready ADV response: code=%d response=%v", response.GetCode(), response.GetBidResponse())
	}
	if !response.GetRekl() {
		t.Fatal("all-ADV response must preserve rekl=true compatibility flag")
	}
	if response.GetBidResponse().GetBidid() != bidResponseID || response.GetBidResponse().GetCur() != currency {
		t.Fatalf("all-ADV response envelope was not preserved: bidid=%q cur=%q", response.GetBidResponse().GetBidid(), response.GetBidResponse().GetCur())
	}
	if got := response.GetWinnerUserIds()[impID]; got != "user-1" {
		t.Fatalf("winner user=%q want user-1", got)
	}
	if got := response.GetImpIdUuidClone()[impID]; got != "winner-uuid" {
		t.Fatalf("winner uuid=%q want winner-uuid", got)
	}
	bids := response.GetBidResponse().GetSeatbid()[0].GetBid()
	if len(bids) != 1 {
		t.Fatalf("got %d bids, want 1", len(bids))
	}
	finalBid := bids[0]
	if !strings.Contains(finalBid.GetAdm(), `<img src="https://cdn.example/banner.png" width="300" height="250">`) {
		t.Fatalf("banner image markup was changed: %q", finalBid.GetAdm())
	}
	assertReadyCallback(t, readyBannerHref(t, finalBid.GetAdm()), "/adm", "winner-uuid", "https://creative.example/render?a=1&b=2")
	assertReadyCallback(t, finalBid.GetNurl(), "/nurl", "winner-uuid", "")
	assertReadyCallback(t, finalBid.GetBurl(), "/burl", "winner-uuid", "")
	if parsed, err := url.Parse(finalBid.GetNurl()); err != nil {
		t.Fatalf("parse ADV NURL: %v", err)
	} else if _, exists := parsed.Query()["url"]; exists {
		t.Fatalf("ADV NURL must not contain downstream url: %q", finalBid.GetNurl())
	}
}

func assertReadyCallback(t *testing.T, raw, path, winnerID, original string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse callback %q: %v", raw, err)
	}
	if parsed.Path != path || parsed.Query().Get("id") != winnerID {
		t.Fatalf("unexpected callback %q", raw)
	}
	if original != "" && parsed.Query().Get("url") != original {
		t.Fatalf("callback original URL=%q want %q", parsed.Query().Get("url"), original)
	}
}

var readyBannerHrefPattern = regexp.MustCompile(`(?is)<a\b[^>]*\bhref\s*=\s*(?:"([^"]*)"|'([^']*)')`)

func readyBannerHref(t *testing.T, adm string) string {
	t.Helper()
	match := readyBannerHrefPattern.FindStringSubmatch(adm)
	if match == nil {
		t.Fatalf("banner href not found in ADM: %q", adm)
	}
	href := match[1]
	if href == "" {
		href = match[2]
	}
	return html.UnescapeString(href)
}

func TestMixedADVAndDSPResponseUsesOneBidEnginePath(t *testing.T) {
	requestID := "request-mixed"
	impADV, impDSP := "imp-adv", "imp-dsp"
	uuidADV, uuidDSP := "uuid-adv", "uuid-dsp"
	advPrice := float32(0.4)
	dspPrice := float32(1.0)
	advADM := "https://adv.example/adm"
	dspADM := "https://dsp.example/adm"

	request := &bidEngineGrpc.BidEngineRequest_V2_5{
		BidRequest: &ortb.BidRequest{
			Id:  &requestID,
			Imp: []*ortb.Imp{{Id: &impADV}, {Id: &impDSP}},
		},
		ReadyBidResponse: &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
			{Impid: &impADV, Price: &advPrice, Adm: &advADM},
		}}}},
		BidResponses: map[string]*ortb.BidResponse{
			"dsp.example": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
				{Impid: &impDSP, Price: &dspPrice, Adm: &dspADM},
			}}}},
		},
		ImpIdUuid:        map[string]string{impADV: uuidADV, impDSP: uuidDSP},
		WinnerUserIds:    map[string]string{impADV: "user-adv"},
		WinnerBasePrices: map[string]float64{impADV: 0.5},
		SspDomain:        "ssp.example",
		Format:           constants.POP,
		Logged:           false,
		Rekl:             false,
	}

	server := &Server{
		ProfitPercent:              0.1,
		admDomain:                  "callbacks.example",
		GetWinnerBidInternal_V_2_5: bidEngineService.GetWinnerBidInternal_V_2_5,
	}
	response, err := server.GetWinnerBid_V2_5(context.Background(), request)
	if err != nil {
		t.Fatalf("GetWinnerBid_V2_5 returned error: %v", err)
	}
	if response.GetCode() != 200 || response.GetBidResponse() == nil {
		t.Fatalf("unexpected mixed response: code=%d response=%v", response.GetCode(), response.GetBidResponse())
	}
	if response.GetRekl() {
		t.Fatal("mixed ADV+DSP response must use rekl=false compatibility flag")
	}
	if got := len(response.GetBidResponse().GetSeatbid()[0].GetBid()); got != 2 {
		t.Fatalf("mixed final bids=%d want 2", got)
	}
	if got := response.GetWinnerUserIds()[impADV]; got != "user-adv" {
		t.Fatalf("ADV winner user=%q want user-adv", got)
	}
	if _, exists := response.GetWinnerUserIds()[impDSP]; exists {
		t.Fatal("DSP impression must not be reported as an ADV winner user")
	}
	if got := response.GetImpIdUuidClone()[impADV]; got != uuidADV {
		t.Fatalf("ADV uuid clone=%q want %q", got, uuidADV)
	}
	if got := response.GetImpIdUuidClone()[impDSP]; got != uuidDSP {
		t.Fatalf("DSP uuid clone=%q want %q", got, uuidDSP)
	}
}
