package auction

import (
	"context"
	"log"
	"math"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

type simplePricingCacheEntry struct {
	CampaignID           string
	TypeModel            int
	RTB                  bool
	ReferenceOriginalBid float64
	EffectiveMin         float64
	MaxMargin            float64
	Pricing              percenter.SimplePricing
}

func (s *AuctionService) ConfigureSimplePercenter(store *percenter.SimpleStateStore, policy percenter.SimplePolicy) {
	if s == nil {
		return
	}
	s.simplePercenter = store
	s.simplePercenterPolicy = policy.Normalize()
}

func simpleCacheCompatible(entry simplePricingCacheEntry, campaign *Campaign, originalBid, effectiveMin float64, rtb bool, policy percenter.SimplePolicy) bool {
	if campaign == nil {
		return false
	}
	policy = policy.Normalize()
	if entry.CampaignID != campaign.ID || entry.TypeModel != percenter.TypeModelSimple || normalizeTypeModel(campaign.TypeModel) != TypeModelSimple || entry.RTB != rtb {
		return false
	}
	if math.Abs(entry.EffectiveMin-effectiveMin) > 1e-12 || math.Abs(entry.MaxMargin-policy.MaxMargin) > 1e-12 {
		return false
	}
	if !rtb && math.Abs(entry.ReferenceOriginalBid-originalBid) > 1e-12 {
		return false
	}
	pricing := entry.Pricing
	return pricing.PointVersion > 0 && pricing.Margin+1e-12 >= effectiveMin && pricing.Margin <= policy.MaxMargin+1e-12
}

func simpleCachedPricing(entry simplePricingCacheEntry, campaign *Campaign, originalBid, effectiveMin float64, rtb bool, policy percenter.SimplePolicy) (percenter.SimplePricing, bool) {
	if !simpleCacheCompatible(entry, campaign, originalBid, effectiveMin, rtb, policy) {
		return percenter.SimplePricing{}, false
	}
	pricing := entry.Pricing
	// RTB rawPrice is intentionally dynamic and does not invalidate the margin
	// state. Re-price the cached margin against the current external raw bid.
	pricing.SSPBid = originalBid * (1 - pricing.Margin)
	return pricing, true
}

func (s *AuctionService) resolveSimplePricing(
	ctx context.Context,
	campaign *Campaign,
	exactSegmentHash string,
	segmentHash string,
	originalBid float64,
	effectiveMin float64,
	mapSource string,
	rtb bool,
	now time.Time,
) percenter.SimplePricing {
	fallback := percenter.NewSimpleState(segmentHash, campaign.ID, originalBid, effectiveMin, rtb, s.simplePercenterPolicy, now, mapSource).PricingForOriginalBid(originalBid)
	// point_version=0 deliberately marks fail-open pricing. The optimizer must
	// not consume statistics for a point that was never persisted in Redis.
	fallback.PointVersion = 0
	if s == nil || s.simplePercenter == nil {
		if s != nil && s.percenterTelemetry != nil {
			s.percenterTelemetry.Record("state_store_unconfigured", campaign.ID, exactSegmentHash, segmentHash, percenter.TypeModelSimple, 0)
		}
		return fallback
	}
	pricing, err := s.simplePercenter.GetOrInitPricing(ctx, segmentHash, campaign.ID, originalBid, effectiveMin, rtb, now, mapSource)
	if err != nil {
		if cached, ok := s.simplePricingCache.Load(segmentHash); ok {
			entry, typeOK := cached.(simplePricingCacheEntry)
			if typeOK {
				if last, compatible := simpleCachedPricing(entry, campaign, originalBid, effectiveMin, rtb, s.simplePercenterPolicy); compatible {
					if s.percenterTelemetry != nil {
						s.percenterTelemetry.Record("state_fallback_local", campaign.ID, exactSegmentHash, segmentHash, percenter.TypeModelSimple, last.PointVersion)
					}
					log.Printf("[ADV][SIMPLE_PERCENTER_LOCAL_FALLBACK] campaign_id=%s segment_hash=%s rtb=%t point_version=%d error=%v", campaign.ID, segmentHash, rtb, last.PointVersion, err)
					return last
				}
			}
		}
		if s.percenterTelemetry != nil {
			s.percenterTelemetry.Record("state_init_failed", campaign.ID, exactSegmentHash, segmentHash, percenter.TypeModelSimple, 0)
		}
		log.Printf("[ADV][SIMPLE_PERCENTER_FALLBACK] campaign_id=%s segment_hash=%s rtb=%t error=%v", campaign.ID, segmentHash, rtb, err)
		return fallback
	}
	policy := s.simplePercenterPolicy.Normalize()
	s.simplePricingCache.Store(segmentHash, simplePricingCacheEntry{
		CampaignID: campaign.ID, TypeModel: percenter.TypeModelSimple, RTB: rtb,
		ReferenceOriginalBid: originalBid, EffectiveMin: effectiveMin, MaxMargin: policy.MaxMargin, Pricing: pricing,
	})
	return pricing
}
