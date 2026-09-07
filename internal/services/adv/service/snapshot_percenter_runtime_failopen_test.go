package auction

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

func TestApplyFailOpenPercenterVersionsReusesOnlyCompatibleState(t *testing.T) {
	policy := percenter.Policy{DefaultMinMargin: 0.20, PromoMinMargin: 0.30}.Normalize()
	previous := &Snapshot{
		Campaigns: []*Campaign{
			{ID: "same", UserID: "u1", TypeModel: percenter.TypeModelSmart, BasePrice: 1, Format: "BANNER", PricingModel: PricingModelCPM, PercenterCampaignVersion: 7},
			{ID: "changed", UserID: "u1", TypeModel: percenter.TypeModelSmart, BasePrice: 1, Format: "BANNER", PricingModel: PricingModelCPM, PercenterCampaignVersion: 8},
			{ID: "promo", UserID: "u2", TypeModel: percenter.TypeModelSimple, BasePrice: 2, Format: "BANNER", PricingModel: PricingModelCPM, PercenterCampaignVersion: 9},
		},
		UserPromoSpendRemaining: map[string]float64{"u1": 0, "u2": 0},
	}
	next := &Snapshot{
		Campaigns: []*Campaign{
			{ID: "same", UserID: "u1", TypeModel: percenter.TypeModelSmart, BasePrice: 1, Format: "BANNER", PricingModel: PricingModelCPM},
			{ID: "changed", UserID: "u1", TypeModel: percenter.TypeModelSmart, BasePrice: 1.1, Format: "BANNER", PricingModel: PricingModelCPM},
			{ID: "promo", UserID: "u2", TypeModel: percenter.TypeModelSimple, BasePrice: 2, Format: "BANNER", PricingModel: PricingModelCPM},
			{ID: "new", UserID: "u3", TypeModel: percenter.TypeModelSmart, BasePrice: 3, Format: "BANNER", PricingModel: PricingModelCPM},
		},
		UserPromoSpendRemaining: map[string]float64{"u1": 0, "u2": 1, "u3": 0},
	}

	reused, baseline := applyFailOpenPercenterVersions(previous, next, policy)
	if reused != 1 || baseline != 3 {
		t.Fatalf("reused=%d baseline=%d", reused, baseline)
	}
	if got := next.Campaigns[0].PercenterCampaignVersion; got != 7 {
		t.Fatalf("unchanged campaign version=%d want=7", got)
	}
	for _, campaign := range next.Campaigns[1:] {
		if campaign.PercenterCampaignVersion != 0 {
			t.Fatalf("campaign %s must use baseline version=0, got %d", campaign.ID, campaign.PercenterCampaignVersion)
		}
	}
}
