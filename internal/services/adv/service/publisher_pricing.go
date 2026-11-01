package auction

import "context"

type publisherPricingKey struct{}
type publisherPricingOverride struct {
	percent float64
	matched bool
}

func (s *AuctionService) ConfigurePublisherPercents(store *PublisherPercentStore) {
	s.publisherPercents = store
}

// Capture one map generation per auction, so a concurrent PUT cannot give
// competing campaigns different Publisher overrides in the same request.
func (s *AuctionService) publisherPricingContext(ctx context.Context, ownerID string) context.Context {
	percent, matched := s.publisherPercents.Lookup(ownerID)
	return context.WithValue(ctx, publisherPricingKey{}, publisherPricingOverride{percent, matched})
}

func (s *AuctionService) publisherMargin(ctx context.Context, campaign *Campaign) (float64, bool) {
	override, _ := ctx.Value(publisherPricingKey{}).(publisherPricingOverride)
	if !override.matched {
		return 0, false
	}
	percent := override.percent
	// This 20% floor is specific to explicit Publisher overrides. Existing
	// advertiser promo policies (30%) and promo spending are unchanged elsewhere.
	if s.effectivePromoSpendRemaining(campaign) > 0 && percent < 0.20 {
		percent = 0.20
	}
	return percent, true
}
