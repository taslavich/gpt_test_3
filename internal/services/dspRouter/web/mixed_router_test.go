package dspRouterWeb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
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

func TestRouterRTBReceivesAllImpressionsAndIsTaggedSeparately(t *testing.T) {
	requestID := "req-rtb-full-request"
	imp1, imp2 := "imp-1", "imp-2"
	advPrice := float32(0.70)
	advClient := &fakeADVClient{response: &advGrpc.DoAuctionResponse{
		BidResponse: &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
			{Impid: &imp1, Price: &advPrice},
			{Impid: &imp2, Price: &advPrice},
		}}}},
		WinnerUserIds:    map[string]string{imp1: "user-1", imp2: "user-2"},
		WinnerBasePrices: map[string]float64{imp1: 1.0, imp2: 1.0},
	}}

	var receivedImpIDs []string
	rtbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var bidRequest ortb.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&bidRequest); err != nil {
			t.Errorf("decode RTB request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, imp := range bidRequest.GetImp() {
			receivedImpIDs = append(receivedImpIDs, imp.GetId())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"rtb-response","seatbid":[{"bid":[{"id":"rtb-bid","impid":"imp-1","price":1.20,"adm":"https://rtb.example/adm"}]}]}`))
	}))
	defer rtbServer.Close()

	rtbDomain := "adl_rtb_partner.example"
	routes := &FormatRoutesV25{
		POP: FormatRouteV25{
			AdultRTBEndpoints: config.MapStringToString{rtbServer.URL: rtbDomain},
		},
	}
	clients := InitSspHttpClients(routes.EndpointSets()...)
	server := NewServer(
		nil, nil, nil, routes, nil, 300*time.Millisecond, clients,
		nil, nil, nil, nil, nil, nil, nil, nil, advClient,
	)

	response, err := server.GetBids_V2_5(context.Background(), &dspRouterGrpc.DspRouterRequest_V2_5{
		BidRequest:  &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &imp1}, {Id: &imp2}}},
		ImpIdUuid:   map[string]string{imp1: "uuid-1", imp2: "uuid-2"},
		SspDomain:   "adl_ssp.example",
		TrafficType: "ADULT",
		Format:      constants.POP,
	})
	if err != nil {
		t.Fatalf("router call failed: %v", err)
	}
	if len(receivedImpIDs) != 2 || receivedImpIDs[0] != imp1 || receivedImpIDs[1] != imp2 {
		t.Fatalf("RTB received impressions=%v want [%s %s]", receivedImpIDs, imp1, imp2)
	}
	key := constants.RTBResponseDomainPrefix + rtbDomain
	if response.GetBidResponses()[key] == nil {
		t.Fatalf("RTB response key %q missing from BidResponses: %v", key, response.GetBidResponses())
	}
	if response.GetRekl() {
		t.Fatal("Router cannot mark all-ADV before BidEngine auctions ADV against RTB")
	}
}

func TestDemandResponseCodesIncludeRTBForAllImpressionsAndDSPOnlyForFallback(t *testing.T) {
	dspImps := map[string]string{"imp-fallback": "uuid-fallback"}
	dspCodes := map[string]string{"adl_dsp.example": "204"}
	rtbCodes := map[string]string{"adl_rtb.example": "200"}

	advResolvedStats := demandResponseCodesForImp("imp-adv", dspImps, dspCodes, rtbCodes)
	if got := advResolvedStats["adl_rtb.example"]; got != "200" {
		t.Fatalf("ADV-resolved imp RTB code=%q want 200", got)
	}
	if _, exists := advResolvedStats["adl_dsp.example"]; exists {
		t.Fatal("ADV-resolved imp must not contain DSP status for a DSP request that was never sent")
	}

	fallbackStats := demandResponseCodesForImp("imp-fallback", dspImps, dspCodes, rtbCodes)
	if got := fallbackStats["adl_rtb.example"]; got != "200" {
		t.Fatalf("fallback imp RTB code=%q want 200", got)
	}
	if got := fallbackStats["adl_dsp.example"]; got != "204" {
		t.Fatalf("fallback imp DSP code=%q want 204", got)
	}
}
