package clickhouse_loader

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

var diagnosticDatabase = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const publisherZoneTableDDL = `CREATE TABLE IF NOT EXISTS {db}.publisher_zone_hourly (
 event_hour DateTime('UTC'), event_bucket DateTime('UTC'),
 pub_user_id String, ad_code_id String, zone_id String, site_id String,
 hostname String, format LowCardinality(String), geo LowCardinality(String),
 device LowCardinality(String), os LowCardinality(String), browser LowCardinality(String),
 requests_200 UInt64, requests_204 UInt64, requests_4xx UInt64, requests_5xx UInt64, requests_other UInt64,
 last_request_at DateTime64(3,'UTC'),
 seconds Array(Tuple(second DateTime('UTC'), n200 UInt64, n204 UInt64, n4xx UInt64, n5xx UInt64, other UInt64)),
 is_meta UInt8, refreshed_at DateTime64(3,'UTC'), revision UInt64, expires_at DateTime('UTC')
) ENGINE=ReplacingMergeTree(revision)
PARTITION BY toDate(event_bucket)
ORDER BY (is_meta,pub_user_id,ad_code_id,zone_id,site_id,event_hour,event_bucket,hostname,format,geo,device,os,browser)
TTL expires_at DELETE`

// Heartbeats acknowledge ONLY the previous successful refresh reported by
// system.view_refreshes. A heartbeat emitted by the current APPEND cannot prove
// its own completion: INSERT SELECT may fail after writing an earlier block.
// Snapshots replace the SAME five-minute key. Consumers must use FINAL before
// SUM, even before merges. Closed slots are retained instead of recomputing them
// from the one-hour ORTB source. Seconds provide an exact sliding hour, not two
// calendar-hour totals. No additional INSERT-triggered MV is installed.
func PublisherZoneSelect(database string, lookback int) string {
	q := `WITH now64(3,'UTC') AS stamp,
 toStartOfFiveMinutes(stamp-INTERVAL {minutes} MINUTE) AS cutoff,
 logical AS (
 SELECT argMin(tuple(event_time,ad_code_id,zone_id,ifNull(site_id,''),ifNull(site_domain,''),format,
 ifNull(geo,''),ifNull(device,''),ifNull(os,''),ifNull(browser,''),code),tuple(created_at,uuid)) AS e, pub_user_id
 FROM {db}.ortb PREWHERE created_at>=stamp-INTERVAL 65 MINUTE
 WHERE pub_user_id!='' AND ad_code_id!='' AND zone_id!='' AND ifNull(site_id,'')!=''
 AND event_time>=cutoff AND event_time<stamp
 GROUP BY pub_user_id,if(logical_event_id='',toString(uuid),logical_event_id)
 ), per_second AS (
 SELECT pub_user_id,e.2 AS ad_code_id,e.3 AS zone_id,e.4 AS site_id,e.5 AS hostname,e.6 AS format,
 e.7 AS geo,e.8 AS device,e.9 AS os,e.10 AS browser,toDateTime(e.1,'UTC') AS second,
 countIf(e.11=200) AS n200,countIf(e.11=204) AS n204,countIf(e.11>=400 AND e.11<500) AS n4xx,
 countIf(e.11>=500 AND e.11<600) AS n5xx,countIf(e.11 NOT IN (200,204) AND (e.11<400 OR e.11>=600)) AS other,
 max(e.1) AS last_seen FROM logical
 GROUP BY pub_user_id,ad_code_id,zone_id,site_id,hostname,format,geo,device,os,browser,second
 )
 SELECT toStartOfHour(second) AS event_hour,toStartOfFiveMinutes(second) AS event_bucket,
 pub_user_id,ad_code_id,zone_id,site_id,hostname,format,geo,device,os,browser,
 sum(n200) AS requests_200,sum(n204) AS requests_204,sum(n4xx) AS requests_4xx,sum(n5xx) AS requests_5xx,sum(other) AS requests_other,max(last_seen) AS last_request_at,
 groupArray(tuple(second,n200,n204,n4xx,n5xx,other)) AS seconds,toUInt8(0) AS is_meta,stamp AS refreshed_at,toUInt64(toUnixTimestamp64Milli(stamp)) AS revision,event_bucket+INTERVAL 7 DAY AS expires_at
 FROM per_second GROUP BY event_hour,event_bucket,pub_user_id,ad_code_id,zone_id,site_id,hostname,format,geo,device,os,browser
 UNION ALL
 SELECT toStartOfHour(last_success_time),toStartOfFiveMinutes(last_success_time),'','','','','','','','','','',
 toUInt64(0),toUInt64(0),toUInt64(0),toUInt64(0),toUInt64(0),toDateTime64(last_success_time,3,'UTC'),[],toUInt8(1),toDateTime64(last_success_time,3,'UTC'),
 toUInt64(toUnixTimestamp64Milli(stamp)),toDateTime(last_success_time,'UTC')+INTERVAL 7 DAY
 FROM system.view_refreshes WHERE database='{db}' AND view='mv_publisher_zone_hourly' AND last_success_time IS NOT NULL
 SETTINGS max_threads=2,max_memory_usage=1073741824,max_bytes_before_external_group_by=134217728`
	return strings.NewReplacer("{db}", database, "{minutes}", fmt.Sprint(lookback)).Replace(q)
}

