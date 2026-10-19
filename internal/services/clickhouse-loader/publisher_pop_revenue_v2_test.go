package clickhouse_loader

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
)

type popRevenueFixture struct {
	t   *testing.T
	ctx context.Context
	ch  clickhouse.Conn
	db  string
}

func newPOPRevenueFixture(t *testing.T) *popRevenueFixture {
	t.Helper()
	addr := os.Getenv("PUBLISHER_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("PUBLISHER_TEST_CLICKHOUSE_ADDR unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{addr}, Auth: clickhouse.Auth{Username: "default"}})
	if e != nil {
		t.Fatal(e)
	}
	f := &popRevenueFixture{t: t, ctx: ctx, ch: ch, db: "pub_v2_" + strings.ReplaceAll(uuid.NewString(), "-", "")}
	t.Cleanup(func() { ch.Exec(context.Background(), "DROP DATABASE IF EXISTS "+f.db+" SYNC"); ch.Close() })
	if e = CreateDB(ctx, ch, f.db); e != nil {
		t.Fatal(e)
	}
	for _, view := range []string{"mv_publisher_pop_revenue_checkpoint", "mv_publisher_pop_revenue_to_agg_stats"} {
		f.exec("SYSTEM STOP VIEW " + f.db + "." + view)
	}
	return f
}
func (f *popRevenueFixture) exec(query string, args ...any) {
	f.t.Helper()
	if e := f.ch.Exec(f.ctx, query, args...); e != nil {
		f.t.Fatal(e, query)
	}
}
func (f *popRevenueFixture) refresh(view string) {
	f.t.Helper()
	for _, action := range []string{"START", "REFRESH", "WAIT", "STOP"} {
		f.exec("SYSTEM " + action + " VIEW " + f.db + "." + view)
	}
}
func (f *popRevenueFixture) settle() {
	f.t.Helper()
	f.refresh("mv_publisher_pop_revenue_to_agg_stats")
	f.refresh("mv_publisher_pop_revenue_checkpoint")
}
func (f *popRevenueFixture) scalar(q string) float64 {
	f.t.Helper()
	var n float64
	if e := f.ch.QueryRow(f.ctx, q).Scan(&n); e != nil {
		f.t.Fatal(e, q)
	}
	return n
}
func (f *popRevenueFixture) dirty() float64 {
	return f.scalar("SELECT toFloat64(count()) FROM " + f.db + ".publisher_pop_revenue_dirty")
}
func (f *popRevenueFixture) money() float64 {
	return f.scalar("SELECT sum(ssp_revenue_views) FROM " + f.db + ".agg_stats WHERE pub_user_id='publisher'")
}
func (f *popRevenueFixture) click(id string, price float64, date string) {
	f.exec("INSERT INTO "+f.db+".fact_clicks(uuid,clicks_uuid,event_time,event_time_clicks,event_date,event_hour,format,pub_user_id,ad_code_id,zone_id,site_id,win_final_price) VALUES(?,?,toDateTime64(?,3),toDateTime64(?,3),toDate(?),toDateTime(?),'POP','publisher','code','zone','site',?)", uuid.NewString(), id, date, date, date, date, price)
}
func TestPublisherPOPRevenueDirtyCheckpointsCrashAndLateReplay(t *testing.T) {
	f := newPOPRevenueFixture(t)
	id, _ := uuid.NewV7()
	old := "2025-01-01 00:00:00"
	f.click(id.String(), 2, old)
	f.settle()
	if f.dirty() != 0 || math.Abs(f.money()-.002) > 1e-10 {
		t.Fatal("initial settlement", f.money(), f.dirty())
	}
	// Acknowledged bucket gets dirty on a replay, yet its entitlement is unchanged.
	// Even a changed event_hour cannot place the same UUID in another dedup bucket.
	f.click(id.String(), 2, "2025-01-02 00:00:00")
	f.settle()
	if f.dirty() != 0 || math.Abs(f.money()-.002) > 1e-10 {
		t.Fatal("replay paid twice")
	}
	// Deliberately checkpoint BEFORE paying: it must refuse to acknowledge work.
	late, _ := uuid.NewV7()
	f.click(late.String(), 3, old)
	f.refresh("mv_publisher_pop_revenue_checkpoint")
	if f.dirty() == 0 {
		t.Fatal("unpaid event checkpointed")
	}
	// Simulate committed money with lost ACK, including a partial amount. A
	// restart/retry computes the remaining delta, never blindly re-adds income.
	f.exec("INSERT INTO "+f.db+".agg_stats(event_date,event_hour,format,pub_user_id,ad_code_id,zone_id,site_id,pop_revenue_bucket,ssp_revenue_views) SELECT toDate(?),toDateTime(?),'POP','publisher','code','zone','site',bucket,.001 FROM "+f.db+".publisher_pop_revenue_events_v2 WHERE clicks_uuid=? LIMIT 1", old, old, late.String())
	f.refresh("mv_publisher_pop_revenue_to_agg_stats")
	f.refresh("mv_publisher_pop_revenue_to_agg_stats")
	if math.Abs(f.money()-.005) > 1e-10 {
		t.Fatal("unknown commit duplicated", f.money())
	}
	f.refresh("mv_publisher_pop_revenue_checkpoint")
	if f.dirty() != 0 {
		t.Fatal("paid bucket not checkpointed")
	}
	f.exec("OPTIMIZE TABLE " + f.db + ".publisher_pop_revenue_events_v2 FINAL")
	f.exec("OPTIMIZE TABLE " + f.db + ".agg_stats FINAL")
	f.exec("OPTIMIZE TABLE " + f.db + ".publisher_pop_revenue_checkpoints FINAL")
	if f.dirty() != 0 {
		t.Fatal("merging parts changed generation")
	}
	if e := CreateDB(f.ctx, f.ch, f.db); e != nil {
		t.Fatal("loader restart", e)
	}
	for _, v := range []string{"mv_publisher_pop_revenue_checkpoint", "mv_publisher_pop_revenue_to_agg_stats"} {
		f.exec("SYSTEM STOP VIEW " + f.db + "." + v)
	}
	if f.dirty() != 0 || math.Abs(f.money()-.005) > 1e-10 {
		t.Fatal("restart replayed history")
	}
	// Kafka and ClickHouse blocks may deliver the same ID concurrently.
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() {
			errors <- f.ch.Exec(f.ctx, "INSERT INTO "+f.db+".fact_clicks(uuid,clicks_uuid,event_date,event_hour,format,pub_user_id,win_final_price) VALUES(?,?,today(),toStartOfHour(now()),'POP','publisher',3)", uuid.NewString(), late.String())
		}()
	}
	for i := 0; i < 12; i++ {
		if e := <-errors; e != nil {
			t.Fatal(e)
		}
	}
	f.settle()
	if f.dirty() != 0 || math.Abs(f.money()-.005) > 1e-10 {
		t.Fatal("concurrent redelivery", f.money())
	}
	t.Log("late events, conflicting replay hour, partial commit, lost ACK, merges, concurrent blocks, loader restart: one entitlement per UUID")
}

