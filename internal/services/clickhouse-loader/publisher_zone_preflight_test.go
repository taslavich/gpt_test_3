package clickhouse_loader

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	events "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
)

func publisherPreflightFixture(t *testing.T) (context.Context, clickhouse.Conn, string) {
	t.Helper()
	addr := os.Getenv("PUBLISHER_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("PUBLISHER_TEST_CLICKHOUSE_ADDR unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{addr}, Auth: clickhouse.Auth{Username: "default"}})
	if e != nil {
		t.Fatal(e)
	}
	db := "zone_preflight_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if e = ch.Exec(ctx, "CREATE DATABASE "+db); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { ch.Exec(context.Background(), "DROP DATABASE IF EXISTS "+db+" SYNC"); ch.Close() })
	// The actual raw ORTB DDL, without running ANY financial cutover.
	for _, statement := range splitClickHouseStatements(ddlTemplate) {
		if strings.Contains(statement, "CREATE TABLE IF NOT EXISTS {db}.ortb\n") {
			if e = ch.Exec(ctx, strings.ReplaceAll(statement, "{db}", db)); e != nil {
				t.Fatal(e)
			}
			return ctx, ch, db
		}
	}
	t.Fatal("raw ORTB DDL not found")
	return nil, nil, ""
}

func TestPublisherPreflightLegacyMigration(t *testing.T) {
	ctx, ch, db := publisherPreflightFixture(t)
	must := func(q string) {
		t.Helper()
		if e := ch.Exec(ctx, q); e != nil {
			t.Fatal(e, q)
		}
	}
	for _, c := range publisherSourceColumns {
		if c.name != "uuid" && c.name != "event_time" && c.name != "created_at" {
			must("ALTER TABLE " + db + ".ortb DROP COLUMN " + c.name)
		}
	}
	// A sentinel represents unrelated production objects and must survive.
	must("CREATE TABLE " + db + ".financial_sentinel(n UInt64) ENGINE=MergeTree ORDER BY n")
	must("INSERT INTO " + db + ".financial_sentinel VALUES(7)")
	for i := 0; i < 2; i++ {
		if e := ApplyPublisherZoneDiagnostics(ctx, ch, db); e != nil {
			t.Fatal(e)
		}
	}
	// Model an already deployed view with the earlier 512 MiB budget.
	oldQuery := strings.Replace(PublisherZoneSelect(db, 45), "max_memory_usage=1073741824", "max_memory_usage=536870912", 1)
	must("SYSTEM STOP VIEW " + db + ".mv_publisher_zone_hourly")
	must("ALTER TABLE " + db + ".mv_publisher_zone_hourly MODIFY QUERY " + oldQuery)
	if e := ApplyPublisherZoneDiagnostics(ctx, ch, db); e != nil {
		t.Fatal(e)
	}
	var definition string
	if e := ch.QueryRow(ctx, "SELECT create_table_query FROM system.tables WHERE database=? AND name='mv_publisher_zone_hourly'", db).Scan(&definition); e != nil || (!strings.Contains(definition, "1073741824") || !strings.Contains(diagnosticSQL(definition), "refreshevery5minuteappendto")) {
		t.Fatal("existing refresh query not upgraded", definition, e)
	}
	// Exercise actual Kafka protobuf decode -> Loader Append, not SQL DEFAULTs.
	for _, code := range []uint32{200, 204, 403, 503} {
		id := uuid.NewString()
		payload, e := proto.Marshal(&events.OrtbEvent{Uuid: id, EventTimeMs: time.Now().UTC().Add(-time.Second).UnixMilli(), Code: code, Format: "POP", PubUserId: "owner", AdCodeId: "trusted", ZoneId: fmt.Sprint(code), SiteId: "site", SiteDomain: "example.test", RequestId: "request-" + id})
		if e != nil {
			t.Fatal(e)
		}
		record, e := unmarshalOrtbEvent(payload)
		if e != nil {
			t.Fatal(e)
		}
		// Retry exactly the same Kafka event; diagnostic dedup must stay synchronous.
		if _, e = insertBatchOrtb(ctx, ch, db+".ortb", []events.OrtbEvent{record, record}); e != nil {
			t.Fatal(e)
		}
	}
	if e := ApplyPublisherZoneDiagnostics(ctx, ch, db); e != nil {
		t.Fatal(e)
	}
	var n, ok, empty, client, server uint64
	if e := ch.QueryRow(ctx, "SELECT sum(requests_200),sum(requests_204),sum(requests_4xx),sum(requests_5xx) FROM "+db+".publisher_zone_hourly FINAL WHERE is_meta=0").Scan(&ok, &empty, &client, &server); e != nil || ok != 1 || empty != 1 || client != 1 || server != 1 {
		t.Fatal(ok, empty, client, server, e)
	}
	if e := ch.QueryRow(ctx, "SELECT countDistinct(zone_id) FROM "+db+".publisher_inventory WHERE pub_user_id='owner' AND ad_code_id='trusted' AND site_id='site' AND hostname='example.test'").Scan(&n); e != nil || n != 4 {
		t.Fatal("discovery was not populated", n, e)
	}
	if e := ch.QueryRow(ctx, "SELECT count() FROM "+db+".ortb WHERE pub_user_id='owner' AND ad_code_id='trusted' AND request_id!='' AND logical_event_id=toString(uuid) AND event_time>toDateTime(0) AND created_at>toDateTime(0)").Scan(&n); e != nil || n != 8 {
		t.Fatal("Publisher values came from defaults", n, e)
	}
	if e := ch.QueryRow(ctx, "SELECT sum(n) FROM "+db+".financial_sentinel").Scan(&n); e != nil || n != 7 {
		t.Fatal("unrelated data touched", n, e)
	}
	// Backfill previously attributed requests when discovery was absent.
	must("DROP VIEW " + db + ".mv_ortb_to_publisher_inventory SYNC")
	must("TRUNCATE TABLE " + db + ".publisher_inventory")
	if e := ApplyPublisherZoneDiagnostics(ctx, ch, db); e != nil {
		t.Fatal(e)
	}
	if e := ch.QueryRow(ctx, "SELECT countDistinct(zone_id) FROM "+db+".publisher_inventory").Scan(&n); e != nil || n != 4 {
		t.Fatal("bounded discovery backfill lost rows", n, e)
	}
}

