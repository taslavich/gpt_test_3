package clickhouse_loader

import (
	"strings"
	"testing"
)

func TestPublisherSchemaAndRecoveryProjections(t *testing.T) {
	for _, name := range []string{"ortb", "fact_impressions", "fact_clicks", "fact_clicks_wins", "fact_conversions"} {
		for _, field := range []string{"pub_user_id", "ad_code_id", "zone_id"} {
			if !strings.Contains(ddlTemplate, "ALTER TABLE {db}."+name+" ADD COLUMN IF NOT EXISTS "+field) {
				t.Fatal(name, field)
			}
		}
	}
	for _, field := range []string{"pub_user_id", "ad_code_id", "zone_id"} {
		for _, prefix := range []string{"o.", "c.", "i."} {
			if !strings.Contains(ddlTemplate, prefix+field+" AS "+field) {
				t.Fatal("projection lost", prefix, field)
			}
		}
	}
	if strings.Contains(ddlTemplate, "CREATE TABLE IF NOT EXISTS {db}.publisher_events") {
		t.Fatal("parallel pipeline")
	}
}
