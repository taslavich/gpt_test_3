package auction

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

func TestSimpleLocalFallbackRejectsChangedOrdinaryOriginalBid(t *testing.T) {
	policy := percenter.SimplePolicy{}.Normalize()
	campaign := &Campaign{ID: "ordinary-simple", TypeModel: TypeModelSimple}
	entry := simplePricingCacheEntry{
		CampaignID: campaign.ID, TypeModel: percenter.TypeModelSimple, RTB: false,
		ReferenceOriginalBid: 10, EffectiveMin: .20, MaxMargin: policy.MaxMargin,
		Pricing: percenter.SimplePricing{SegmentHash: "segment-a", Margin: .30, SSPBid: 7, PointVersion: 9},
	}

	if _, ok := simpleCachedPricing(entry, campaign, 12, .20, false, policy); ok {
		t.Fatal("ordinary Simple cache built for original bid 10 must not survive original bid change to 12")
	}
	if _, ok := simpleCachedPricing(entry, campaign, 8, .20, false, policy); ok {
		t.Fatal("ordinary Simple cache built for original bid 10 must not survive original bid change to 8")
	}
}

func TestComplexLocalFallbackRejectsChangedOriginalBid(t *testing.T) {
	policy := percenter.ComplexPolicy{}.Normalize()
	campaign := &Campaign{ID: "ordinary-complex", TypeModel: TypeModelComplex}
	entry := complexPricingCacheEntry{
		CampaignID: campaign.ID, TypeModel: percenter.TypeModelComplex,
		ReferenceOriginalBid: 10, EffectiveMin: .20, MaxMargin: policy.MaxMargin,
		Pricing: percenter.ComplexPricing{
			SegmentHash: "segment-c", AdvertiserPrice: 6.25, Margin: .20, SSPBid: 5, PointVersion: 17,
		},
	}

	for _, changedOriginal := range []float64{8, 12} {
		if _, ok := complexCachedPricing(entry, campaign, changedOriginal, .20, policy); ok {
			t.Fatalf("Complex cache built for original bid 10 must not survive original bid change to %v", changedOriginal)
		}
	}
}

func TestRTBSimpleLocalFallbackKeepsMarginAcrossRawPriceChange(t *testing.T) {
	policy := percenter.SimplePolicy{}.Normalize()
	campaign := &Campaign{ID: "rtb-simple", TypeModel: TypeModelSimple, RTB: true}
	entry := simplePricingCacheEntry{
		CampaignID: campaign.ID, TypeModel: percenter.TypeModelSimple, RTB: true,
		ReferenceOriginalBid: 10, EffectiveMin: .30, MaxMargin: policy.MaxMargin,
		Pricing: percenter.SimplePricing{SegmentHash: "segment-r", Margin: .40, SSPBid: 6, PointVersion: 23},
	}

	pricing, ok := simpleCachedPricing(entry, campaign, 20, .30, true, policy)
	if !ok {
		t.Fatal("RTB Simple rawPrice change must not invalidate compatible cached margin state")
	}
	if pricing.PointVersion != 23 || math.Abs(pricing.Margin-.40) > 1e-12 {
		t.Fatalf("cached point identity/margin changed: %+v", pricing)
	}
	if math.Abs(pricing.SSPBid-12) > 1e-12 {
		t.Fatalf("RTB SSP bid must be recalculated from current rawPrice: got %v want 12", pricing.SSPBid)
	}
}

func TestLocalFallbackRejectsChangedEffectiveMinimum(t *testing.T) {
	simplePolicy := percenter.SimplePolicy{}.Normalize()
	simpleCampaign := &Campaign{ID: "simple-floor", TypeModel: TypeModelSimple}
	simpleEntry := simplePricingCacheEntry{
		CampaignID: simpleCampaign.ID, TypeModel: percenter.TypeModelSimple,
		ReferenceOriginalBid: 10, EffectiveMin: .20, MaxMargin: simplePolicy.MaxMargin,
		Pricing: percenter.SimplePricing{SegmentHash: "s", Margin: .40, SSPBid: 6, PointVersion: 3},
	}
	if _, ok := simpleCachedPricing(simpleEntry, simpleCampaign, 10, .30, false, simplePolicy); ok {
		t.Fatal("changed Simple effective_min must invalidate cached optimizer point even when cached margin is above the new floor")
	}

	complexPolicy := percenter.ComplexPolicy{}.Normalize()
	complexCampaign := &Campaign{ID: "complex-floor", TypeModel: TypeModelComplex}
	complexEntry := complexPricingCacheEntry{
		CampaignID: complexCampaign.ID, TypeModel: percenter.TypeModelComplex,
		ReferenceOriginalBid: 10, EffectiveMin: .20, MaxMargin: complexPolicy.MaxMargin,
		Pricing: percenter.ComplexPricing{SegmentHash: "c", AdvertiserPrice: 6.25, Margin: .20, SSPBid: 5, PointVersion: 4},
	}
	if _, ok := complexCachedPricing(complexEntry, complexCampaign, 10, .30, complexPolicy); ok {
		t.Fatal("changed Complex effective_min must invalidate cached optimizer point")
	}
}

