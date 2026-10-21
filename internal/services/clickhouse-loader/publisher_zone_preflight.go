package clickhouse_loader

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Only fields used by diagnostics/discovery or written by the Publisher Kafka
// contract. Missing historical identity/timestamps cannot be reconstructed.
var publisherSourceColumns = []struct{ name, typ, def string }{
	{"uuid", "UUID", "toUUID('00000000-0000-0000-0000-000000000000')"},
	{"event_time", "DateTime64(3, 'UTC')", "toDateTime64(0,3,'UTC')"},
	{"created_at", "DateTime64(3, 'UTC')", "now64(3,'UTC')"},
	{"logical_event_id", "String", "toString(uuid)"},
	{"code", "UInt16", "0"}, {"format", "LowCardinality(String)", "''"},
	{"pub_user_id", "String", "''"}, {"ad_code_id", "String", "''"}, {"zone_id", "String", "''"},
	{"site_id", "Nullable(String)", "NULL"}, {"site_domain", "Nullable(String)", "NULL"},
	{"geo", "Nullable(String)", "NULL"}, {"device", "Nullable(String)", "NULL"},
	{"os", "Nullable(String)", "NULL"}, {"browser", "Nullable(String)", "NULL"},
	{"request_id", "String", "''"}, {"site_page", "String", "''"},
	{"request_error", "UInt8", "0"}, {"error_reason", "String", "''"},
}

const publisherInventoryDDL = `CREATE TABLE IF NOT EXISTS {db}.publisher_inventory (
 pub_user_id String,site_id String,ad_code_id String,zone_id String,hostname String,
 first_request_at SimpleAggregateFunction(min,DateTime64(3,'UTC')),
 last_request_at SimpleAggregateFunction(max,DateTime64(3,'UTC'))
) ENGINE=AggregatingMergeTree ORDER BY (pub_user_id,site_id,ad_code_id,zone_id,hostname)`

const publisherInventorySelect = `SELECT pub_user_id,ifNull(site_id,'') AS site_id,ad_code_id,zone_id,
 ifNull(site_domain,'') AS hostname,min(event_time) AS first_request_at,max(event_time) AS last_request_at
 FROM {db}.ortb WHERE pub_user_id!='' AND site_id!='' AND zone_id!=''
 GROUP BY pub_user_id,site_id,ad_code_id,zone_id,hostname`

type diagnosticTableSchema struct {
	defaults         map[string]string
	engine, key, ddl string
	columns          map[string]string
	kinds            map[string]string
}

func diagnosticSchema(ctx context.Context, ch clickhouse.Conn, db, table string) (diagnosticTableSchema, error) {
	out := diagnosticTableSchema{columns: map[string]string{}, kinds: map[string]string{}, defaults: map[string]string{}}
	var n uint64
	if err := ch.QueryRow(ctx, "SELECT count() FROM system.tables WHERE database=? AND name=?", db, table).Scan(&n); err != nil {
		return out, err
	}
	if n == 0 {
		return out, nil
	}
	if err := ch.QueryRow(ctx, "SELECT engine,sorting_key,create_table_query FROM system.tables WHERE database=? AND name=?", db, table).Scan(&out.engine, &out.key, &out.ddl); err != nil {
		return out, err
	}
	rows, err := ch.Query(ctx, "SELECT name,type,default_kind,default_expression FROM system.columns WHERE database=? AND table=?", db, table)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, typ, kind, expression string
		if err = rows.Scan(&name, &typ, &kind, &expression); err != nil {
			return out, err
		}
		out.columns[name] = typ
		out.kinds[name] = kind
		out.defaults[name] = expression
	}
	return out, rows.Err()
}

