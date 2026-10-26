package clickhouse_loader

import (
	"context"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPublisherKeyNormalizationAndPrefix(t *testing.T) {
	old := "(win_user_id, win_cid, win_crid, event_date, device_type, os, event_hour, browser, geo, site_id, format, typic, os_version)"
	want := strings.Join(publisherAggPrefix, ",")
	if normalizedKey(old) != normalizedKey(want) {
		t.Fatalf("legacy key was not recognized: %s != %s", normalizedKey(old), normalizedKey(want))
	}
	newKey := strings.Join(append(append([]string{}, publisherAggPrefix...), publisherAggSuffix...), ",")
	if normalizedKey(newKey) == normalizedKey(old) {
		t.Fatal("publisher key did not extend legacy prefix")
	}
}

func TestPublisherCurrentDDLStatementsExtractable(t *testing.T) {
	for _, prefix := range []string{
		"CREATE TABLE IF NOT EXISTS {db}.publisher_inventory",
		"CREATE TABLE IF NOT EXISTS {db}.publisher_pop_revenue_events_v2",
		"CREATE TABLE IF NOT EXISTS {db}.publisher_pop_revenue_checkpoints",
		"ALTER TABLE {db}.agg_stats ADD PROJECTION IF NOT EXISTS publisher_pop_by_bucket",
		"CREATE VIEW {db}.publisher_pop_revenue_dirty AS",
		"CREATE VIEW {db}.publisher_pop_revenue_delta AS",
	} {
		query, err := ddlStatement(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(query, prefix) {
			t.Errorf("wrong DDL returned for %s", prefix)
		}
		if strings.Contains(strings.ToUpper(query), "DROP TABLE") || strings.Contains(strings.ToUpper(query), "TRUNCATE TABLE") {
			t.Fatalf("destructive Publisher schema query: %s", prefix)
		}
	}
}

func TestPublisherMoneyViewsAreUpdatedWithoutDrop(t *testing.T) {
	for _, name := range []string{
		"mv_impressions_to_fact", "mv_clicks_wins_to_fact", "mv_conversions_to_fact", "mv_clicks_to_fact",
		"mv_fact_impressions_to_agg_stats", "mv_fact_clicks_to_agg_stats", "mv_fact_clicks_wins_to_agg_stats", "mv_fact_conversions_to_agg_stats",
		"mv_ortb_to_publisher_inventory", "mv_fact_clicks_to_publisher_pop_revenue",
		"mv_publisher_pop_revenue_to_agg_stats", "mv_publisher_pop_revenue_checkpoint",
	} {
		stmt, err := ddlStatement("CREATE MATERIALIZED VIEW {db}." + name)
		if err != nil {
			stmt, err = ddlStatement("CREATE MATERIALIZED VIEW IF NOT EXISTS {db}." + name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stmt, "AS") || !strings.Contains(stmt, "SELECT") {
			t.Fatalf("no SELECT in %s", name)
		}
	}
	if !strings.Contains(strings.Join(publisherAggSuffix, ","), "pop_revenue_bucket") {
		t.Fatal("POP idempotency bucket missing")
	}
}

// Requires disposable ClickHouse; verifies migration on a genuinely populated
// legacy SummingMergeTree, WITHOUT dropping/copying/renaming its historical data.
func TestPublisherLegacyAggKeyPreservesHistoricalAdvertiserMoney(t *testing.T) {
	addr := os.Getenv("PUBLISHER_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("PUBLISHER_TEST_CLICKHOUSE_ADDR unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ch, err := clickhouse.Open(&clickhouse.Options{Addr: []string{addr}, Auth: clickhouse.Auth{Username: "default"}})
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	db := "pub_inplace_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	defer ch.Exec(context.Background(), "DROP DATABASE IF EXISTS "+db+" SYNC")
	if err = ch.Exec(ctx, "CREATE DATABASE "+db); err != nil {
		t.Fatal(err)
	}
	create := "CREATE TABLE " + db + ".agg_stats(" +
		"win_user_id String,win_cid String,win_crid String,event_date Date,device_type String,os String," +
		"event_hour DateTime('UTC'),browser String,geo String,site_id String,format String,typic String," +
		"os_version String,pub_user_id String DEFAULT '',ad_code_id String DEFAULT '',zone_id String DEFAULT ''," +
		"pop_revenue_bucket UInt64 DEFAULT 0,spend_views_table Float64,spend_clicks_table Float64," +
		"ssp_revenue_views Float64 DEFAULT 0) ENGINE=SummingMergeTree PARTITION BY toYYYYMMDD(event_date) ORDER BY (" +
		strings.Join(publisherAggPrefix, ",") + ")"
	if err = ch.Exec(ctx, create); err != nil {
		t.Fatal(err)
	}
	if err = ch.Exec(ctx, "INSERT INTO "+db+".agg_stats(win_user_id,win_cid,event_date,event_hour,format,spend_views_table,spend_clicks_table) VALUES ('adv','camp',toDate('2026-04-01'),toDateTime('2026-04-01 00:00:00','UTC'),'POP',123.25,0),('adv','camp',toDate('2026-05-01'),toDateTime('2026-05-01 00:00:00','UTC'),'IPP',0,91.5)"); err != nil {
		t.Fatal(err)
	}
	s, err := diagnosticSchema(ctx, ch, db, "agg_stats")
	if err != nil {
		t.Fatal(err)
	}
	if err = ensurePublisherAggKey(ctx, ch, db, s, false); err != nil {
		t.Fatal(err)
	}
	var rows uint64
	var views, clicks float64
	if err = ch.QueryRow(ctx, "SELECT count(),sum(spend_views_table),sum(spend_clicks_table) FROM "+db+".agg_stats").Scan(&rows, &views, &clicks); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || views != 123.25 || clicks != 91.5 {
		t.Fatalf("historical money changed: rows=%d views=%f clicks=%f", rows, views, clicks)
	}
	upgraded, err := diagnosticSchema(ctx, ch, db, "agg_stats")
	if err != nil {
		t.Fatal(err)
	}
	expected := normalizedKey(strings.Join(append(append([]string{}, publisherAggPrefix...), publisherAggSuffix...), ","))
	if normalizedKey(upgraded.key) != expected {
		t.Fatalf("wrong key %q", upgraded.key)
	}
}
