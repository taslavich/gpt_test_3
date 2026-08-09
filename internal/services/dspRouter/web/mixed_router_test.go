package dspRouterWeb

import (
	"context"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/grpc"
)

type fakeADVClient struct {
	response *advGrpc.DoAuctionResponse
	err      error
	calls    int
}

func (f *fakeADVClient) DoAuction(_ context.Context, _ *advGrpc.DoAuctionRequest, _ ...grpc.CallOption) (*advGrpc.DoAuctionResponse, error) {
	f.calls++
	return f.response, f.err
}

func TestRouterAllADVReturnsBeforeDSPPath(t *testing.T) {
	requestID := "req-all-adv"
	imp1, imp2 := "imp-1", "imp-2"
	price := float32(1)
	advClient := &fakeADVClient{response: &advGrpc.DoAuctionResponse{
		BidResponse: &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
			{Impid: &imp1, Price: &price},
			{Impid: &imp2, Price: &price},
		}}}},
		WinnerUserIds:    map[string]string{imp1: "user-1", imp2: "user-2"},
		WinnerBasePrices: map[string]float64{imp1: 1.1, imp2: 1.2},
	}}

	server := &Server{
		advClient:                     advClient,
		dspEndpoints_adult_v_2_5:      map[string]string{"http://127.0.0.1:1": "dsp-should-not-be-called"},
		dspEndpoints_mainstream_v_2_5: map[string]string{},
	}
	req := &dspRouterGrpc.DspRouterRequest_V2_5{
		BidRequest:  &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &imp1}, {Id: &imp2}}},
		ImpIdUuid:   map[string]string{imp1: "uuid-1", imp2: "uuid-2"},
		SspDomain:   "adl_test",
		TrafficType: "ADULT",
		Format:      constants.POP,
	}

	response, err := server.GetBids_V2_5(context.Background(), req)
	if err != nil {
		t.Fatalf("all-ADV router call unexpectedly entered DSP path: %v", err)
	}
	if advClient.calls != 1 {
		t.Fatalf("ADV calls=%d want 1", advClient.calls)
	}
	if !response.GetRekl() {
		t.Fatal("all-ADV router response must preserve rekl=true")
	}
	if len(response.GetBidResponses()) != 0 {
		t.Fatalf("all-ADV router response unexpectedly contains DSP responses: %v", response.GetBidResponses())
	}
	if got := len(bidResponseImpIDs(response.GetReadyBidResponse())); got != 2 {
		t.Fatalf("ready ADV impressions=%d want 2", got)
	}
}

func TestRouterPartialADVKeepsADVAndFallsThroughForUnresolved(t *testing.T) {
	requestID := "req-partial"
	impADV, impFallback := "imp-adv", "imp-fallback"
	price := float32(1)
	advClient := &fakeADVClient{response: &advGrpc.DoAuctionResponse{
		BidResponse: &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
			{Impid: &impADV, Price: &price},
		}}}},
		WinnerUserIds:    map[string]string{impADV: "user-adv"},
		WinnerBasePrices: map[string]float64{impADV: 1.1},
	}}

	// No DSP endpoints are needed for this contract test. Reaching the fallback
	// completion path is enough to verify that Router does not return early on a
	// partial ADV result and that it forwards the ADV winner to BidEngine.
	server := &Server{
		advClient:                     advClient,
		dspEndpoints_adult_v_2_5:      map[string]string{},
		dspEndpoints_mainstream_v_2_5: map[string]string{},
	}
	req := &dspRouterGrpc.DspRouterRequest_V2_5{
		BidRequest:  &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &impADV}, {Id: &impFallback}}},
		ImpIdUuid:   map[string]string{impADV: "uuid-adv", impFallback: "uuid-fallback"},
		SspDomain:   "adl_test",
		TrafficType: "ADULT",
		Format:      constants.POP,
	}

	response, err := server.GetBids_V2_5(context.Background(), req)
	if err != nil {
		t.Fatalf("partial ADV router call failed: %v", err)
	}
	if response.GetRekl() {
		t.Fatal("partial ADV result must not be marked all-ADV")
	}
	if got := len(response.GetBidRequest().GetImp()); got != 2 {
		t.Fatalf("BidEngine must receive the original full request, got %d impressions", got)
	}
	if got := len(bidResponseImpIDs(response.GetReadyBidResponse())); got != 1 {
		t.Fatalf("forwarded ADV winners=%d want 1", got)
	}
	if got := response.GetWinnerUserIds()[impADV]; got != "user-adv" {
		t.Fatalf("forwarded ADV winner user=%q want user-adv", got)
	}
}
