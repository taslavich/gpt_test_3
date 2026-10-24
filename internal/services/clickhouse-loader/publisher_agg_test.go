package clickhouse_loader

import (
	"context"
	"fmt"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func aggWriter(name string) string {
	start := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW {db}."+name)
	if start < 0 {
		return ""
	}
	end := strings.Index(ddlTemplate[start:], ";")
	return ddlTemplate[start : start+end+1]
}
func TestSharedAggStatsPublisherContract(t *testing.T) {
	for _, name := range []string{"mv_fact_impressions_to_agg_stats", "mv_fact_clicks_to_agg_stats", "mv_fact_clicks_wins_to_agg_stats", "mv_fact_conversions_to_agg_stats"} {
		view := aggWriter(name)
		if view == "" {
			t.Fatal(name)
		}
		for _, field := range []string{"pub_user_id", "ad_code_id", "zone_id", "ssp_revenue_views"} {
			if !strings.Contains(view, field) {
				t.Fatal(name, field)
			}
		}
		if name == "mv_fact_impressions_to_agg_stats" {
			if !strings.Contains(view, "sum(win_dsp_price / 1000) AS spend_views_table") || !strings.Contains(view, "toFloat64(0) AS ssp_revenue_views") {
				t.Fatal("price basis changed")
			}
		} else if !strings.Contains(view, "toFloat64(0) AS ssp_revenue_views") {
			t.Fatal("non-view revenue", name)
		}
	}
	if strings.Contains(ddlTemplate, "agg_publisher_stats") {
		t.Fatal("second money aggregate")
	}
	if strings.Count(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_fact_impressions_to_agg_stats SYNC;") != 1 {
		t.Fatal("old impressions writer not replaced")
	}
	start := strings.Index(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_fact_impressions_to_agg_stats SYNC;")
	end := strings.Index(ddlTemplate[start:], "-- IPP CUTOVER") + start
	migration := ddlTemplate[start:end]
	for _, field := range []string{"pub_user_id", "ad_code_id", "zone_id"} {
		if !strings.Contains(migration, "ADD COLUMN IF NOT EXISTS "+field+" String,") {
			t.Fatal("key addition must be without DEFAULT", field)
		}
	}
	if !strings.Contains(migration, "pub_user_id,\n        ad_code_id,\n        zone_id") {
		t.Fatal("key suffix")
	}
}

// Requires a disposable current ClickHouse server (same major as production).
// The test migrates a populated old sorting key; it does NOT reinterpret or
// reimplement recovery: known recovered fact rows go through the ordinary MV.
func TestSharedAggStatsRealMigrationAndRecoveryFacts(t *testing.T) {
	addr := os.Getenv("PUBLISHER_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("PUBLISHER_TEST_CLICKHOUSE_ADDR unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{addr}, Auth: clickhouse.Auth{Username: "default"}})
	if e != nil {
		t.Fatal(e)
	}
	defer ch.Close()
	db := "publisher_agg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	defer ch.Exec(context.Background(), "DROP DATABASE IF EXISTS "+db+" SYNC")
	if e = ch.Exec(ctx, "CREATE DATABASE "+db); e != nil {
		t.Fatal(e)
	}
	start := strings.Index(ddlTemplate, "CREATE TABLE IF NOT EXISTS {db}.agg_stats")
	end := strings.Index(ddlTemplate[start:], ";") + start
	legacy := ddlTemplate[start : end+1]
	for _, line := range []string{"    pub_user_id String DEFAULT '',\n", "    ad_code_id String DEFAULT '',\n", "    zone_id String DEFAULT '',\n", "    ssp_revenue_views Float64 DEFAULT 0,\n", "    pop_revenue_bucket UInt64 DEFAULT 0\n", "    pub_user_id,\n", "    ad_code_id,\n", "    zone_id,\n", "    pop_revenue_bucket\n"} {
		legacy = strings.ReplaceAll(legacy, line, "")
	}
	legacy = strings.ReplaceAll(legacy, "pop_recovered_spend Float64 DEFAULT 0,", "pop_recovered_spend Float64 DEFAULT 0")
	legacy = strings.ReplaceAll(legacy, "    os_version,\n)", "    os_version\n)")
	if e = ch.Exec(ctx, strings.ReplaceAll(legacy, "{db}", db)); e != nil {
		t.Fatal(e)
	}
	// Historical rows outside IPP repair retain their exact old advertiser totals.
	adv, campaign := uuid.NewString(), uuid.NewString()
	if e = ch.Exec(ctx, `INSERT INTO `+db+`.agg_stats(win_user_id,win_cid,event_date,event_hour,format,spend_views_table,spend_clicks_table) VALUES(?,?,today()-1,now()-INTERVAL 12 HOUR,'POP',7,0),(?,?,today()-1,now()-INTERVAL 12 HOUR,'IPP',0,2)`, adv, campaign, adv, campaign); e != nil {
		t.Fatal(e)
	}
	var beforeViews, beforeClicks float64
	if e = ch.QueryRow(ctx, `SELECT sum(spend_views_table),sum(spend_clicks_table) FROM `+db+`.agg_stats`).Scan(&beforeViews, &beforeClicks); e != nil {
		t.Fatal(e)
	}
	if e = CreateDB(ctx, ch, db); e != nil {
		t.Fatal(e)
	}
	pubA, pubB := uuid.NewString(), uuid.NewString()
	site := c.SiteID("example.test")
	codeA, codeB, codeC := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	cases := []struct {
		pub, code  string
		price, dsp float64
		recovered  uint8
	}{{pubA, codeA, 8, 10, 0}, {pubA, codeA, 8, 10, 0}, {pubA, codeB, 4, 7, 1}, {pubB, codeC, 3, 6, 0}, {"", "", 9, 9, 0}}
	started := time.Now()
	for _, v := range cases {
		zone := ""
		if v.pub != "" {
			zone = c.ZoneID(site, v.code)
		}
		if e = ch.Exec(ctx, `INSERT INTO `+db+`.fact_impressions(uuid,impressions_uuid,event_time_impressions,event_time,event_date,event_hour,win_user_id,win_cid,site_id,format,typic,pub_user_id,ad_code_id,zone_id,win_final_price,win_dsp_price,is_pop_recovered,pop_recovery_source_key) VALUES(?,?,?,?,?,?,?, ?,?,'POP','MAINSTREAM',?,?,?,?,?,?,?)`, uuid.NewString(), uuid.NewString(), now, now, now, now.Truncate(time.Hour), adv, campaign, site, v.pub, v.code, zone, v.price, v.dsp, v.recovered, fmt.Sprint("pop-click:", uuid.NewString())); e != nil {
			t.Fatal(e)
		}
	}
	insertTime := time.Since(started)
	// Clicks, IPP wins and conversions may carry attribution but never add POP SSP revenue.
	for _, source := range []string{"fact_clicks", "fact_clicks_wins", "fact_conversions"} {
		var columns, values string
		switch source {
		case "fact_clicks":
			columns = "event_time_clicks,clicks_uuid"
			values = "now64(),generateUUIDv4()"
		case "fact_clicks_wins":
			columns = "event_time_clicks_wins,clicks_wins_uuid"
			values = "now64(),generateUUIDv4()"
		case "fact_conversions":
			columns = "conversions_event_time,clicks_uuid"
			values = "now64(),generateUUIDv4()"
		}
		format := "POP"
		if source == "fact_clicks_wins" {
			format = "IPP"
		}
		// Zero money fields test every writer's projection without altering the old cost fixture.
		if e = ch.Exec(ctx, `INSERT INTO `+db+`.`+source+`(uuid,event_time,event_date,event_hour,win_user_id,win_cid,format,pub_user_id,ad_code_id,site_id,zone_id,`+columns+`) VALUES(generateUUIDv4(),now64(),today(),toStartOfHour(now()),?,?,?,?,?,?,?,`+values+`)`, adv, campaign, format, pubA, codeA, site, c.ZoneID(site, codeA)); e != nil {
			t.Fatal(source, e)
		}
	}
	var afterViews, afterClicks, revenue float64
	if e = ch.QueryRow(ctx, `SELECT sum(spend_views_table),sum(spend_clicks_table),sum(ssp_revenue_views) FROM `+db+`.agg_stats`).Scan(&afterViews, &afterClicks, &revenue); e != nil {
		t.Fatal(e)
	}
	if math.Abs(afterViews-beforeViews-.042) > 1e-10 || math.Abs(afterClicks-beforeClicks) > 1e-10 || math.Abs(revenue) > 1e-10 {
		t.Fatal(beforeViews, beforeClicks, afterViews, afterClicks, revenue)
	}
	var own float64
	if e = ch.QueryRow(ctx, `SELECT sum(ssp_revenue_views) FROM `+db+`.agg_stats WHERE pub_user_id=?`, pubA).Scan(&own); e != nil || math.Abs(own) > 1e-10 {
		t.Fatal(own, e)
	}
	if e = ch.Exec(ctx, "OPTIMIZE TABLE "+db+".agg_stats FINAL"); e != nil {
		t.Fatal(e)
	}
	var groups uint64
	if e = ch.QueryRow(ctx, `SELECT count() FROM (SELECT pub_user_id,ad_code_id,zone_id FROM `+db+`.agg_stats WHERE format='POP' AND pub_user_id!='' GROUP BY pub_user_id,ad_code_id,zone_id)`).Scan(&groups); e != nil || groups != 3 {
		t.Fatal("publisher keys merged", groups, e)
	}
	if e = CreateDB(ctx, ch, db); e != nil {
		t.Fatal("repeat migration", e)
	}
	var again float64
	if e = ch.QueryRow(ctx, `SELECT sum(ssp_revenue_views) FROM `+db+`.agg_stats`).Scan(&again); e != nil || math.Abs(again-revenue) > 1e-10 {
		t.Fatal("repeat DDL duplicated POP income", again, e)
	}
	var parts, bytes uint64
	if e = ch.QueryRow(ctx, `SELECT count(),sum(bytes_on_disk) FROM system.parts WHERE database=? AND table='agg_stats' AND active`, db).Scan(&parts, &bytes); e != nil {
		t.Fatal(e)
	}
	t.Logf("small fixture: insert+MVs=%s publisher_groups=%d parts=%d bytes=%d; not a production CPU/RPS benchmark", insertTime, groups, parts, bytes)
}