// Synthetic settled history is seeded outside the measurement. Only a fixed
// active bucket changes between measurements; this is the operating cost once
// historical buckets have been paid, not a benchmark of first-time backfill.
func TestPublisherPOPRevenueHistoryScaling(t *testing.T) {
	if os.Getenv("PUBLISHER_TEST_PERFORMANCE") != "true" {
		t.Skip("set PUBLISHER_TEST_PERFORMANCE=true for 3M-event integration benchmark")
	}
	f := newPOPRevenueFixture(t)
	dims := "win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id"
	tuple := "tuple('','','',toDate('2025-01-01'),'','','',toDateTime('2025-01-01 00:00:00','UTC'),'','','site','POP','','publisher','code','zone',toFloat64(2))"
	idExpr := `toUUID(concat(substring(mshex,1,8),'-',substring(mshex,9,4),'-7000-8000-',leftPad(hex(number),12,'0')))`
	last := 0
	for _, n := range []int{100000, 1000000, 3000000} {
		f.exec(fmt.Sprintf("INSERT INTO %s.publisher_pop_revenue_events_v2(clicks_uuid,event,seen) SELECT %s,initializeAggregation('argMinState',%s,toDateTime64('2025-01-01 00:00:00',3)),toUInt64(1) FROM (SELECT number,leftPad(hex(toUInt64(1735689600000)+intDiv(number,10000)*3600000),12,'0') AS mshex FROM numbers(%d,%d))", f.db, idExpr, tuple, last, n-last))
		// The same settled aggregates and checkpoints a preceding sequence of normal
		// hourly runs would have produced. Each new cold bucket has exactly 10k UUIDs.
		f.exec(fmt.Sprintf("INSERT INTO %s.agg_stats(%s,pop_revenue_bucket,ssp_revenue_views) SELECT '','','',toDate('2025-01-01'),'','','',toDateTime('2025-01-01 00:00:00','UTC'),'','','site','POP','','publisher','code','zone',1000000+intDiv(toUInt64(1735689600000),3600000)+intDiv(number,10000),count()*0.002 FROM numbers(%d,%d) GROUP BY intDiv(number,10000)", f.db, dims, last, n-last))
		f.exec("INSERT INTO " + f.db + ".publisher_pop_revenue_checkpoints SELECT bucket,sum(seen) FROM " + f.db + ".publisher_pop_revenue_events_v2 GROUP BY bucket")
		// Add a small fixed-size genuinely dirty bucket with independent UUIDv7 IDs.
		f.exec(fmt.Sprintf("INSERT INTO %s.publisher_pop_revenue_events_v2(clicks_uuid,event,seen) SELECT toUUID(concat('01c89ce4-0000-7000-8000-',leftPad(hex(number),12,'0'))),initializeAggregation('argMinState',%s,toDateTime64('2025-01-01 00:00:00',3)),toUInt64(1) FROM numbers(10000)", f.db, tuple))
		var readRows, readBytes atomic.Uint64
		qid := "pub_perf_" + uuid.NewString()
		measured := clickhouse.Context(f.ctx, clickhouse.WithQueryID(qid), clickhouse.WithProgress(func(p *clickhouse.Progress) { readRows.Add(p.Rows); readBytes.Add(p.Bytes) }))
		begin := time.Now()
		rows, e := f.ch.Query(measured, "SELECT * FROM "+f.db+".publisher_pop_revenue_delta SETTINGS log_queries=1")
		if e != nil {
			t.Fatal(e)
		}
		for rows.Next() {
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		rows.Close()
		elapsed := time.Since(begin)
		// Generation projections must actually be chosen, not merely exist in DDL.
		if readRows.Load() > uint64(n/2) {
			t.Fatalf("historical scan: history=%d read_rows=%d", n, readRows.Load())
		}
		memory := float64(-1)
		if e = f.ch.Exec(f.ctx, "SYSTEM FLUSH LOGS"); e == nil {
			_ = f.ch.QueryRow(f.ctx, "SELECT toFloat64(memory_usage) FROM system.query_log WHERE query_id=? AND type='QueryFinish' ORDER BY event_time_microseconds DESC LIMIT 1", qid).Scan(&memory)
		}
		f.settle()
		if f.dirty() != 0 {
			t.Fatal("dirty bucket not settled")
		}
		// The hot UUIDs were reinserted at every stage, and must still pay only .002 each.
		if math.Abs(f.money()-(float64(n)*.002+20)) > 1e-7 {
			t.Fatal("scaled duplicate income", f.money(), n)
		}
		t.Logf("history=%d hot_UUIDs=10000 query=%s read_rows=%d read_bytes=%d peak_query_memory_bytes=%.0f", n, elapsed, readRows.Load(), readBytes.Load(), memory)

		// Compare the old all-history canonical scan on exactly the same data.
		// Disk spilling bounds the baseline's memory instead of making a 3M-row
		// GROUP BY fail under the disposable server's 2 GB query limit.
		var oldRows atomic.Uint64
		baselineID := "pub_baseline_" + uuid.NewString()
		baselineCtx := clickhouse.Context(f.ctx, clickhouse.WithQueryID(baselineID), clickhouse.WithProgress(func(p *clickhouse.Progress) { oldRows.Add(p.Rows) }))
		start := time.Now()
		var oldMoney float64
		if e = f.ch.QueryRow(baselineCtx, "SELECT sum(e.17/1000) FROM (SELECT clicks_uuid,argMinMerge(event) AS e FROM "+f.db+".publisher_pop_revenue_events_v2 GROUP BY clicks_uuid) SETTINGS max_bytes_before_external_group_by=100000000,log_queries=1").Scan(&oldMoney); e != nil {
			t.Fatal(e)
		}
		oldElapsed := time.Since(start)
		oldMemory := float64(-1)
		if e = f.ch.Exec(f.ctx, "SYSTEM FLUSH LOGS"); e == nil {
			_ = f.ch.QueryRow(f.ctx, "SELECT toFloat64(memory_usage) FROM system.query_log WHERE query_id=? AND type='QueryFinish' ORDER BY event_time_microseconds DESC LIMIT 1", baselineID).Scan(&oldMemory)
		}
		if math.Abs(oldMoney-f.money()) > 1e-7 {
			t.Fatal("baseline/new entitlement differs", oldMoney, f.money())
		}
		var noChangeRows atomic.Uint64
		quiet := clickhouse.Context(f.ctx, clickhouse.WithProgress(func(p *clickhouse.Progress) { noChangeRows.Add(p.Rows) }))
		var noChange uint64
		if e = f.ch.QueryRow(quiet, "SELECT count() FROM "+f.db+".publisher_pop_revenue_delta").Scan(&noChange); e != nil || noChange != 0 {
			t.Fatal(noChange, e)
		}
		t.Logf("old_full_scan history=%d elapsed=%s read_rows=%d peak_query_memory_bytes=%.0f; new_no_change_read_rows=%d", n, oldElapsed, oldRows.Load(), oldMemory, noChangeRows.Load())

		var diskBytes, physicalRows uint64
		if e = f.ch.QueryRow(f.ctx, "SELECT sum(bytes_on_disk),sum(rows) FROM system.parts WHERE database=? AND table='publisher_pop_revenue_events_v2' AND active", f.db).Scan(&diskBytes, &physicalRows); e != nil {
			t.Fatal(e)
		}
		t.Logf("registry history=%d physical_rows=%d disk_bytes=%d", n, physicalRows, diskBytes)
		last = n
	}
	before := f.money()
	begin := time.Now()
	f.exec("OPTIMIZE TABLE " + f.db + ".publisher_pop_revenue_events_v2 FINAL")
	if f.dirty() != 0 || math.Abs(f.money()-before) > 1e-7 {
		t.Fatal("3M-part merge changed financial state", f.money(), before)
	}
	t.Logf("3M registry FINAL merge=%s, checkpoint generations and income unchanged", time.Since(begin))
}

func TestPublisherPOPRevenueV1CutoverAndFailedMVRepair(t *testing.T) {
	f := newPOPRevenueFixture(t)
	id := uuid.NewString()
	f.click(id, 2, "2025-01-01 00:00:00")
	f.exec("INSERT INTO " + f.db + ".publisher_pop_revenue_events SELECT clicks_uuid,event FROM " + f.db + ".publisher_pop_revenue_events_v2")
	f.exec("TRUNCATE TABLE " + f.db + ".publisher_pop_revenue_events_v2")
	f.exec("ALTER TABLE " + f.db + ".fact_clicks DELETE WHERE 1 SETTINGS mutations_sync=2")
	f.exec("TRUNCATE TABLE " + f.db + ".publisher_pop_revenue_migrations")
	f.exec("INSERT INTO " + f.db + ".agg_stats(event_date,event_hour,format,pub_user_id,ssp_revenue_views,spend_views_table) VALUES(toDate('2025-01-01'),toDateTime('2025-01-01 00:00:00'),'POP','publisher',.002,7)")
	if e := CreateDB(f.ctx, f.ch, f.db); e != nil {
		t.Fatal(e)
	}
	f.settle()
	if math.Abs(f.money()-.002) > 1e-10 || math.Abs(f.scalar("SELECT sum(spend_views_table) FROM "+f.db+".agg_stats")-7) > 1e-10 {
		t.Fatal("v1 cutover lost money or changed advertiser spend", f.money())
	}
	// Model a source row committed without its MV side effect after an ingestion
	// failure. Explicit repair restores it; running repair again is harmless.
	f.exec("DETACH TABLE " + f.db + ".mv_fact_clicks_to_publisher_pop_revenue")
	missing, _ := uuid.NewV7()
	f.click(missing.String(), 3, "2025-01-01 00:00:00")
	f.exec("ATTACH TABLE " + f.db + ".mv_fact_clicks_to_publisher_pop_revenue")
	for i := 0; i < 2; i++ {
		if e := RepairPublisherPOPRevenueRegistry(f.ctx, f.ch, f.db); e != nil {
			t.Fatal(e)
		}
		f.settle()
		if math.Abs(f.money()-.005) > 1e-10 {
			t.Fatal("repair duplicated/missed entitlement", f.money())
		}
	}
}
func TestPublisherPOPFactProducerResumesAfterRegistry(t *testing.T) {
	source := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_clicks_to_fact")
	consumer := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW {db}.mv_fact_clicks_to_publisher_pop_revenue")
	if source < consumer {
		t.Fatal("restart can insert facts before their registry consumer exists")
	}
	if strings.Index(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_clicks_to_fact") > strings.Index(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_fact_clicks_to_publisher_pop_revenue") {
		t.Fatal("registry detached while source is running")
	}
}

func TestPublisherPOPRevenueFreezesDirtySetDuringCheckpoint(t *testing.T) {
	f := newPOPRevenueFixture(t)
	id, _ := uuid.NewV7()
	f.click(id.String(), 2, time.Now().UTC().Format("2006-01-02 15:04:05"))
	f.settle()
	stop, finished := make(chan struct{}), make(chan error, 1)
	// Every new input is a duplicate, so acknowledging its generation is valid.
	// Force checkpoints to change while the money query is reading both sides.
	go func() {
		for {
			select {
			case <-stop:
				finished <- nil
				return
			default:
			}
			if e := f.ch.Exec(f.ctx, "INSERT INTO "+f.db+".publisher_pop_revenue_checkpoints SELECT bucket,sum(seen) FROM "+f.db+".publisher_pop_revenue_events_v2 GROUP BY bucket"); e != nil {
				finished <- e
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	defer func() {
		close(stop)
		if e := <-finished; e != nil {
			t.Error(e)
		}
	}()
	for i := 0; i < 20; i++ {
		f.click(id.String(), 2, time.Now().UTC().Format("2006-01-02 15:04:05"))
		f.refresh("mv_publisher_pop_revenue_to_agg_stats")
		if math.Abs(f.money()-.002) > 1e-10 {
			t.Fatal("checkpoint race duplicated revenue", f.money())
		}
	}
}
