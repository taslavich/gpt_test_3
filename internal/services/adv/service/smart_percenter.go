package auction

import (
	"fmt"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

// ConfigurePercenter wires both standard (type_model=1) and Smart
// (type_model=2) percenter modes into AuctionService. cmd/adv only assembles
// dependencies; pricing decisions remain in the service/percenter packages.
func (s *AuctionService) ConfigurePercenter(store *percenter.StateStore, policy percenter.Policy) {
	if s == nil {
		return
	}
	s.smartPercenter = store
	s.percenterPolicy = policy.Normalize()
}

// ConfigureSmartPercenter is kept as a compatibility alias for older wiring.
func (s *AuctionService) ConfigureSmartPercenter(store *percenter.StateStore, policy percenter.Policy) {
	s.ConfigurePercenter(store, policy)
}

func percenterProfitModel(pricingModel, format string) string {
	// Popunder is always auctioned and stored as CPM-equivalent, regardless of
	// the pricing_model value preserved for the cabinet UI.
	if normalizeFormat(format) == "POP" {
		return percenter.ProfitModelImpression
	}
	if strings.EqualFold(strings.TrimSpace(pricingModel), PricingModelCPC) {
		return percenter.ProfitModelClick
	}
	return percenter.ProfitModelImpression
}

func percenterCampaignContext(campaign *Campaign) string {
	if campaign == nil {
		return ""
	}
	format := normalizeFormat(campaign.Format)
	model := strings.ToUpper(strings.TrimSpace(campaign.PricingModel))
	if format == "POP" {
		model = PricingModelCPM
	}
	return fmt.Sprintf("%s|%s", format, model)
}
