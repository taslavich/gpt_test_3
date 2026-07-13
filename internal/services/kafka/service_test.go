package kafka_service

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

func TestKafkaTopicsFinancialOnlySpentTotals(t *testing.T) {
	cfg := config.KafkaConfig{KafkaTopicOrtb: "ortb", KafkaTopicImpressions: "impressions", KafkaTopicClicks: "clicks", KafkaTopicConversions: "conversions", KafkaTopicSpentTotals: "spent_totals"}
	topics := kafkaTopics(cfg)
	seen := map[string]bool{}
	for _, topic := range topics {
		seen[topic] = true
	}
	if !seen["spent_totals"] {
		t.Fatalf("spent_totals topic missing: %v", topics)
	}
	for _, legacy := range []string{"user_balance_plus", "user_balance_minus", "campaign_balance_minus", "campaign_balance_plus", "campaigns_created"} {
		if seen[legacy] {
			t.Fatalf("legacy financial topic %s must not be auto-created", legacy)
		}
	}
}

func TestSpentTotalKey(t *testing.T) {
	if got := string(SpentTotalKey("campaign", "42")); got != "campaign:42" {
		t.Fatalf("unexpected key %q", got)
	}
}