func diagnosticSQL(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '`' || r == '(' || r == ')' {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
func sourceTypeCompatible(name, actual, want string) bool {
	if diagnosticSQL(actual) == diagnosticSQL(want) {
		return true
	}
	switch name {
	case "event_time", "created_at":
		return actual == "DateTime" || strings.HasPrefix(actual, "DateTime(") || strings.HasPrefix(actual, "DateTime64(")
	case "site_id", "site_domain", "geo", "device", "os", "browser":
		return actual == "String" || actual == "LowCardinality(String)" || actual == "LowCardinality(Nullable(String))"
	case "format":
		return actual == "String"
	}
	return false
}

// Inspect ALL existing objects before adding anything or stopping any writer.
// A mismatch is actionable; it must never fall back to the financial CreateDB.
func preparePublisherZoneSource(ctx context.Context, ch clickhouse.Conn, db string) error {
	source, err := diagnosticSchema(ctx, ch, db, "ortb")
	if err != nil {
		return err
	}
	if source.engine != "MergeTree" && source.engine != "ReplicatedMergeTree" {
		return fmt.Errorf("publisher diagnostics preflight: %s.ortb requires a local MergeTree source (found %q); full CreateDB was NOT run", db, source.engine)
	}
	for _, c := range publisherSourceColumns {
		if actual, ok := source.columns[c.name]; ok && (!sourceTypeCompatible(c.name, actual, c.typ) || source.kinds[c.name] == "ALIAS" || source.kinds[c.name] == "MATERIALIZED") {
			return fmt.Errorf("publisher diagnostics preflight: %s.ortb.%s type/kind %s/%s is incompatible; expected %s (or supported compatible type); no destructive changes made", db, c.name, actual, source.kinds[c.name], c.typ)
		}
	}
	if source.columns["created_at"] != "" {
		expression := strings.TrimSpace(source.defaults["created_at"])
		clockDefault := regexp.MustCompile(`^(now\(\)|now64\(\s*([0-9](\s*,\s*'[^']+')?)?\s*\))$`)
		if source.kinds["created_at"] != "DEFAULT" || !clockDefault.MatchString(expression) {
			return fmt.Errorf("publisher diagnostics preflight: %s.ortb.created_at must default to now()/now64() for the actual Loader, which does not insert created_at; found %q; existing defaults were not changed", db, expression)
		}
	}
	inventory, err := diagnosticSchema(ctx, ch, db, "publisher_inventory")
	if err != nil {
		return err
	}
	if inventory.engine != "" {
		if strings.Contains(strings.ToUpper(inventory.ddl), "TTL ") {
			return fmt.Errorf("publisher diagnostics preflight: %s.publisher_inventory must be durable; unexpected TTL must be reviewed explicitly", db)
		}
		if inventory.engine != "AggregatingMergeTree" || diagnosticSQL(inventory.key) != "pub_user_id,site_id,ad_code_id,zone_id,hostname" {
			return fmt.Errorf("publisher diagnostics preflight: %s.publisher_inventory incompatible engine/key: %s / %s", db, inventory.engine, inventory.key)
		}
		expected := map[string]string{"pub_user_id": "String", "site_id": "String", "ad_code_id": "String", "zone_id": "String", "hostname": "String", "first_request_at": "SimpleAggregateFunction(min,DateTime64(3,'UTC'))", "last_request_at": "SimpleAggregateFunction(max,DateTime64(3,'UTC'))"}
		for name, typ := range expected {
			if diagnosticSQL(inventory.columns[name]) != diagnosticSQL(typ) || inventory.kinds[name] == "ALIAS" || inventory.kinds[name] == "MATERIALIZED" {
				return fmt.Errorf("publisher diagnostics preflight: %s.publisher_inventory.%s has %q; expected %s", db, name, inventory.columns[name], typ)
			}
		}
	}
	discovery, err := diagnosticSchema(ctx, ch, db, "mv_ortb_to_publisher_inventory")
	if err != nil {
		return err
	}
	selectSQL := strings.ReplaceAll(publisherInventorySelect, "{db}", db)
	if discovery.engine != "" && (discovery.engine != "MaterializedView" || !strings.Contains(diagnosticSQL(discovery.ddl), "to"+diagnosticSQL(db)+".publisher_inventory") || !strings.HasSuffix(diagnosticSQL(discovery.ddl), diagnosticSQL("AS "+selectSQL))) {
		return fmt.Errorf("publisher diagnostics preflight: %s.mv_ortb_to_publisher_inventory has an incompatible definition; review it explicitly", db)
	}
	diagnosticView, err := diagnosticSchema(ctx, ch, db, "mv_publisher_zone_hourly")
	if err != nil {
		return err
	}
	if diagnosticView.engine != "" {
		var refreshable uint64
		if err = ch.QueryRow(ctx, "SELECT count() FROM system.view_refreshes WHERE database=? AND view='mv_publisher_zone_hourly'", db).Scan(&refreshable); err != nil {
			return err
		}
		if diagnosticView.engine != "MaterializedView" || refreshable != 1 {
			return fmt.Errorf("publisher diagnostics preflight: %s.mv_publisher_zone_hourly must already be a Refreshable Materialized View", db)
		}
	}
	// Existing attributed data must not gain invented identity or event times.
	missingHistory := source.columns["uuid"] == "" || source.columns["event_time"] == "" || source.columns["created_at"] == ""
	if missingHistory && source.columns["pub_user_id"] != "" {
		var n uint64
		if err = ch.QueryRow(ctx, "SELECT count() FROM (SELECT 1 FROM "+db+".ortb WHERE pub_user_id!='' LIMIT 1)").Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("publisher diagnostics preflight: attributed historical rows exist but uuid/event_time/created_at is missing; cannot reconstruct their identity/time safely")
		}
	}
	// Additive, metadata-only source upgrades; defaults never claim old SSP rows
	// as Publisher requests. Future Publisher values are explicitly written by Loader.
	for _, c := range publisherSourceColumns {
		if source.columns[c.name] == "" {
			if err = ch.Exec(ctx, "ALTER TABLE "+db+".ortb ADD COLUMN IF NOT EXISTS "+c.name+" "+c.typ+" DEFAULT "+c.def); err != nil {
				return fmt.Errorf("publisher diagnostics add %s: %w", c.name, err)
			}
			log.Printf("Publisher diagnostics: added %s.ortb.%s; historical missing attribution is not reconstructed", db, c.name)
		}
	}
	if err = ch.Exec(ctx, strings.ReplaceAll(publisherInventoryDDL, "{db}", db)); err != nil {
		return err
	}
	if err = ch.Exec(ctx, "CREATE MATERIALIZED VIEW IF NOT EXISTS "+db+".mv_ortb_to_publisher_inventory TO "+db+".publisher_inventory AS "+selectSQL); err != nil {
		return err
	}
	// The retained source tail also predates creation of discovery on old installs.
	// min/max remain idempotent under retries and concurrent discovery INSERTs.
	backfill := strings.Replace(selectSQL, " WHERE ", " PREWHERE created_at>=now64(3,'UTC')-INTERVAL 65 MINUTE WHERE ", 1)
	if err = ch.Exec(ctx, "INSERT INTO "+db+".publisher_inventory "+backfill+" SETTINGS max_threads=2,max_memory_usage=536870912,max_bytes_before_external_group_by=134217728"); err != nil {
		return fmt.Errorf("publisher inventory bounded backfill: %w", err)
	}
	return nil
}
