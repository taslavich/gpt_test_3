package auction

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

func TestPercenterProfitModelFollowsCampaignPaymentModelExceptPopunder(t *testing.T) {
	tests := []struct {
		name         string
		pricingModel string
		format       string
		want         string
	}{
		{name: "CPM banner", pricingModel: PricingModelCPM, format: "BAN", want: percenter.ProfitModelImpression},
		{name: "CPC push", pricingModel: PricingModelCPC, format: "IPP", want: percenter.ProfitModelClick},
		{name: "CPC native", pricingModel: PricingModelCPC, format: "NAT", want: percenter.ProfitModelClick},
		{name: "CPC popunder is CPM-equivalent", pricingModel: PricingModelCPC, format: "POP", want: percenter.ProfitModelImpression},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percenterProfitModel(tt.pricingModel, tt.format); got != tt.want {
				t.Fatalf("profit model=%q want %q", got, tt.want)
			}
		})
	}
}

func TestPercenterCampaignContextTreatsPopunderAsCPM(t *testing.T) {
	popCPC := &Campaign{Format: "popunder", PricingModel: PricingModelCPC}
	popCPM := &Campaign{Format: "popunder", PricingModel: PricingModelCPM}
	if got, want := percenterCampaignContext(popCPC), percenterCampaignContext(popCPM); got != want {
		t.Fatalf("popunder CPC/CPM must share the same percenter pricing context: %q != %q", got, want)
	}

	pushCPC := &Campaign{Format: "push", PricingModel: PricingModelCPC}
	pushCPM := &Campaign{Format: "push", PricingModel: PricingModelCPM}
	if got, other := percenterCampaignContext(pushCPC), percenterCampaignContext(pushCPM); got == other {
		t.Fatalf("non-pop pricing model must affect campaign version context: %q", got)
	}
}
