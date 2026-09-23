package billing

import "testing"

func TestPromoSpendAppliesToEveryValidCampaignMode(t *testing.T) {
	tests := []struct {
		name      string
		typeModel int
		want      bool
	}{
		{name: "legacy simple", typeModel: 0, want: true},
		{name: "simple", typeModel: 1, want: true},
		{name: "complex", typeModel: 2, want: true},
		{name: "map only", typeModel: 3, want: true},
		{name: "invalid", typeModel: 99, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := promoSpendAppliesToTypeModel(tt.typeModel); got != tt.want {
				t.Fatalf("promoSpendAppliesToTypeModel(%d)=%t want %t", tt.typeModel, got, tt.want)
			}
		})
	}
}
