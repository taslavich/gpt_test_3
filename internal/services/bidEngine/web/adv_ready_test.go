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
)

func TestReadyADVResponseSkipsAuctionButFinalizesCallbacks(t *testing.T) {
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
	readyResponse := &ortb.BidResponse{
		Seatbid: []*ortb.SeatBid{
			{Bid: []*ortb.Bid{readyBid}},
		},
	}
	request := &bidEngineGrpc.BidEngineRequest_V2_5{
		BidRequest:       &ortb.BidRequest{},
		ReadyBidResponse: readyResponse,
		ImpIdUuid:        map[string]string{impID: "winner-uuid"},
		WinnerUserIds:    map[string]string{impID: "user-1"},
		WinnerBasePrices: map[string]float64{impID: 2.5},
		SspDomain:        "ssp.example",
		Format:           constants.BAN,
		Logged:           false,
		Rekl:             true,
	}

	server := &Server{admDomain: "callbacks.example"}
	response := server.handleReadyADVResponse(context.Background(), request)
	if response.GetCode() != 200 || response.GetBidResponse() == nil {
		t.Fatalf("unexpected ready ADV response: code=%d response=%v", response.GetCode(), response.GetBidResponse())
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
