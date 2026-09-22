package auction

import (
	"context"
	"log"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

func (s *AuctionService) ConfigureSimplePercenter(store *percenter.SimpleStateStore, policy percenter.SimplePolicy) {
	if s == nil {
		return
	}
	s.simplePercenter = store
	s.simplePercenterPolicy = policy.Normalize()
}

func (s *AuctionService) resolveSimplePricing(
	ctx context.Context,
	campaign *Campaign,
	segmentHash string,
	originalBid float64,
	effectiveMin float64,
	rtb bool,
	now time.Time,
) percenter.SimplePricing {
	fallback := percenter.NewSimpleState(segmentHash, campaign.ID, originalBid, effectiveMin, rtb, s.simplePercenterPolicy, now).PricingForOriginalBid(originalBid)
	// point_version=0 deliberately marks fail-open pricing. The optimizer must
	// not consume statistics for a point that was never persisted in Redis.
	fallback.PointVersion = 0
	if s == nil || s.simplePercenter == nil {
		return fallback
	}
	pricing, err := s.simplePercenter.GetOrInitPricing(ctx, segmentHash, campaign.ID, originalBid, effectiveMin, rtb, now)
	if err != nil {
		log.Printf("[ADV][SIMPLE_PERCENTER_FALLBACK] campaign_id=%s segment_hash=%s rtb=%t error=%v", campaign.ID, segmentHash, rtb, err)
		return fallback
	}
	return pricing
}
