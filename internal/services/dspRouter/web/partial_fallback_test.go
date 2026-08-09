package dspRouterWeb

import (
	"testing"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestBuildDSPFallbackBidRequestKeepsOnlyUnresolvedImpressions(t *testing.T) {
	requestID := "request-1"
	imp1, imp2, imp3 := "imp-1", "imp-2", "imp-3"
	source := &ortb.BidRequest{
		Id:  &requestID,
		Imp: []*ortb.Imp{{Id: &imp1}, {Id: &imp2}, {Id: &imp3}},
	}
	price := float32(1)
	readyADV := &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{
		{Impid: &imp1, Price: &price},
		{Impid: &imp3, Price: &price},
	}}}}
	uuids := map[string]string{imp1: "uuid-1", imp2: "uuid-2", imp3: "uuid-3"}

	fallback, fallbackUUIDs, err := buildDSPFallbackBidRequest(source, uuids, readyADV)
	if err != nil {
		t.Fatalf("build fallback: %v", err)
	}
	if fallback == source {
		t.Fatal("fallback request must be a deep clone")
	}
	if len(source.GetImp()) != 3 {
		t.Fatalf("source request was mutated: %d impressions", len(source.GetImp()))
	}
	if len(fallback.GetImp()) != 1 || fallback.GetImp()[0].GetId() != imp2 {
		t.Fatalf("fallback impressions=%v want only %s", fallback.GetImp(), imp2)
	}
	if len(fallbackUUIDs) != 1 || fallbackUUIDs[imp2] != "uuid-2" {
		t.Fatalf("fallback UUIDs=%v want only imp-2", fallbackUUIDs)
	}
}

func TestBuildDSPFallbackBidRequestAllADVProducesNoDSPImpressions(t *testing.T) {
	imp1, imp2 := "imp-1", "imp-2"
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp1}, {Id: &imp2}}}
	readyADV := &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &imp1}, {Impid: &imp2}}}}}

	fallback, fallbackUUIDs, err := buildDSPFallbackBidRequest(source, map[string]string{imp1: "u1", imp2: "u2"}, readyADV)
	if err != nil {
		t.Fatalf("build fallback: %v", err)
	}
	if len(fallback.GetImp()) != 0 || len(fallbackUUIDs) != 0 {
		t.Fatalf("all-ADV request must not go to DSP: imp=%d uuids=%v", len(fallback.GetImp()), fallbackUUIDs)
	}
}

func TestBuildDSPFallbackBidRequestEmptyADVFallsBackAll(t *testing.T) {
	imp1, imp2 := "imp-1", "imp-2"
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp1}, {Id: &imp2}}}
	readyADV := &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{}}}}

	fallback, fallbackUUIDs, err := buildDSPFallbackBidRequest(source, map[string]string{imp1: "u1", imp2: "u2"}, readyADV)
	if err != nil {
		t.Fatalf("build fallback: %v", err)
	}
	if len(fallback.GetImp()) != 2 || len(fallbackUUIDs) != 2 {
		t.Fatalf("empty ADV response must fall back all impressions: imp=%d uuids=%v", len(fallback.GetImp()), fallbackUUIDs)
	}
}
