package auction

import (
	"context"
	"log"
	"math"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

type complexPricingCacheEntry struct {
	CampaignID           string
	TypeModel            int
	ReferenceOriginalBid float64
	EffectiveMin         float64
	MaxMargin            float64
	Pricing              percenter.ComplexPricing
}

func (s *AuctionService) ConfigureComplexPercenter(store *percenter.ComplexStateStore, policy percenter.ComplexPolicy) {
	if s == nil {
		return
	}
	s.complexPercenter = store
	s.complexPercenterPolicy = policy.Normalize()
}

func shouldUseComplexPercenter(campaign *Campaign) bool {
	return campaign != nil && !campaign.RTB && normalizeTypeModel(campaign.TypeModel) == TypeModelComplex
}

func complexCachedPricing(entry complexPricingCacheEntry, campaign *Campaign, originalBid, effectiveMin float64, policy percenter.ComplexPolicy) (percenter.ComplexPricing, bool) {
	if campaign == nil || campaign.RTB {
		return percenter.ComplexPricing{}, false
	}
	policy = policy.Normalize()
	if entry.CampaignID != campaign.ID || entry.TypeModel != percenter.TypeModelComplex || normalizeTypeModel(campaign.TypeModel) != TypeModelComplex ||
		math.Abs(entry.ReferenceOriginalBid-originalBid) > 1e-12 || math.Abs(entry.EffectiveMin-effectiveMin) > 1e-12 ||
		math.Abs(entry.MaxMargin-policy.MaxMargin) > 1e-12 {
		return percenter.ComplexPricing{}, false
	}
	pricing := entry.Pricing
	if pricing.PointVersion == 0 || pricing.Margin+1e-12 < effectiveMin || pricing.Margin > policy.MaxMargin+1e-12 ||
		pricing.AdvertiserPrice <= 0 || pricing.SSPBid <= 0 {
		return percenter.ComplexPricing{}, false
	}
	return pricing, true
}

func (s *AuctionService) resolveComplexPricing(
	ctx context.Context,
	campaign *Campaign,
	exactSegmentHash string,
	segmentHash string,
	originalBid float64,
	effectiveMin float64,
	mapSource string,
	now time.Time,
) percenter.ComplexPricing {
	fallback := percenter.NewComplexState(segmentHash, campaign.ID, originalBid, effectiveMin, s.complexPercenterPolicy, now, mapSource).Pricing()
	fallback.PointVersion = 0
	if s == nil || s.complexPercenter == nil {
		if s != nil && s.percenterTelemetry != nil {
			s.percenterTelemetry.Record("state_store_unconfigured", campaign.ID, exactSegmentHash, segmentHash, percenter.TypeModelComplex, 0)
		}
		return fallback
	}
	pricing, err := s.complexPercenter.GetOrInitPricing(ctx, segmentHash, campaign.ID, originalBid, effectiveMin, now, mapSource)
	if err != nil {
		if cached, ok := s.complexPricingCache.Load(segmentHash); ok {
			entry, typeOK := cached.(complexPricingCacheEntry)
			if typeOK {
				if last, compatible := complexCachedPricing(entry, campaign, originalBid, effectiveMin, s.complexPercenterPolicy); compatible {
					if s.percenterTelemetry != nil {
						s.percenterTelemetry.Record("state_fallback_local", campaign.ID, exactSegmentHash, segmentHash, percenter.TypeModelComplex, last.PointVersion)
					}
					log.Printf("[ADV][COMPLEX_PERCENTER_LOCAL_FALLBACK] campaign_id=%s segment_hash=%s point_version=%d error=%v", campaign.ID, segmentHash, last.PointVersion, err)
					return last
				}
			}
		}
		if s.percenterTelemetry != nil {
			s.percenterTelemetry.Record("state_init_failed", campaign.ID, exactSegmentHash, segmentHash, percenter.TypeModelComplex, 0)
		}
		log.Printf("[ADV][COMPLEX_PERCENTER_FALLBACK] campaign_id=%s segment_hash=%s error=%v", campaign.ID, segmentHash, err)
		return fallback
	}
	policy := s.complexPercenterPolicy.Normalize()
	s.complexPricingCache.Store(segmentHash, complexPricingCacheEntry{
		CampaignID: campaign.ID, TypeModel: percenter.TypeModelComplex, ReferenceOriginalBid: originalBid,
		EffectiveMin: effectiveMin, MaxMargin: policy.MaxMargin, Pricing: pricing,
	})
	return pricing
}
