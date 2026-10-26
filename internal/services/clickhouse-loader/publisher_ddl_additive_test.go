package clickhouse_loader

import (
	"os"
	"strings"
	"testing"
)

func TestPublisherDDLNeverDeletesHistoricalTablesOrColumns(t *testing.T) {
	for _, forbidden := range []string{
		"DROP TABLE", "DROP COLUMN", "DROP PROJECTION", "TRUNCATE TABLE",
		"DELETE WHERE", "RENAME TABLE", "{ipp_cutover_from}",
	} {
		if strings.Contains(strings.ToUpper(ddlTemplate), strings.ToUpper(forbidden)) {
			t.Fatalf("forbidden automatic data-destructive DDL: %s", forbidden)
		}
	}
	body, err := os.ReadFile("publisher_safe_schema.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"DROP COLUMN", "DROP PROJECTION", "MODIFY QUERY"} {
		// The production Publisher schema path must not execute this SQL.
		if strings.Contains(strings.ToUpper(string(body)), "\""+forbidden) {
			t.Fatalf("forbidden statement in Publisher migration: %s", forbidden)
		}
	}
}

func TestPublisherFinancialMVCreateDefinitionsComplete(t *testing.T) {
	for _, name := range []string{
		"mv_impressions_to_fact", "mv_clicks_wins_to_fact", "mv_conversions_to_fact", "mv_clicks_to_fact",
		"mv_fact_impressions_to_agg_stats", "mv_fact_clicks_to_agg_stats",
		"mv_fact_clicks_wins_to_agg_stats", "mv_fact_conversions_to_agg_stats",
		"mv_ortb_to_publisher_inventory", "mv_fact_clicks_to_publisher_pop_revenue",
		"mv_publisher_pop_revenue_to_agg_stats", "mv_publisher_pop_revenue_checkpoint",
	} {
		stmt, err := ddlStatement("CREATE MATERIALIZED VIEW {db}." + name)
		if err != nil {
			stmt, err = ddlStatement("CREATE MATERIALIZED VIEW IF NOT EXISTS {db}." + name)
		}
		if err != nil {
			t.Fatal(name, err)
		}
		if !strings.Contains(stmt, " AS") && !strings.Contains(stmt, "\nAS") {
			t.Errorf("MV %s missing SELECT/AS", name)
		}
	}
}
