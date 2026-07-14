package bidEngineWeb

import (
	"context"
	"net/url"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestReadyADVResponseSkipsAuctionButFinalizesCallbacks(t *testing.T) {
	impID := "imp-1"
	bidID := "bid-1"
	adm := "https://creative.example/render"
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
	assertReadyCallback(t, finalBid.GetAdm(), "/adm", "winner-uuid", adm)
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
