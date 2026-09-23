package billing

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

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

func TestPromoDebitRequiresCapturedActiveGeneration(t *testing.T) {
	tests := []struct {
		name   string
		record outbox.Record
		want   bool
	}{
		{name: "current active promo", record: outbox.Record{TypeModel: 1, PromoStateCaptured: true, PromoActive: true, PromoGeneration: 4}, want: true},
		{name: "promo inactive at pricing", record: outbox.Record{TypeModel: 1, PromoStateCaptured: true, PromoActive: false, PromoGeneration: 4}, want: false},
		{name: "legacy winner without generation", record: outbox.Record{TypeModel: 1}, want: false},
		{name: "invalid campaign mode", record: outbox.Record{TypeModel: 99, PromoStateCaptured: true, PromoActive: true, PromoGeneration: 4}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := promoDebitRequired(tt.record); got != tt.want {
				t.Fatalf("promoDebitRequired(%+v)=%t want %t", tt.record, got, tt.want)
			}
		})
	}
}
