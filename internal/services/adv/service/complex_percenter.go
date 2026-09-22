package auction

import (
	"context"
	"log"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

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

func (s *AuctionService) resolveComplexPricing(
	ctx context.Context,
	campaign *Campaign,
	segmentHash string,
	originalBid float64,
	effectiveMin float64,
	now time.Time,
) percenter.ComplexPricing {
	fallback := percenter.NewComplexState(segmentHash, campaign.ID, originalBid, effectiveMin, s.complexPercenterPolicy, now).Pricing()
	fallback.PointVersion = 0
	if s == nil || s.complexPercenter == nil {
		return fallback
	}
	pricing, err := s.complexPercenter.GetOrInitPricing(ctx, segmentHash, campaign.ID, originalBid, effectiveMin, now)
	if err != nil {
		log.Printf("[ADV][COMPLEX_PERCENTER_FALLBACK] campaign_id=%s segment_hash=%s error=%v", campaign.ID, segmentHash, err)
		return fallback
	}
	return pricing
}
