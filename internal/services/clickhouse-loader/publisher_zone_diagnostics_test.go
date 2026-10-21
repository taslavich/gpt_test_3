package clickhouse_loader

import (
	"context"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestPublisherZoneSnapshots(t *testing.T) {
	f := newPOPRevenueFixture(t)
	f.exec("SYSTEM STOP VIEW " + f.db + ".mv_publisher_zone_hourly")
	// CreateDB is the actual RTB DDL; no guessed substitute ORTB schema.
	insert := func(id, zone, age string, code int) {
		f.exec("INSERT INTO "+f.db+".ortb(uuid,logical_event_id,event_time,code,format,pub_user_id,ad_code_id,zone_id,site_id,site_domain) SELECT toUUID(?),?,now64(3,'UTC')-INTERVAL "+age+",?,'POP','publisher','code',?,'site','example.test'", uuid.NewString(), id, code, zone)
	}
	insert("a", "ok", "1 MINUTE", 200)
	insert("a", "ok", "1 MINUTE", 200)
	insert("b", "empty", "2 MINUTE", 204)
	insert("c", "mixed", "2 MINUTE", 200)
	insert("d", "mixed", "2 MINUTE", 500)
	insert("e", "error", "3 MINUTE", 403)
	insert("f", "sealed", "55 MINUTE", 200)
	refresh := func() { f.exec("INSERT INTO " + f.db + ".publisher_zone_hourly " + PublisherZoneSelect(f.db, 45)) }
	// Bootstrap covers the available hour, normal refresh only mutable slots.
	f.exec("INSERT INTO " + f.db + ".publisher_zone_hourly " + PublisherZoneSelect(f.db, 60))
	refresh()
	refresh()
	var total uint64
	sum := func(zone string) uint64 {
		var n uint64
		if e := f.ch.QueryRow(f.ctx, "SELECT sum(requests_200+requests_204+requests_4xx+requests_5xx) FROM "+f.db+".publisher_zone_hourly FINAL WHERE is_meta=0 AND zone_id=?", zone).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if sum("ok") != 1 || sum("mixed") != 2 || sum("empty") != 1 || sum("error") != 1 {
		t.Fatal("duplicate or missing events")
	}
	insert("late", "ok", "20 MINUTE", 204)
	refresh()
	if sum("ok") != 2 {
		t.Fatal("late event lost")
	}
	f.exec("ALTER TABLE " + f.db + ".ortb DELETE WHERE zone_id='sealed' SETTINGS mutations_sync=2")
	insert("f", "sealed", "55 MINUTE", 200)
	refresh()
	if sum("sealed") != 1 {
		t.Fatal("sealed replay counted again")
	}
	f.exec("OPTIMIZE TABLE " + f.db + ".publisher_zone_hourly FINAL")
	if sum("ok") != 2 {
		t.Fatal("merge changed totals")
	}
	// An existing Loader restart does no financial/schema DDL.
	before := f.scalar("SELECT sum(ssp_revenue_views) FROM " + f.db + ".agg_stats")
	if e := EnsureLoaderSchema(f.ctx, f.ch, f.db); e != nil {
		t.Fatal(e)
	}
	if f.money() != before {
		t.Fatal("restart changed money")
	}
	if e := f.ch.QueryRow(f.ctx, "SELECT count() FROM system.tables WHERE database=? AND name IN ('publisher_requests','publisher_requests_logical','mv_ortb_to_publisher_requests')", f.db).Scan(&total); e != nil || total != 0 {
		t.Fatal("legacy diagnostics recreated", total, e)
	}
	// Verify UTC second boundaries on both sides of a calendar hour.
	end := time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC)
	start := end.Add(-time.Hour)
	f.exec("INSERT INTO " + f.db + ".publisher_zone_hourly(event_hour,event_bucket,pub_user_id,zone_id,seconds,expires_at) SELECT toDateTime('2026-10-01 12:00:00','UTC'),toDateTime('2026-10-01 12:00:00','UTC'),'boundary','boundary',[(toDateTime('2026-10-01 11:29:59','UTC'),1,0,0,0,0),(toDateTime('2026-10-01 11:30:00','UTC'),1,0,0,0,0),(toDateTime('2026-10-01 12:29:59','UTC'),0,1,0,0,0),(toDateTime('2026-10-01 12:30:00','UTC'),1,0,0,0,0)],now()+INTERVAL 7 DAY")
	if e := f.ch.QueryRow(f.ctx, "SELECT sum(s.n200+s.n204) FROM "+f.db+".publisher_zone_hourly FINAL ARRAY JOIN seconds AS s WHERE pub_user_id='boundary' AND s.second>=? AND s.second<?", start, end).Scan(&total); e != nil || total != 2 {
		t.Fatal("rolling UTC boundary", total, e)
	}
	f.exec("INSERT INTO " + f.db + ".publisher_zone_hourly(event_hour,event_bucket,pub_user_id,expires_at) SELECT now()-INTERVAL 8 DAY,now()-INTERVAL 8 DAY,'expired',now()-INTERVAL 1 DAY")
	f.exec("ALTER TABLE " + f.db + ".publisher_zone_hourly MATERIALIZE TTL SETTINGS mutations_sync=2")
	if e := f.ch.QueryRow(f.ctx, "SELECT count() FROM "+f.db+".publisher_zone_hourly WHERE pub_user_id='expired'").Scan(&total); e != nil || total != 0 {
		t.Fatal("TTL did not remove expired data", total, e)
	}
	// Model an existing legacy installation: preserve both its data and read
	// view while replacing only the old writer.
	f.exec("CREATE TABLE " + f.db + ".publisher_requests(n UInt8) ENGINE=MergeTree ORDER BY n")
	f.exec("INSERT INTO " + f.db + ".publisher_requests VALUES(7)")
	f.exec("CREATE VIEW " + f.db + ".publisher_requests_logical AS SELECT * FROM " + f.db + ".publisher_requests")
	f.exec("CREATE MATERIALIZED VIEW " + f.db + ".mv_ortb_to_publisher_requests TO " + f.db + ".publisher_requests AS SELECT toUInt8(1) AS n FROM " + f.db + ".ortb")
	// A failed APPEND can write a heartbeat before failing another block.
	// Its metadata MUST acknowledge the previous success, never the failed run.
	var beforeBeat, afterBeat time.Time
	if e := f.ch.QueryRow(f.ctx, "SELECT max(refreshed_at) FROM "+f.db+".publisher_zone_hourly FINAL WHERE is_meta=1").Scan(&beforeBeat); e != nil {
		t.Fatal(e)
	}
	f.exec("ALTER TABLE " + f.db + ".publisher_zone_hourly ADD CONSTRAINT reject_diagnostic_test CHECK is_meta!=0 OR pub_user_id!='publisher'")
	wait, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	err := ApplyPublisherZoneDiagnostics(wait, f.ch, f.db)
	cancel()
	f.exec("SYSTEM STOP VIEW " + f.db + ".mv_publisher_zone_hourly")
	if err == nil {
		t.Fatal("migration accepted injected refresh failure")
	}
	if e := f.ch.QueryRow(f.ctx, "SELECT count() FROM system.tables WHERE database=? AND name='mv_ortb_to_publisher_requests'", f.db).Scan(&total); e != nil || total != 1 {
		t.Fatal("failed migration stopped legacy writer", total, e)
	}

	if e := f.ch.QueryRow(f.ctx, "SELECT max(refreshed_at) FROM "+f.db+".publisher_zone_hourly FINAL WHERE is_meta=1").Scan(&afterBeat); e != nil {
		t.Fatal(e)
	}
	if !beforeBeat.Equal(afterBeat) {
		t.Fatal("failed APPEND faked successful coverage", beforeBeat, afterBeat)
	}
	f.exec("ALTER TABLE " + f.db + ".publisher_zone_hourly DROP CONSTRAINT reject_diagnostic_test")
	// Migration must leave the existing financial view definitions unchanged.
	var definition string
	if e := f.ch.QueryRow(f.ctx, "SELECT create_table_query FROM system.tables WHERE database=? AND name='mv_fact_impressions_to_agg_stats'", f.db).Scan(&definition); e != nil {
		t.Fatal(e)
	}
	if e := ApplyPublisherZoneDiagnostics(context.Background(), f.ch, f.db); e != nil {
		t.Fatal(e)
	}
	if e := f.ch.QueryRow(f.ctx, "SELECT count() FROM system.tables WHERE database=? AND name='mv_ortb_to_publisher_requests'", f.db).Scan(&total); e != nil || total != 0 {
		t.Fatal("legacy writer still exists", total, e)
	}
	if e := f.ch.QueryRow(f.ctx, "SELECT sum(n) FROM "+f.db+".publisher_requests_logical").Scan(&total); e != nil || total != 7 {
		t.Fatal("legacy data/view removed", total, e)
	}
	var after string
	f.ch.QueryRow(f.ctx, "SELECT create_table_query FROM system.tables WHERE database=? AND name='mv_fact_impressions_to_agg_stats'", f.db).Scan(&after)
	if definition == "" || definition != after {
		t.Fatal("financial writer changed")
	}
}

func TestPublisherZoneDDLContract(t *testing.T) {
	q := PublisherZoneSelect("ads", 45)
	if !strings.Contains(q, "argMin(tuple(") || !strings.Contains(q, "INTERVAL 45 MINUTE") || strings.Contains(q, "ortb_logical") || strings.Contains(q, "publisher_requests") {
		t.Fatal(q)
	}
}
