package clickhouse_loader

import (
	"context"
	"fmt"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	events "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"os"
	"testing"
	"time"
)

func TestPublisherRealClickHouseIngestAndRecovery(t *testing.T) {
	addr := os.Getenv("PUBLISHER_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("PUBLISHER_TEST_CLICKHOUSE_ADDR unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{addr}, Auth: clickhouse.Auth{Username: "default"}})
	if e != nil {
		t.Fatal(e)
	}
	defer ch.Close()
	database := "publisher_test_" + fmt.Sprintf("%d", time.Now().UnixNano())
	defer ch.Exec(context.Background(), "DROP DATABASE IF EXISTS "+database+" SYNC")
	if e = CreateDB(ctx, ch, database); e != nil {
		t.Fatal(e)
	}
	old := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour + 5*time.Minute)
	auction := uuid.NewString()
	imp := uuid.NewString()
	records := []events.OrtbEvent{{Uuid: auction, EventTimeMs: old.UnixMilli(), Code: 200, Format: "POP", Typic: "MAINSTREAM", PubUserId: "publisher", WinUserId: "advertiser", AdCodeId: "code", ZoneId: "zone", SiteId: "site", SiteDomain: "example.test", WinDspPrice: 1, WinPrice: .8}}
	if _, e = insertBatchOrtb(ctx, ch, database+".ortb", records); e != nil {
		t.Fatal(e)
	}
	if e = ch.Exec(ctx, "INSERT INTO "+database+".impressions_in(uuid,impressions_uuid,ad_format,event_time_impressions) VALUES(?,?,?,?)", auction, imp, "POP", old); e != nil {
		t.Fatal(e)
	}
	for _, view := range []string{"mv_impressions_to_fact"} {
		if e = ch.Exec(ctx, "SYSTEM REFRESH VIEW "+database+"."+view); e != nil {
			t.Fatal(e)
		}
		if e = ch.Exec(ctx, "SYSTEM WAIT VIEW "+database+"."+view); e != nil {
			t.Fatal(e)
		}
	}
	var pub, adv, code, zone string
	if e = ch.QueryRow(ctx, "SELECT pub_user_id,win_user_id,ad_code_id,zone_id FROM "+database+".fact_impressions WHERE uuid=? LIMIT 1", auction).Scan(&pub, &adv, &code, &zone); e != nil {
		t.Fatal(e)
	}
	if pub != "publisher" || adv != "advertiser" || code != "code" || zone != "zone" {
		t.Fatal(pub, adv, code, zone)
	}
	for i := 0; i < 2; i++ {
		if e = ch.Exec(ctx, "INSERT INTO "+database+".clicks_in(uuid,clicks_uuid,ad_format,event_time_clicks) VALUES(?,?,?,?)", auction, uuid.NewString(), "POP", old); e != nil {
			t.Fatal(e)
		}
		if e = ch.Exec(ctx, "SYSTEM REFRESH VIEW "+database+".mv_clicks_to_fact"); e != nil {
			t.Fatal(e)
		}
		if e = ch.Exec(ctx, "SYSTEM WAIT VIEW "+database+".mv_clicks_to_fact"); e != nil {
			t.Fatal(e)
		}
	}
	for _, view := range []string{"mv_clicks_to_fact", "mv_recover_pop_impressions_v2"} {
		if e = ch.Exec(ctx, "SYSTEM REFRESH VIEW "+database+"."+view); e != nil {
			t.Fatal(e)
		}
		if e = ch.Exec(ctx, "SYSTEM WAIT VIEW "+database+"."+view); e != nil {
			t.Fatal(e)
		}
	}
	var clickCount uint64
	ch.QueryRow(ctx, "SELECT count() FROM "+database+".fact_clicks WHERE uuid=?", auction).Scan(&clickCount)
	t.Log("clicks", clickCount, "old", old)
	var now time.Time
	ch.QueryRow(ctx, "SELECT now()").Scan(&now)
	t.Log("CH now", now)
	var recovered uint64
	if e = ch.QueryRow(ctx, "SELECT count() FROM "+database+".fact_impressions WHERE uuid=? AND is_pop_recovered=1 AND pub_user_id='publisher' AND win_user_id='advertiser' AND ad_code_id='code' AND zone_id='zone'", auction).Scan(&recovered); e != nil || recovered != 1 {
		t.Fatal("recovery attribution", recovered, e)
	}
}