func TestPublisherPreflightRejectsIncompatibleSchema(t *testing.T) {
	for _, which := range []string{"source", "inventory", "discovery", "created_default", "inventory_ttl"} {
		t.Run(which, func(t *testing.T) {
			ctx, ch, db := publisherPreflightFixture(t)
			must := func(q string) {
				if e := ch.Exec(ctx, q); e != nil {
					t.Fatal(e)
				}
			}
			switch which {
			case "source":
				must("ALTER TABLE " + db + ".ortb MODIFY COLUMN pub_user_id UInt64 DEFAULT 0")
			case "inventory":
				must("CREATE TABLE " + db + ".publisher_inventory(n UInt8) ENGINE=MergeTree ORDER BY n")
			case "created_default":
				must("ALTER TABLE " + db + ".ortb MODIFY COLUMN created_at DateTime64(3,'UTC') DEFAULT toDateTime64(0,3,'UTC')")
			case "inventory_ttl":
				must(strings.ReplaceAll(publisherInventoryDDL, "{db}", db))
				must("ALTER TABLE " + db + ".publisher_inventory MODIFY TTL toDateTime(last_request_at)+INTERVAL 1 HOUR")
			case "discovery":
				must("CREATE VIEW " + db + ".mv_ortb_to_publisher_inventory AS SELECT 1 AS n")
			}
			e := ApplyPublisherZoneDiagnostics(ctx, ch, db)
			if e == nil || !strings.Contains(e.Error(), "preflight") {
				t.Fatal("incompatible schema accepted", e)
			}
			var n uint64
			if e = ch.QueryRow(ctx, "SELECT count() FROM system.tables WHERE database=? AND name='publisher_zone_hourly'", db).Scan(&n); e != nil || n != 0 {
				t.Fatal("diagnostic cutover started before preflight", n, e)
			}
		})
	}
}
