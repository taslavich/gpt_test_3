package auction

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func ptr[T any](v T) *T { return &v }

func testCampaign(id string) *Campaign {
	return &Campaign{ID: id, UserID: "u1", Status: CampaignStatusActive, PricingModel: PricingModelCPM, Format: "ban", QualitySegment: "usual", TrafficType: "MAINSTREAM", BasePrice: 10, GoalTotalDollars: 100, StartTS: time.Now().Add(-time.Hour), EndTS: time.Now().Add(time.Hour), Creatives: map[string]*Creative{"cr1": {ID: "cr1", CampaignID: id, ADMURL: "https://adm.example/path?x=1", W: 300, H: 250, TrackersMacros: map[string]bool{"campaign_id": true, "missing": true}}}}
}

func TestPricingDeductionOnce(t *testing.T) {
	if got := EffectivePrice(10, PricingModelCPM, "ban", 0.2); got != 0.008 {
		t.Fatalf("effective=%v", got)
	}
	if got := ChargePrice(10, PricingModelCPC, "pop"); got != 0.01 {
		t.Fatalf("cpc pop=%v", got)
	}
	if got := ChargePrice(10, PricingModelCPC, "ipp"); got != 10 {
		t.Fatalf("cpc ipp=%v", got)
	}
}

func TestSnapshotDeepCopyAndConcurrentReads(t *testing.T) {
	c := testCampaign("c1")
	s := BuildSnapshot(map[string]*Campaign{"c1": c}, map[string]float64{"u1": 100})
	c.Creatives["cr1"].ADMURL = "mutated"
	if s.Campaigns["c1"].Creatives["cr1"].ADMURL == "mutated" {
		t.Fatal("snapshot creative was mutated")
	}
	store := newSnapshotStore()
	store.Store(s)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = store.Load().Campaigns["c1"].Creatives["cr1"].ADMURL }()
	}
	wg.Wait()
}

func TestQualityAndPercentStores(t *testing.T) {
	q := NewQualityStore()
	if err := q.Replace(map[string][]string{"usual": {"ssp.example"}}); err != nil {
		t.Fatal(err)
	}
	if q.Segment("SSP.EXAMPLE") != "usual" {
		t.Fatal("quality lookup failed")
	}
	p := NewPercentStore("", "")
	m := PercentMap{"ssp.example": {"US": {"u1": {Percent: .5}}}}
	if err := p.Update("MAINSTREAM", m); err != nil {
		t.Fatal(err)
	}
	if got := p.Lookup("MAINSTREAM", "ssp.example", "US", "u1"); got != .5 {
		t.Fatalf("percent=%v", got)
	}
}

func TestRuntimeNilRedisAndPacing(t *testing.T) {
	store := NewRuntimeStore(nil, time.Minute)
	if v, err := store.UserSpent(context.Background(), "u1"); err != nil || v != 0 {
		t.Fatalf("nil redis spent %v %v", v, err)
	}
	c := testCampaign("c1")
	c.EvennessBySlotMode = true
	ok, err := PacingEligible(context.Background(), store, c, time.Now(), 0)
	if err != nil || !ok {
		t.Fatalf("pacing ok=%v err=%v", ok, err)
	}
}

func TestAuctionMultiImpBannerNilAndMacros(t *testing.T) {
	q := NewQualityStore()
	_ = q.Replace(map[string][]string{"usual": {"ssp.example"}})
	p := NewPercentStore("", "")
	_ = p.Update("MAINSTREAM", PercentMap{"ssp.example": {"US": {"u1": {Percent: .9}}}})
	s := NewAuctionService(nil)
	s.SetQualityStore(q)
	s.SetPercentStore(p)
	s.ReplaceSnapshot(map[string]*Campaign{"c1": testCampaign("c1")}, map[string]float64{"u1": 100})
	req := &ortb_V2_5.BidRequest{Id: ptr("req1"), Device: &ortb_V2_5.Device{Geo: &ortb_V2_5.Geo{Country: ptr("US")}}, Imp: []*ortb_V2_5.Imp{{Id: ptr("i1"), Banner: &ortb_V2_5.Banner{W: ptr(int32(300)), H: ptr(int32(250))}}, {Id: ptr("i2"), Banner: &ortb_V2_5.Banner{W: ptr(int32(728)), H: ptr(int32(90))}}}}
	candidates := s.SelectAuctionCandidates(context.Background(), req, time.Now(), AuctionRequestOptions{TrafficType: "MAINSTREAM", SSPDomain: "ssp.example"})
	if len(candidates) != 1 {
		t.Fatalf("candidates=%d", len(candidates))
	}
	u, err := url.Parse(candidates[0].ADM)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("campaign_id") != "c1" || u.Query().Get("missing") != "unknown" {
		t.Fatalf("bad macros: %s", candidates[0].ADM)
	}
}

func TestPercentValidationRejectsOutOfRange(t *testing.T) {
	p := NewPercentStore("", "")
	if err := p.Update("ADULT", PercentMap{"ssp": {"US": {"u": &types.PercentAndBidfloor{Percent: 2}}}}); err == nil {
		t.Fatal("expected validation error")
	}
}
