package bidEngine

import (
	"context"
	"math"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestGetWinnerBidInternalAllADVDoesNotRequireDSPResponses(t *testing.T) {
	requestID := "req-all-adv"
	imp1, imp2 := "imp-1", "imp-2"
	uuid1, uuid2 := "uuid-1", "uuid-2"
	price1, price2 := float32(1.1), float32(2.2)
	adm1, adm2 := "https://adv.example/one", "https://adv.example/two"

	req := &bidEngineGrpc.BidEngineRequest_V2_5{
		BidRequest: &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &imp1}, {Id: &imp2}}},
		ReadyBidResponse: &ortb.BidResponse{
			Id: &requestID,
			Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
				{Impid: &imp1, Price: &price1, Adm: &adm1},
				{Impid: &imp2, Price: &price2, Adm: &adm2},
			}}},
		},
		ImpIdUuid:        map[string]string{imp1: uuid1, imp2: uuid2},
		WinnerUserIds:    map[string]string{imp1: "user-1", imp2: "user-2"},
		WinnerBasePrices: map[string]float64{imp1: 1.5, imp2: 2.5},
		SspDomain:        "ssp.example",
		Format:           constants.POP,
	}

	response, clickhouse, burlUUIDs, admUUIDs := GetWinnerBidInternal_V_2_5(
		context.Background(), req, 0.1, req.ImpIdUuid, nil, nil, false, "", "callbacks.example",
	)
	if got := len(response.GetSeatbid()[0].GetBid()); got != 2 {
		t.Fatalf("all ADV bids=%d want 2", got)
	}
	if got := clickhouse[uuid1].WinDspDomain; got == nil || *got != "adv" {
		t.Fatalf("imp1 winner domain=%v want adv", got)
	}
	if got := clickhouse[uuid2].WinDspDomain; got == nil || *got != "adv" {
		t.Fatalf("imp2 winner domain=%v want adv", got)
	}
	if len(burlUUIDs) != 0 || len(admUUIDs) != 0 {
		t.Fatalf("ADV-only path must not create DSP tracking UUIDs: burl=%v adm=%v", burlUUIDs, admUUIDs)
	}
}

func TestGetWinnerBidInternalDSPAuctionSelectionIsUnchanged(t *testing.T) {
	requestID := "req-dsp"
	impID := "imp-1"
	uuid := "uuid-1"
	low, high := float32(0.50), float32(0.80)
	admLow, admHigh := "https://dsp-a.example/adm", "https://dsp-b.example/adm"
	cidLow, cidHigh := "cid-a", "cid-b"

	req := &bidEngineGrpc.BidEngineRequest_V2_5{
		BidRequest: &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &impID}}},
		BidResponses: map[string]*ortb.BidResponse{
			"dsp-a": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &impID, Price: &low, Adm: &admLow, Cid: &cidLow}}}}},
			"dsp-b": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &impID, Price: &high, Adm: &admHigh, Cid: &cidHigh}}}}},
		},
		ImpIdUuid: map[string]string{impID: uuid},
		SspDomain: "ssp.example",
		Format:    constants.POP,
	}

	response, clickhouse, burlUUIDs, _ := GetWinnerBidInternal_V_2_5(
		context.Background(), req, 0.10, req.ImpIdUuid, nil, nil, false, "", "callbacks.example",
	)
	bids := response.GetSeatbid()[0].GetBid()
	if len(bids) != 1 {
		t.Fatalf("DSP winners=%d want 1", len(bids))
	}
	if got := bids[0].GetCid(); got != cidHigh {
		t.Fatalf("winner cid=%q want %q", got, cidHigh)
	}
	wantFinal := high - high*0.10
	if math.Abs(float64(bids[0].GetPrice()-wantFinal)) > 1e-6 {
		t.Fatalf("winner final price=%f want %f", bids[0].GetPrice(), wantFinal)
	}
	if got := clickhouse[uuid].WinDspDomain; got == nil || *got != "dsp-b" {
		t.Fatalf("winner domain=%v want dsp-b", got)
	}
	if len(burlUUIDs) != 1 || burlUUIDs[0] != uuid {
		t.Fatalf("DSP BURL UUIDs=%v want [%s]", burlUUIDs, uuid)
	}
}

func TestGetWinnerBidInternalMixedADVPriorityAndDSPFallback(t *testing.T) {
	requestID := "req-mixed"
	impADV, impDSP := "imp-adv", "imp-dsp"
	uuidADV, uuidDSP := "uuid-adv", "uuid-dsp"
	advPrice := float32(0.20)
	advADM := "https://adv.example/adm"
	dspPrice := float32(1.00)
	dspADM := "https://dsp.example/adm"
	maliciousDSPPrice := float32(100.00)
	maliciousDSPADM := "https://dsp.example/should-not-win"

	req := &bidEngineGrpc.BidEngineRequest_V2_5{
		BidRequest: &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &impADV}, {Id: &impDSP}}},
		ReadyBidResponse: &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
			{Impid: &impADV, Price: &advPrice, Adm: &advADM},
		}}}},
		BidResponses: map[string]*ortb.BidResponse{
			"dsp-main": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
				{Impid: &impDSP, Price: &dspPrice, Adm: &dspADM},
				{Impid: &impADV, Price: &maliciousDSPPrice, Adm: &maliciousDSPADM},
			}}}},
		},
		ImpIdUuid:        map[string]string{impADV: uuidADV, impDSP: uuidDSP},
		WinnerUserIds:    map[string]string{impADV: "user-adv"},
		WinnerBasePrices: map[string]float64{impADV: 0.30},
		SspDomain:        "ssp.example",
		Format:           constants.POP,
	}

	response, clickhouse, burlUUIDs, _ := GetWinnerBidInternal_V_2_5(
		context.Background(), req, 0.10, req.ImpIdUuid, nil, nil, false, "", "callbacks.example",
	)
	bids := response.GetSeatbid()[0].GetBid()
	if len(bids) != 2 {
		t.Fatalf("mixed winners=%d want 2", len(bids))
	}
	byImp := map[string]*ortb.Bid{}
	for _, bid := range bids {
		byImp[bid.GetImpid()] = bid
	}
	if got := clickhouse[uuidADV].WinDspDomain; got == nil || *got != "adv" {
		t.Fatalf("ADV impression winner domain=%v want adv", got)
	}
	if got := clickhouse[uuidDSP].WinDspDomain; got == nil || *got != "dsp-main" {
		t.Fatalf("DSP impression winner domain=%v want dsp-main", got)
	}
	if byImp[impADV] == nil || math.Abs(float64(byImp[impADV].GetPrice()-advPrice)) > 1e-6 {
		t.Fatalf("ADV impression was replaced by DSP: %+v", byImp[impADV])
	}
	if len(burlUUIDs) != 1 || burlUUIDs[0] != uuidDSP {
		t.Fatalf("only DSP fallback must create DSP BURL tracking UUID, got %v", burlUUIDs)
	}
}
