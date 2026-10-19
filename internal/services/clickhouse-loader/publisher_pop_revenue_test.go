package clickhouse_loader

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
)

func TestPublisherPOPRevenueCanonicalAcrossBlocksAndRestart(t *testing.T) {
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
	db := "pub_money_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	defer ch.Exec(context.Background(), "DROP DATABASE IF EXISTS "+db+" SYNC")
	must := func(sql string, args ...any) {
		t.Helper()
		if e := ch.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e, sql)
		}
	}
	if e = CreateDB(ctx, ch, db); e != nil {
		t.Fatal(e)
	}
	must("SYSTEM STOP VIEW " + db + ".mv_publisher_pop_revenue_to_agg_stats")
	refresh := func() {
		t.Helper()
		must("SYSTEM START VIEW " + db + ".mv_publisher_pop_revenue_to_agg_stats")
		must("SYSTEM REFRESH VIEW " + db + ".mv_publisher_pop_revenue_to_agg_stats")
		must("SYSTEM WAIT VIEW " + db + ".mv_publisher_pop_revenue_to_agg_stats")
		must("SYSTEM STOP VIEW " + db + ".mv_publisher_pop_revenue_to_agg_stats")
	}
	money := func(want float64) {
		t.Helper()
		var v float64
		if e := ch.QueryRow(ctx, "SELECT sum(ssp_revenue_views) FROM "+db+".agg_stats WHERE pub_user_id='publisher' AND format='POP'").Scan(&v); e != nil || math.Abs(v-want) > 1e-10 {
			t.Fatalf("revenue=%v want %v error=%v", v, want, e)
		}
	}
	clickID, auction := uuid.NewString(), uuid.NewString()
	insertClick := func(id, pub, code, format string, price float64) {
		t.Helper()
		must("INSERT INTO "+db+".fact_clicks(uuid,clicks_uuid,event_time,event_time_clicks,event_date,event_hour,format,pub_user_id,ad_code_id,zone_id,site_id,win_final_price,win_dsp_price,win_user_id,win_cid) VALUES(?,?,now64(3),now64(3),today(),toStartOfHour(now()),?,?,?,'zone','site',?,4,'advertiser','campaign')", auction, id, format, pub, code, price)
	}
	insertClick(clickID, "publisher", "code", "POP", 2)
	refresh()
	money(.002)
	// Redelivery in several independent insert blocks, not merely DISTINCT in one block.
	for i := 0; i < 4; i++ {
		insertClick(clickID, "publisher", "code", "POP", 2)
		refresh()
		money(.002)
	}
	second := uuid.NewString()
	insertClick(second, "publisher", "other-code", "POP", 3)
	// Simulate an interrupted/unknown previous refresh that committed a partial amount.
	must("INSERT INTO " + db + ".agg_stats(win_user_id,win_cid,event_date,event_hour,format,pub_user_id,ad_code_id,zone_id,site_id,pop_revenue_bucket,ssp_revenue_views) VALUES('advertiser','campaign',today(),toStartOfHour(now()),'POP','publisher','other-code','zone','site',1+cityHash64(toUUID('" + second + "'))%256,.001)")
	refresh()
	money(.005)
	// Ordinary POP has no publisher income; an IPP click is not a POP view payment.
	insertClick(uuid.NewString(), "", "", "POP", 900)
	insertClick(uuid.NewString(), "publisher", "ipp-code", "IPP", 900)
	// BURL and recovery impressions still count/spend for the advertiser; neither pays POP SSP.
	for _, recovered := range []uint8{0, 1} {
		must("INSERT INTO "+db+".fact_impressions(uuid,impressions_uuid,event_date,event_hour,format,pub_user_id,ad_code_id,zone_id,site_id,win_final_price,win_dsp_price,is_pop_recovered) VALUES(?,?,today(),toStartOfHour(now()),'POP','publisher','code','zone','site',999,7,?)", auction, uuid.NewString(), recovered)
	}
	// Other Publisher formats are not payable in the current popunder-only MVP.
	must("INSERT INTO "+db+".fact_impressions(uuid,impressions_uuid,event_date,event_hour,format,pub_user_id,win_final_price,win_dsp_price) VALUES(?,?,today(),toStartOfHour(now()),'BAN','publisher',8,10)", uuid.NewString(), uuid.NewString())
	refresh()
	money(.005)
	var views uint64
	var cost, ban float64
	if e := ch.QueryRow(ctx, "SELECT sum(impressions),sum(spend_views_table),sumIf(ssp_revenue_views,format='BAN') FROM "+db+".agg_stats").Scan(&views, &cost, &ban); e != nil || views != 3 || math.Abs(cost-.024) > 1e-10 || math.Abs(ban) > 1e-10 {
		t.Fatal("advertiser/other format regression", views, cost, ban, e)
	}
	must("OPTIMIZE TABLE " + db + ".publisher_pop_revenue_events_v2 FINAL")
	refresh()
	money(.005)
	if e = CreateDB(ctx, ch, db); e != nil {
		t.Fatal("repeat DDL", e)
	}
	must("SYSTEM STOP VIEW " + db + ".mv_publisher_pop_revenue_to_agg_stats")
	refresh()
	money(.005)
	// The registry has no TTL: source fact expiration cannot make an old replay new.
	must("ALTER TABLE " + db + ".fact_clicks DELETE WHERE pub_user_id='publisher' SETTINGS mutations_sync=1")
	refresh()
	money(.005)
	insertClick(clickID, "publisher", "code", "POP", 2)
	refresh()
	money(.005)
	t.Log(fmt.Sprintf("unique POP clicks=2, income=.005; duplicates/partial commit/DDL restart/fact expiry do not duplicate income"))
}
