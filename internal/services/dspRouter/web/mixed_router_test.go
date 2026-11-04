package dspRouterWeb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/grpc"
)

type fakeParallelADVClient struct {
	response   *advGrpc.DoAuctionResponse
	waitForDSP <-chan struct{}
	mu         sync.Mutex
	calls      int
	impCount   int
}

func (f *fakeParallelADVClient) DoAuction(ctx context.Context, req *advGrpc.DoAuctionRequest, _ ...grpc.CallOption) (*advGrpc.DoAuctionResponse, error) {
	f.mu.Lock()
	f.calls++
	if req != nil && req.GetBidRequest() != nil {
		f.impCount = len(req.GetBidRequest().GetImp())
	}
	f.mu.Unlock()

	if f.waitForDSP != nil {
		select {
		case <-f.waitForDSP:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.response, nil
}

func (f *fakeParallelADVClient) snapshot() (calls, impCount int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.impCount
}

func TestRouterStartsADVAndDSPInParallelAndDSPGetsFullRequest(t *testing.T) {
	requestID := "req-parallel"
	imp1, imp2 := "imp-1", "imp-2"
	advPrice := float32(0.70)

	dspSeen := make(chan struct{})
	var closeDSPSeen sync.Once
	dspServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var got ortb.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode DSP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(got.GetImp()) != 2 || got.GetImp()[0].GetId() != imp1 || got.GetImp()[1].GetId() != imp2 {
			t.Errorf("DSP impressions=%v want full request [%s %s]", got.GetImp(), imp1, imp2)
		}
		closeDSPSeen.Do(func() { close(dspSeen) })
		w.WriteHeader(http.StatusNoContent)
	}))
	defer dspServer.Close()

	advClient := &fakeParallelADVClient{
		waitForDSP: dspSeen,
		response: &advGrpc.DoAuctionResponse{
			BidResponse: &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
				{Impid: &imp1, Price: &advPrice},
				{Impid: &imp2, Price: &advPrice},
			}}}},
			WinnerUserIds:    map[string]string{imp1: "user-1", imp2: "user-2"},
			WinnerBasePrices: map[string]float64{imp1: 1.0, imp2: 1.0},
		},
	}

	dspDomain := "adl_parallel.example"
	routes := &FormatRoutesV25{POP: FormatRouteV25{
		AdultEndpoints: config.MapStringToString{dspServer.URL: dspDomain},
	}}
	clients := InitSspHttpClients(routes.EndpointSets()...)
	server := NewServer(
		nil, nil, nil, routes, nil, 500*time.Millisecond, clients,
		nil, nil, nil, nil, nil, nil,
		config.MapStringToDuration{"ssp.example": 500 * time.Millisecond}, nil, advClient,
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
	calls, advImpCount := advClient.snapshot()
	if calls != 1 || advImpCount != 2 {
		t.Fatalf("ADV calls/impressions=%d/%d want 1/2", calls, advImpCount)
	}
	if got := len(response.GetBidRequest().GetImp()); got != 2 {
		t.Fatalf("BidEngine request impressions=%d want 2", got)
	}
	if got := len(bidResponseImpIDs(response.GetReadyBidResponse())); got != 2 {
		t.Fatalf("ready ADV impressions=%d want 2", got)
	}
}

func TestRouterAddsCIDForSSPDomainOnDSPAndADV(t *testing.T) {
	const domain = "ssp.example"
	const dsp = "adl_dsp.example"
	imp1, imp2 := "imp-1", "imp-2"
	adm, cid, price := "https://example.test/ad", "real-campaign", float32(0.7)
	path := filepath.Join(t.TempDir(), "spp_rules_v25.json")
	if err := os.WriteFile(path, []byte(`{"spps":{"`+domain+`":{"add":[{"field":"bid.cid","pool_size":50}]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	rm := filter.NewRuleManager()
	loader := filter.NewFileRuleLoader(rm, "", path)
	if err := loader.LoadSPPRules(); err != nil {
		t.Fatal(err)
	}
	dspServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
			{Impid: &imp1, Adm: &adm, Price: &price}, {Impid: &imp2, Adm: &adm, Price: &price, Cid: &cid},
		}}}})
	}))
	defer dspServer.Close()
	routes := &FormatRoutesV25{POP: FormatRouteV25{AdultEndpoints: config.MapStringToString{dspServer.URL: dsp}}}
	for _, test := range []struct {
		name, domain string
		want         bool
		rejectADV    bool
	}{
		{"Adult domain", "adl_" + domain, true, false},
		{"Mainstream prefix", "mc_" + domain, true, false},
		{"Bare domain", domain, true, false},
		{"Other domain", "adl_other.example", false, false},
		{"Rejected ADV keeps DSP available", "adl_" + domain, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			advADM := &adm
			if test.rejectADV {
				advADM = nil
			}
			advClient := &fakeParallelADVClient{response: &advGrpc.DoAuctionResponse{
				BidResponse: &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
					{Impid: &imp1, Adm: advADM, Price: &price}, {Impid: &imp2, Adm: &adm, Price: &price, Cid: &cid},
				}}}},
				WinnerUserIds:    map[string]string{imp1: "u", imp2: "u"},
				WinnerBasePrices: map[string]float64{imp1: 1, imp2: 1},
			}}
			server := NewServer(rm, loader, filter.NewOptimizedFilterProcessor(rm), routes, nil,
				time.Second, InitSspHttpClients(routes.EndpointSets()...), nil, nil, nil, nil, nil, nil,
				config.MapStringToDuration{"ssp.example": time.Second}, nil, advClient)

			got, err := server.GetBids_V2_5(context.Background(), &dspRouterGrpc.DspRouterRequest_V2_5{
				BidRequest: &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp1}, {Id: &imp2}}},
				SspDomain:  test.domain, TrafficType: "ADULT", Format: constants.POP,
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range []struct {
				name string
				resp *ortb.BidResponse
			}{{"ADV", got.GetReadyBidResponse()}, {"DSP", got.GetBidResponses()[dsp]}} {
				if item.name == "ADV" && test.rejectADV {
					if item.resp != nil || len(got.GetWinnerUserIds()) != 0 || len(got.GetWinnerBasePrices()) != 0 {
						t.Fatal("SSP-rejected ADV response or winner data leaked")
					}
					bid := advClient.response.GetBidResponse().GetSeatbid()[0].GetBid()[0]
					if bid.Cid != nil {
						t.Fatal("SSP-rejected bid was modified")
					}
					continue
				}
				if len(item.resp.GetSeatbid()) != 1 {
					t.Fatalf("%s response missing: %v", item.name, item.resp)
				}
				bids := item.resp.GetSeatbid()[0].GetBid()
				if len(bids) != 2 || bids[1].GetCid() != cid {
					t.Fatalf("%s real CID changed: %v", item.name, bids)
				}
				if test.want {
					parsed, err := uuid.Parse(bids[0].GetCid())
					if err != nil || parsed.Version() != 5 {
						t.Fatalf("%s missing pooled UUID: %v", item.name, bids[0])
					}
				} else if bids[0].Cid != nil {
					t.Fatalf("%s: unrelated domain got a synthetic CID", item.name)
				}
			}
		})
	}
}