// Explicit, diagnostics-only migration for an existing installation. It never
// calls the full V4 DDL or touches financial writers, aggregates or facts.
func ApplyPublisherZoneDiagnostics(ctx context.Context, ch clickhouse.Conn, database string) error {
	if !diagnosticDatabase.MatchString(database) {
		return fmt.Errorf("invalid diagnostics database")
	}
	if err := preparePublisherZoneSource(ctx, ch, database); err != nil {
		return err
	}
	if err := ch.Exec(ctx, strings.ReplaceAll(publisherZoneTableDDL, "{db}", database)); err != nil {
		return err
	}
	var boot uint64
	if err := ch.QueryRow(ctx, "SELECT count() FROM "+database+".publisher_zone_hourly FINAL WHERE is_meta>0 AND expires_at>now()").Scan(&boot); err != nil {
		return err
	}
	if boot == 0 {
		if err := ch.Exec(ctx, "INSERT INTO "+database+".publisher_zone_hourly "+PublisherZoneSelect(database, 60)); err != nil {
			return err
		}
		// A bootstrap proves coverage of the available source hour only. A stale
		// refresh chain subsequently makes the API fail closed, never fake zeros.
		if err := ch.Exec(ctx, "INSERT INTO "+database+".publisher_zone_hourly(event_hour,event_bucket,is_meta,refreshed_at,revision,expires_at) SELECT toDateTime(0,'UTC'),toDateTime(0,'UTC'),2,now64(3,'UTC'),toUInt64(toUnixTimestamp64Milli(now64(3,'UTC'))),now()+INTERVAL 7 DAY"); err != nil {
			return err
		}
	}
	var existed uint64
	if err := ch.QueryRow(ctx, "SELECT count() FROM system.tables WHERE database=? AND name='mv_publisher_zone_hourly'", database).Scan(&existed); err != nil {
		return err
	}
	if err := ch.Exec(ctx, "CREATE MATERIALIZED VIEW IF NOT EXISTS "+database+".mv_publisher_zone_hourly REFRESH EVERY 5 MINUTE APPEND TO "+database+".publisher_zone_hourly AS "+PublisherZoneSelect(database, 45)); err != nil {
		return err
	}
	// CREATE IF NOT EXISTS does not upgrade an already deployed refresh query.
	// Only this diagnostics view is modified; its target, schedule and data remain.
	if existed > 0 {
		if err := ch.Exec(ctx, "SYSTEM STOP VIEW "+database+".mv_publisher_zone_hourly"); err != nil {
			return err
		}
		defer func() {
			resume, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := ch.Exec(resume, "SYSTEM START VIEW "+database+".mv_publisher_zone_hourly"); err != nil {
				log.Printf("Publisher diagnostics resume after query upgrade: %v", err)
			}
		}()
		if err := ch.Exec(ctx, "SYSTEM WAIT VIEW "+database+".mv_publisher_zone_hourly"); err != nil {
			var previous *clickhouse.Exception
			// A completed FAILED run is not an in-flight writer. Permit repairing
			// its query; the NEW refresh below must still succeed before cutover.
			if !errors.As(err, &previous) || previous.Code != 730 {
				return err
			}
		}
		if err := ch.Exec(ctx, "ALTER TABLE "+database+".mv_publisher_zone_hourly MODIFY QUERY "+PublisherZoneSelect(database, 45)); err != nil {
			return fmt.Errorf("upgrade diagnostics refresh query: %w", err)
		}
	}
	for _, action := range []string{"START", "REFRESH", "WAIT"} {
		if err := ch.Exec(ctx, "SYSTEM "+action+" VIEW "+database+".mv_publisher_zone_hourly"); err != nil {
			return err
		}
	}
	// WAIT completion alone is insufficient after a failed asynchronous run.
	// Do not cut over the legacy writer unless ClickHouse confirms recent success.
	var exception string
	var succeeded uint8
	if err := ch.QueryRow(ctx, "SELECT exception,toUInt8(ifNull(last_success_time>=now()-INTERVAL 10 MINUTE,0)) FROM system.view_refreshes WHERE database=? AND view='mv_publisher_zone_hourly'", database).Scan(&exception, &succeeded); err != nil {
		return err
	}
	if exception != "" || succeeded != 1 {
		return fmt.Errorf("publisher diagnostics refresh not confirmed: %s", exception)
	}
	// Keep the legacy data and read view for rollback; only stop its writer after
	// the replacement has been successfully refreshed.
	return ch.Exec(ctx, "DROP VIEW IF EXISTS "+database+".mv_ortb_to_publisher_requests SYNC")
}

// Ordinary restarts of an existing Loader must not run the full financial
// cutover DDL. A missing diagnostics migration cannot stop existing ingestion.
func EnsureLoaderSchema(ctx context.Context, ch clickhouse.Conn, database string) error {
	if !diagnosticDatabase.MatchString(database) {
		return fmt.Errorf("invalid database")
	}
	var exists uint64
	if err := ch.QueryRow(ctx, "SELECT count() FROM system.tables WHERE database=? AND name='ortb'", database).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return CreateDB(ctx, ch, database)
	}
	var ready uint64
	if err := ch.QueryRow(ctx, "SELECT count() FROM system.tables WHERE database=? AND name IN ('publisher_zone_hourly','mv_publisher_zone_hourly')", database).Scan(&ready); err != nil {
		return err
	}
	if ready != 2 {
		log.Print("Publisher diagnostics migration pending; run --publisher-zone-diagnostics-only separately")
	}
	return nil
}
