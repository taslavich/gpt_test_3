package dspRouterWeb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	adv "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestPublisherFlagOnEverySuccessfulRouterPath(t *testing.T) {
	for _, publisher := range []bool{false, true} {
		for _, path := range []string{"DSP", "ADV", "mixed", "empty", "filtered"} {
			t.Run(fmt.Sprintf("%s/publisher=%t", path, publisher), func(t *testing.T) {
				imp, requestID := "1", "publisher-router"
				price := float32(2)
				response := &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &imp, Price: &price}}}}}
				dsp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if path == "DSP" || path == "mixed" {
						json.NewEncoder(w).Encode(response)
					} else {
						w.WriteHeader(204)
					}
				}))
				defer dsp.Close()
				advClient := &fakeParallelADVClient{}
				if path == "ADV" || path == "mixed" {
					advClient.response = &adv.DoAuctionResponse{BidResponse: response, WinnerUserIds: map[string]string{imp: "owner"}, WinnerBasePrices: map[string]float64{imp: 2}}
				}
				routes := &FormatRoutesV25{POP: FormatRouteV25{AdultEndpoints: config.MapStringToString{dsp.URL: "dsp.example"}}}
				var filters *filter.FiltersBox
				if path == "filtered" {
					filters = &filter.FiltersBox{Allowers: map[string]*filter.Filters{"ALL": {SiteId: filter.NewSiteIdBox(true, true, []string{"other-site"})}}}
				}
				server := NewServer(nil, nil, nil, routes, nil, time.Second, InitSspHttpClients(routes.EndpointSets()...), filters, nil, nil, nil, nil, nil, nil, nil, advClient)
				got, e := server.GetBids_V2_5(context.Background(), &pb.DspRouterRequest_V2_5{IsPublisher: publisher, BidRequest: &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &imp}}}, ImpIdUuid: map[string]string{imp: "uuid"}, SspDomain: "ssp.example", TrafficType: "ADULT", Format: "POP"})
				if e != nil || got == nil {
					t.Fatal(got, e)
				}
				if got.GetIsPublisher() != publisher {
					t.Fatal("flag lost", got)
				}
				if path == "filtered" {
					if got.GetCode() != 703 {
						t.Fatal(got)
					}
					return
				}
				wantDSP := path == "DSP" || path == "mixed"
				wantADV := path == "ADV" || path == "mixed"
				if (len(got.GetBidResponses()) > 0) != wantDSP || (got.GetReadyBidResponse() != nil) != wantADV {
					t.Fatal("test did not exercise expected result", got)
				}
			})
		}
	}
}