func unavailablePercenterRedis(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:1", DialTimeout: 10 * time.Millisecond, ReadTimeout: 10 * time.Millisecond,
		WriteTimeout: 10 * time.Millisecond, MaxRetries: -1,
	})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestOrdinarySimpleRedisOutageRejectsCacheAfterOriginalBidChange(t *testing.T) {
	policy := percenter.SimplePolicy{}.Normalize()
	service := &AuctionService{}
	service.ConfigureSimplePercenter(percenter.NewSimpleStateStore(unavailablePercenterRedis(t), policy), policy)
	campaign := &Campaign{ID: "ordinary-simple-outage", TypeModel: TypeModelSimple}
	service.simplePricingCache.Store("segment-simple", simplePricingCacheEntry{
		CampaignID: campaign.ID, TypeModel: percenter.TypeModelSimple, RTB: false,
		ReferenceOriginalBid: 10, EffectiveMin: .20, MaxMargin: policy.MaxMargin,
		Pricing: percenter.SimplePricing{SegmentHash: "segment-simple", Margin: .40, SSPBid: 6, PointVersion: 31},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	pricing := service.resolveSimplePricing(ctx, campaign, "exact", "segment-simple", 12, .20, "ALL", false, time.Now().UTC())
	if pricing.PointVersion != 0 {
		t.Fatalf("changed ordinary original bid must not reuse cached point_version: %+v", pricing)
	}
	if math.Abs(pricing.Margin-.20) > 1e-12 || math.Abs(pricing.SSPBid-9.6) > 1e-12 {
		t.Fatalf("incompatible cache must fall back to bounded baseline for current original bid: %+v", pricing)
	}
}

func TestOrdinaryComplexRedisOutageRejectsCacheAfterOriginalBidChange(t *testing.T) {
	policy := percenter.ComplexPolicy{}.Normalize()
	service := &AuctionService{}
	service.ConfigureComplexPercenter(percenter.NewComplexStateStore(unavailablePercenterRedis(t), policy), policy)
	campaign := &Campaign{ID: "ordinary-complex-outage", TypeModel: TypeModelComplex}
	service.complexPricingCache.Store("segment-complex", complexPricingCacheEntry{
		CampaignID: campaign.ID, TypeModel: percenter.TypeModelComplex,
		ReferenceOriginalBid: 10, EffectiveMin: .20, MaxMargin: policy.MaxMargin,
		Pricing: percenter.ComplexPricing{SegmentHash: "segment-complex", AdvertiserPrice: 6.25, Margin: .20, SSPBid: 5, PointVersion: 41},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	pricing := service.resolveComplexPricing(ctx, campaign, "exact", "segment-complex", 12, .20, "ALL", time.Now().UTC())
	if pricing.PointVersion != 0 {
		t.Fatalf("changed Complex original bid must not reuse cached point_version: %+v", pricing)
	}
	if math.Abs(pricing.AdvertiserPrice-12) > 1e-12 || math.Abs(pricing.Margin-.20) > 1e-12 || math.Abs(pricing.SSPBid-9.6) > 1e-12 {
		t.Fatalf("incompatible Complex cache must use baseline built for current original bid: %+v", pricing)
	}
}

func TestRTBSimpleRedisOutageReusesCompatibleMarginForNewRawPrice(t *testing.T) {
	policy := percenter.SimplePolicy{}.Normalize()
	service := &AuctionService{}
	service.ConfigureSimplePercenter(percenter.NewSimpleStateStore(unavailablePercenterRedis(t), policy), policy)
	campaign := &Campaign{ID: "rtb-simple-outage", TypeModel: TypeModelSimple, RTB: true}
	service.simplePricingCache.Store("segment-rtb", simplePricingCacheEntry{
		CampaignID: campaign.ID, TypeModel: percenter.TypeModelSimple, RTB: true,
		ReferenceOriginalBid: 10, EffectiveMin: .30, MaxMargin: policy.MaxMargin,
		Pricing: percenter.SimplePricing{SegmentHash: "segment-rtb", Margin: .40, SSPBid: 6, PointVersion: 51},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	pricing := service.resolveSimplePricing(ctx, campaign, "exact", "segment-rtb", 20, .30, "ALL_RTB", true, time.Now().UTC())
	if pricing.PointVersion != 51 || math.Abs(pricing.Margin-.40) > 1e-12 || math.Abs(pricing.SSPBid-12) > 1e-12 {
		t.Fatalf("RTB rawPrice change must keep compatible cached margin and reprice SSP bid: %+v", pricing)
	}
}
