package auction

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

const publisherOwnerA = "11111111-1111-4111-8111-111111111111"
const publisherOwnerB = "22222222-2222-4222-8222-222222222222"

func publisherTestMap(t *testing.T) *PublisherPercentStore {
	t.Helper()
	s, err := NewPublisherPercentStore(filepath.Join(t.TempDir(), "publisher.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPublisherPercentPersistenceValidationAndRemoval(t *testing.T) {
	s := publisherTestMap(t)
	if _, found := s.Lookup(publisherOwnerA); found {
		t.Fatal("missing file invented an override")
	}
	group := publisherOwnerA + ", " + publisherOwnerB
	if err := s.Update(PublisherPercentMap{group: 0}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{publisherOwnerA, publisherOwnerB} {
		if p, found := s.Lookup(id); !found || p != 0 {
			t.Fatal("explicit zero/group expansion lost", id, p, found)
		}
	}
	if len(s.Saved()) != 1 || len(s.Memory()) != 2 {
		t.Fatal("saved/runtime grouping differs")
	}
	before, _ := os.ReadFile(s.filename)
	for _, bad := range []PublisherPercentMap{
		{publisherOwnerA: -0.01}, {publisherOwnerA: 1.01}, {publisherOwnerA: math.NaN()}, {publisherOwnerA: math.Inf(1)},
		{"ALL": .1}, {"": .1}, {publisherOwnerA + ",": .1}, {group: .1, publisherOwnerA: .2},
	} {
		if err := s.Update(bad); err == nil {
			t.Fatalf("invalid map accepted: %v", bad)
		}
		after, _ := os.ReadFile(s.filename)
		if string(after) != string(before) {
			t.Fatal("invalid PUT changed file")
		}
		if p, found := s.Lookup(publisherOwnerA); !found || p != 0 {
			t.Fatal("invalid PUT changed runtime")
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"` + publisherOwnerA + `":null}`, `{"` + publisherOwnerA + `":"0.2"}`} {
		var value PublisherPercentMap
		if json.Unmarshal([]byte(raw), &value) == nil {
			t.Fatal("invalid JSON accepted", raw)
		}
	}
	if err := s.Update(PublisherPercentMap{publisherOwnerA: 1}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewPublisherPercentStore(s.filename)
	if err != nil {
		t.Fatal(err)
	}
	if p, found := reloaded.Lookup(publisherOwnerA); !found || p != 1 {
		t.Fatal("restart lost 100%")
	}
	copy := reloaded.Memory()
	copy[publisherOwnerA] = .3
	if p, _ := reloaded.Lookup(publisherOwnerA); p != 1 {
		t.Fatal("snapshot is mutable")
	}
	if err := s.Update(PublisherPercentMap{}); err != nil {
		t.Fatal(err)
	}
	if _, found := s.Lookup(publisherOwnerA); found {
		t.Fatal("removed owner still overridden")
	}
	// Failed persistence must not publish the new generation.
	if err := os.Remove(s.filename); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.filename, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(PublisherPercentMap{publisherOwnerA: .4}); err == nil {
		t.Fatal("write failure ignored")
	}
	if _, found := s.Lookup(publisherOwnerA); found {
		t.Fatal("failed write changed runtime")
	}
}

func TestPublisherPercentRequestSnapshotPromoAndConcurrentUpdates(t *testing.T) {
	s := NewAuctionService(nil, nil, nil, nil, nil)
	m := publisherTestMap(t)
	s.ConfigurePublisherPercents(m)
	c := &Campaign{ID: "c", UserID: "advertiser"}
	for _, promo := range []float64{0, 1} {
		c.PromoSpendRemaining = promo
		for _, configured := range []float64{0, .1, .199, .20, .21, .22, .95, 1} {
			if err := m.Update(PublisherPercentMap{publisherOwnerA: configured}); err != nil {
				t.Fatal(err)
			}
			ctx := s.publisherPricingContext(context.Background(), publisherOwnerA)
			want := configured
			if promo > 0 && want < .2 {
				want = .2
			}
			if p, found := s.publisherMargin(ctx, c); !found || p != want {
				t.Fatalf("promo=%v configured=%v got=%v found=%t", promo, configured, p, found)
			}
			for _, absent := range []string{"", publisherOwnerB} {
				if _, found := s.publisherMargin(s.publisherPricingContext(context.Background(), absent), c); found {
					t.Fatal("owner isolation lost")
				}
			}
		}
	}
	if err := m.Update(PublisherPercentMap{publisherOwnerA: .1}); err != nil {
		t.Fatal(err)
	}
	old := s.publisherPricingContext(context.Background(), publisherOwnerA)
	if err := m.Update(PublisherPercentMap{publisherOwnerA: .7}); err != nil {
		t.Fatal(err)
	}
	c.PromoSpendRemaining = 0
	if p, _ := s.publisherMargin(old, c); p != .1 {
		t.Fatal("in-flight auction changed map generation")
	}
	// A live promo revision still takes effect; zero spend removes only the floor.
	if err := s.ApplyPromoSpendRemaining(c.UserID, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.publisherMargin(old, c); p != .2 {
		t.Fatal("live promo floor missing")
	}
	if err := s.ApplyPromoSpendRemaining(c.UserID, 0, 2, 1); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.publisherMargin(old, c); p != .1 {
		t.Fatal("exhausted promo still applies floor")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = m.Memory()
				_ = m.Saved()
				_, _ = m.Lookup(publisherOwnerA)
			}
		}()
	}
	for i := 0; i < 10; i++ {
		if err := m.Update(PublisherPercentMap{publisherOwnerA: float64(i) / 10}); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}

func publisherTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset: requires an isolated test Redis")
	}
	c := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	t.Cleanup(func() { c.Close() })
	if err := c.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	return c
}

func publisherTestService(t *testing.T, r *redis.Client) *AuctionService {
	t.Helper()
	q := &QualityStore{}
	q.value.Store(&qualitySnapshot{bySegment: QualityMaps{"usual": {"ssp.test": true}}})
	s := NewAuctionService(NewRuntimeStore(r, time.Minute, time.Hour), NewWinnerStore(r, time.Minute),
		newPercentStoreForPolicyTest(t, PercentMap{"ALL": .6, "ALL_RTB": .7}), q, &SiteIDQualityStore{})
	s.antiperekrutEnabled = false
	s.ConfigurePublisherPercents(publisherTestMap(t))
	s.ConfigureSimplePercenter(percenter.NewSimpleStateStore(r, percenter.SimplePolicy{}), percenter.SimplePolicy{})
	s.ConfigureComplexPercenter(percenter.NewComplexStateStore(r, percenter.ComplexPolicy{}), percenter.ComplexPolicy{})
	return s
}

func TestPublisherPercentAllPricingModesAndRTBCampaigns(t *testing.T) {
	r := publisherTestRedis(t)
	s := publisherTestService(t, r)
	now := time.Now().UTC()
	impID, requestID, destination := "imp", "request", "https://advertiser.example"
	req := &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &impID}}}
	for _, model := range []int{TypeModelSimple, TypeModelComplex, TypeModelMapOnly} {
		for _, remote := range []bool{false, true} {
			for _, promo := range []float64{0, 1} {
				for _, configured := range []float64{0, .1, .2, .21, .95, 1} {
					t.Run(fmt.Sprintf("model=%d/RTB=%t/promo=%v/percent=%v", model, remote, promo, configured), func(t *testing.T) {
						campaign := &Campaign{ID: uuid.NewString(), UserID: uuid.NewString(), TypeModel: model, RTB: remote, PromoSpendRemaining: promo,
							BasePrice: 10, GoalTotalDollars: 1000, PricingModel: PricingModelCPM, Format: "POP", TrafficType: TrafficMainstream,
							QualitySegment: "usual", Status: CampaignStatusActive, StartTS: now.Add(-time.Hour), EndTS: now.Add(time.Hour),
							Creatives: []*Creative{{ID: uuid.NewString(), ADMURL: destination}}}
						if err := s.publisherPercents.Update(PublisherPercentMap{publisherOwnerA: configured}); err != nil {
							t.Fatal(err)
						}
						ctx := s.publisherPricingContext(context.Background(), publisherOwnerA)
						segment := BuildPercenterSegmentHash(req, "ssp.test", campaign.ID)
						t.Cleanup(func() {
							r.Del(context.Background(), percenter.SimpleStateKey(segment), percenter.ComplexStateKey(segment))
						})
						var cand candidate
						var ok bool
						var err error
						if remote {
							price := float32(10)
							cand, ok, err = s.evaluateRTBCandidate(ctx, campaign, &ortb.Bid{Price: &price}, req.Imp[0], now, "POP", "exact", segment, 1000)
						} else {
							cand, ok, _, err = s.evaluateCampaign(ctx, campaign, req, req.Imp[0], now, "POP", TrafficMainstream, "ssp.test", "", "exact", "uuid", nil, false, false, nil, false, func(string, ...any) {})
						}
						if err != nil {
							t.Fatal(err)
						}
						if configured == 1 {
							if ok {
								t.Fatal("100% must reject zero bid")
							}
							return
						}
						if !ok {
							t.Fatal("Publisher candidate rejected")
						}
						want := configured
						if promo > 0 && want < .2 {
							want = .2
						}
						if math.Abs(1-cand.effectivePrice/cand.basePrice-want) > 1e-12 {
							t.Fatalf("wrong retained margin: base=%v SSP=%v want=%v", cand.basePrice, cand.effectivePrice, want)
						}
						if cand.originalBid != 10 || cand.pointVersion != 0 {
							t.Fatal("original bid/probe identity changed", cand)
						}
						if !remote && model == TypeModelComplex {
							if math.Abs(cand.effectivePrice-4) > 1e-10 {
								t.Fatal("Complex SSP baseline/search was replaced", cand.effectivePrice)
							}
							if math.Abs(cand.chargePrice-cand.basePrice/1000) > 1e-12 {
								t.Fatal("Complex billing price inconsistent")
							}
						} else if math.Abs(cand.basePrice-10) > 1e-12 || math.Abs(cand.chargePrice-.01) > 1e-12 {
							t.Fatal("advertiser charge changed")
						}
					})
				}
			}
		}
	}
}

func TestPublisherPercentComplexKeepsSearchedSSPPriceAndOtherPublishers(t *testing.T) {
	r := publisherTestRedis(t)
	s := publisherTestService(t, r)
	ctx := context.Background()
	now := time.Now().UTC()
	imp, id, adm := "i", "request", "https://advertiser.example"
	req := &ortb.BidRequest{Id: &id, Imp: []*ortb.Imp{{Id: &imp}}}
	c := &Campaign{ID: uuid.NewString(), UserID: uuid.NewString(), TypeModel: TypeModelComplex, BasePrice: 10, GoalTotalDollars: 1000, PricingModel: PricingModelCPM, Format: "POP", TrafficType: TrafficMainstream, QualitySegment: "usual", Status: CampaignStatusActive, StartTS: now.Add(-time.Hour), EndTS: now.Add(time.Hour), Creatives: []*Creative{{ID: uuid.NewString(), ADMURL: adm}}}
	segment := BuildPercenterSegmentHash(req, "ssp.test", c.ID)
	state := percenter.NewComplexState(segment, c.ID, 10, .6, percenter.ComplexPolicy{}, now, "ALL")
	state.SSPBid = 2
	state.LastGoodSSPBid = 2
	state.Margin = .75
	state.LastGoodMargin = .75
	state.AdvertiserPrice = 8
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	key := percenter.ComplexStateKey(segment)
	if err = r.Set(ctx, key, raw, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	defer r.Del(ctx, key)
	if err = s.publisherPercents.Update(PublisherPercentMap{publisherOwnerA: .1}); err != nil {
		t.Fatal(err)
	}
	eval := func(ctx context.Context) candidate {
		t.Helper()
		cand, ok, reason, err := s.evaluateCampaign(ctx, c, req, req.Imp[0], now, "POP", TrafficMainstream, "ssp.test", "", "exact", "uuid", nil, false, false, nil, false, func(string, ...any) {})
		if err != nil || !ok {
			t.Fatal(reason, err)
		}
		return cand
	}
	manual := eval(s.publisherPricingContext(ctx, publisherOwnerA))
	if manual.effectivePrice != 2 || math.Abs(manual.basePrice-2/.9) > 1e-12 || manual.pointVersion != 0 {
		t.Fatal("manual margin changed SSP search", manual)
	}
	ordinary := eval(s.publisherPricingContext(ctx, publisherOwnerB))
	if ordinary.effectivePrice != 2 || ordinary.basePrice != 8 || ordinary.pointVersion != state.PointVersion {
		t.Fatal("other Publisher policy changed", ordinary)
	}
	// Removing the override resumes exactly the existing Complex point.
	if err = s.publisherPercents.Update(PublisherPercentMap{}); err != nil {
		t.Fatal(err)
	}
	resumed := eval(s.publisherPricingContext(ctx, publisherOwnerA))
	if resumed.basePrice != ordinary.basePrice || resumed.effectivePrice != ordinary.effectivePrice || resumed.pointVersion != ordinary.pointVersion {
		t.Fatal("removal did not resume normal pricing")
	}
	// The retained SSP price with a high manual margin can increase the charge;
	// the existing final Complex balance guard must still reject overspending.
	if err = s.publisherPercents.Update(PublisherPercentMap{publisherOwnerA: .95}); err != nil {
		t.Fatal(err)
	}
	c.GoalTotalDollars = .03 // SSP=2, gross=40, CPM charge=.04 > remaining .03.
	_, ok, _, err := s.evaluateCampaign(s.publisherPricingContext(ctx, publisherOwnerA), c, req, req.Imp[0], now,
		"POP", TrafficMainstream, "ssp.test", "", "exact", "uuid", nil, false, false, nil, false, func(string, ...any) {})
	if err != nil || ok {
		t.Fatal("manual Complex pricing bypassed balance guard", ok, err)
	}
}

func TestPublisherPercentRealAuctionWinnerBillingAndLiveRemoval(t *testing.T) {
	r := publisherTestRedis(t)
	s := publisherTestService(t, r)
	now := time.Now().UTC()
	imp, id, adm := "i", "request", "https://advertiser.example"
	req := &ortb.BidRequest{Id: &id, Imp: []*ortb.Imp{{Id: &imp}}}
	c := &Campaign{ID: uuid.NewString(), UserID: uuid.NewString(), TypeModel: TypeModelMapOnly, BasePrice: 10,
		GoalTotalDollars: 1000, PricingModel: PricingModelCPM, Format: "POP", TrafficType: TrafficMainstream,
		QualitySegment: "usual", Status: CampaignStatusActive, StartTS: now.Add(-time.Hour), EndTS: now.Add(time.Hour),
		Creatives: []*Creative{{ID: uuid.NewString(), ADMURL: adm}}}
	if err := s.PublishSnapshot(&Snapshot{Campaigns: []*Campaign{c}, UserGoals: map[string]float64{c.UserID: 1000}}); err != nil {
		t.Fatal(err)
	}
	if err := s.publisherPercents.Update(PublisherPercentMap{publisherOwnerA: .1}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		owner string
		want  float32
	}{{publisherOwnerA, 9}, {publisherOwnerB, 4}, {"", 4}} {
		winnerID := uuid.NewString()
		defer r.Del(context.Background(), winnerID)
		out, err := s.Auction(context.Background(), req, now, AuctionRequestOptions{PublisherUserID: test.owner, Format: "POP", TrafficType: TrafficMainstream, SSPDomain: "ssp.test", ImpIDUUID: map[string]string{imp: winnerID}})
		if err != nil || out == nil || out.BidResponse == nil {
			t.Fatal("real auction failed", err, out)
		}
		bid := out.BidResponse.GetSeatbid()[0].GetBid()[0]
		if bid.GetPrice() != test.want || out.WinnerBasePrices[imp] != 10 || out.WinnerUserIDs[imp] != c.UserID {
			t.Fatal("winner pricing/identity changed", out)
		}
		// The existing winner record still bills the advertiser, never the Publisher.
		stored := r.HGetAll(context.Background(), winnerID).Val()
		if stored["price"] != "0.01" || stored["user_id"] != c.UserID || stored["campaign_id"] != c.ID {
			t.Fatal("billing winner changed", stored)
		}
	}
	if err := s.publisherPercents.Update(PublisherPercentMap{publisherOwnerA: 1}); err != nil {
		t.Fatal(err)
	}
	winnerID := uuid.NewString()
	defer r.Del(context.Background(), winnerID)
	out, err := s.Auction(context.Background(), req, now, AuctionRequestOptions{PublisherUserID: publisherOwnerA, Format: "POP", TrafficType: TrafficMainstream, SSPDomain: "ssp.test", ImpIDUUID: map[string]string{imp: winnerID}})
	if err != nil || out.BidResponse != nil || r.Exists(context.Background(), winnerID).Val() != 0 {
		t.Fatal("100% created a winner/billing record", out, err)
	}
}
