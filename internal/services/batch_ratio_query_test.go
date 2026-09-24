package services

import (
	"strings"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

func TestBuildBatchRatioDiffsQueryUsesPhysicalOrtbTable(t *testing.T) {
	cfg := config.BatchRatioConfig{
		TableOrtb:        "ads.ortb",
		TableImpressions: "ads.impressions_in",
		TableClicks:      "ads.clicks_in",
	}

	query := buildBatchRatioDiffsQuery(cfg)

	if strings.Contains(query, "ortb_logical") {
		t.Fatalf("batch-ratio query must not read global dedupe view ortb_logical: %s", query)
	}
	if got := strings.Count(query, "FROM ads.ortb"); got != 2 {
		t.Fatalf("batch-ratio query must read physical ORTB table in latest-batch subquery and max(created_at) subquery; got %d occurrences\n%s", got, query)
	}
	if !strings.Contains(query, "WHERE created_at = (") || !strings.Contains(query, "SELECT max(created_at)") {
		t.Fatalf("batch-ratio query must stay restricted to the newest physical insert batch: %s", query)
	}
}

func TestBuildBatchRatioDiffsQueryQuotesConfiguredTables(t *testing.T) {
	cfg := config.BatchRatioConfig{
		TableOrtb:        "ads-prod.ortb-events",
		TableImpressions: "ads-prod.impressions-in",
		TableClicks:      "ads-prod.clicks-in",
	}

	query := buildBatchRatioDiffsQuery(cfg)
	for _, want := range []string{
		"`ads-prod`.`ortb-events`",
		"`ads-prod`.`impressions-in`",
		"`ads-prod`.`clicks-in`",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query does not contain safely quoted table %q: %s", want, query)
		}
	}
}
