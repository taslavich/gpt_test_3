package clickhouse_loader

// Upgrade a populated ClickHouse installation without deleting tables, columns,
// projections, or financial rows. Financial MVs are recreated from current DDL
// once, with upstream refreshes paused and no historical Publisher POP import.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
)

const publisherSchemaVersion = 248

var publisherAggPrefix = []string{
	"win_user_id", "win_cid", "win_crid", "event_date", "device_type",
	"os", "event_hour", "browser", "geo", "site_id", "format", "typic",
	"os_version",
}
var publisherAggSuffix = []string{"pub_user_id", "ad_code_id", "zone_id", "pop_revenue_bucket"}

func normalizedKey(s string) string {
	return strings.TrimPrefix(strings.ToLower(strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' || r == '(' || r == ')' || r == '`' {
			return -1
		}
		return r
	}, s)), "tuple")
}

// ddlStatement selects one current CREATE definition without the preceding
// comments. No DROP, DELETE, TRUNCATE or INSERT from the historical cutover
// template is ever executed on an existing installation.
func ddlStatement(name string) (string, error) {
	for _, raw := range splitClickHouseStatements(ddlTemplate) {
		pos := strings.Index(raw, name)
		if pos < 0 {
			continue
		}
		return strings.TrimSpace(raw[pos:]), nil
	}
	return "", fmt.Errorf("missing Publisher schema template for %s", name)
}

func execPublisherDDL(ctx context.Context, ch clickhouse.Conn, db, prefix string) error {
	q, err := ddlStatement(prefix)
	if err != nil {
		return err
	}
	q = strings.ReplaceAll(q, "{db}", db)
	if err = ch.Exec(ctx, q); err != nil {
		return fmt.Errorf("Publisher DDL %s: %w", prefix, err)
	}
	return nil
}

func tableExists(ctx context.Context, ch clickhouse.Conn, db, table string) (bool, error) {
	var n uint64
	err := ch.QueryRow(ctx, "SELECT count() FROM system.tables WHERE database=? AND name=?", db, table).Scan(&n)
	return n != 0, err
}

func publisherPreflight(ctx context.Context, ch clickhouse.Conn, db string) (diagnosticTableSchema, bool, error) {
	agg, err := diagnosticSchema(ctx, ch, db, "agg_stats")
	if err != nil {
		return agg, false, err
	}
	if agg.engine != "SummingMergeTree" && agg.engine != "ReplicatedSummingMergeTree" {
		return agg, false, fmt.Errorf("%s.agg_stats must be a SummingMergeTree; refusing to touch historical data (engine=%q)", db, agg.engine)
	}
	oldKey := normalizedKey(strings.Join(publisherAggPrefix, ","))
	newKey := normalizedKey(strings.Join(append(append([]string{}, publisherAggPrefix...), publisherAggSuffix...), ","))
	current := normalizedKey(agg.key)
	if current != oldKey && current != newKey {
		return agg, false, fmt.Errorf("%s.agg_stats unexpected ORDER BY %q; financial table untouched", db, agg.key)
	}
	// On the old key all four Publisher columns and the dependent projection
	// must be removed MANUALLY before deployment. Never discard column data.
	// Even an empty preexisting non-key column cannot be incorporated into the
	// sorting key with ADD COLUMN: ClickHouse requires the key suffix to be new.
	if current == oldKey {
		for _, col := range publisherAggSuffix {
			if _, exists := agg.columns[col]; exists {
				return agg, false, fmt.Errorf("%s.agg_stats.%s still exists outside ORDER BY; remove it manually before Loader startup", db, col)
			}
		}
	} else {
		for _, col := range publisherAggSuffix {
			want := "String"
			if col == "pop_revenue_bucket" {
				want = "UInt64"
			}
			if _, ok := agg.columns[col]; !ok || diagnosticSQL(agg.columns[col]) != diagnosticSQL(want) {
				return agg, false, fmt.Errorf("%s.agg_stats.%s missing or incompatible in existing ORDER BY", db, col)
			}
		}
	}
	if current == oldKey {
		ortb, e := diagnosticSchema(ctx, ch, db, "ortb")
		if e != nil {
			return agg, false, e
		}
		if _, found := ortb.columns["pub_user_id"]; found {
			var inFlight uint64
			if e = ch.QueryRow(ctx, "SELECT countIf(pub_user_id!='') FROM "+db+".ortb").Scan(&inFlight); e != nil {
				return agg, false, e
			}
			if inFlight != 0 {
				return agg, false, fmt.Errorf("%d Publisher ORTB events already in flight; cannot safely migrate agg_stats key", inFlight)
			}
		}
	}
	// Preflight rejects in-flight Publisher traffic as well; this migration
	// intentionally does NOT backfill any facts or financial balances.
	for _, source := range []string{"fact_impressions", "fact_clicks", "fact_clicks_wins", "fact_conversions"} {
		sch, e := diagnosticSchema(ctx, ch, db, source)
		if e != nil {
			return agg, false, e
		}
		if sch.engine == "" {
			return agg, false, fmt.Errorf("missing %s.%s; refusing partial financial migration", db, source)
		}
		if _, has := sch.columns["pub_user_id"]; has {
			var count uint64
			if e = ch.QueryRow(ctx, "SELECT countIf(pub_user_id!='') FROM "+db+"."+source).Scan(&count); e != nil {
				return agg, false, e
			}
			if count > 0 && current != newKey {
				return agg, false, fmt.Errorf("Publisher financial facts already exist in %s.%s; in-place first cutover is blocked", db, source)
			}
		}
	}
	return agg, current == newKey, nil
}

func ensurePublisherAggKey(ctx context.Context, ch clickhouse.Conn, db string, agg diagnosticTableSchema, current bool) error {
	if current {
		return nil
	}
	// Manual precondition: publisher_pop_by_bucket and all four old non-key
	// columns have already been dropped by the operator. No DROP here.
	target := "(" + strings.Join(append(append([]string{}, publisherAggPrefix...), publisherAggSuffix...), ",") + ")"
	// All new ORDER BY columns have NO DEFAULT in this atomic ADD+MODIFY.
	alter := "ALTER TABLE " + db + ".agg_stats " +
		"ADD COLUMN IF NOT EXISTS pub_user_id String, " +
		"ADD COLUMN IF NOT EXISTS ad_code_id String, " +
		"ADD COLUMN IF NOT EXISTS zone_id String, " +
		"ADD COLUMN IF NOT EXISTS pop_revenue_bucket UInt64, " +
		"MODIFY ORDER BY " + target
	if err := ch.Exec(ctx, alter); err != nil {
		return fmt.Errorf("agg_stats key extension failed; original table and advertiser history retained: %w", err)
	}
	if err := ch.Exec(ctx, "ALTER TABLE "+db+".agg_stats "+
		"MODIFY COLUMN pub_user_id String DEFAULT '', "+
		"MODIFY COLUMN ad_code_id String DEFAULT '', "+
		"MODIFY COLUMN zone_id String DEFAULT '', "+
		"MODIFY COLUMN pop_revenue_bucket UInt64 DEFAULT 0"); err != nil {
		return err
	}
	return nil
}

// The DROP VIEW syntax must never be allowed to target a real data table,
// including in an unexpected partially deployed schema.
func dropPublisherMV(ctx context.Context, ch clickhouse.Conn, db, name string) error {
	var engine string
	err := ch.QueryRow(ctx, "SELECT engine FROM system.tables WHERE database=? AND name=?", db, name).Scan(&engine)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if engine != "MaterializedView" && engine != "RefreshableMaterializedView" {
		return fmt.Errorf("refusing DROP VIEW %s.%s: engine %s is not an MV", db, name, engine)
	}
	if err = ch.Exec(ctx, "DROP VIEW IF EXISTS "+db+"."+name+" SYNC"); err != nil {
		return fmt.Errorf("drop MV %s (target retained): %w", name, err)
	}
	return nil
}

// Always recreate financial MVs from the canonical CREATE DDL, rather than
// silently retaining obsolete SELECTs with CREATE IF NOT EXISTS or MODIFY QUERY.
// Dropping an MV does NOT drop its TO/APPEND TO destination table or its rows.
func replacePublisherMV(ctx context.Context, ch clickhouse.Conn, db, name string) error {
	prefix := "CREATE MATERIALIZED VIEW {db}." + name
	stmt, err := ddlStatement(prefix)
	if err != nil {
		stmt, err = ddlStatement("CREATE MATERIALIZED VIEW IF NOT EXISTS {db}." + name)
	}
	if err != nil {
		return err
	}
	stmt = strings.ReplaceAll(stmt, "{db}", db)
	if err = dropPublisherMV(ctx, ch, db, name); err != nil {
		return err
	}
	if err = ch.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("recreate MV %s: %w", name, err)
	}
	return nil
}

// Stop refreshable writers and wait for in-flight runs BEFORE changing the
// source table or dropping downstream MVs. A completed failed run (730) is safe.
func stopPublisherRefreshMV(ctx context.Context, ch clickhouse.Conn, db, name string) error {
	exists, err := tableExists(ctx, ch, db, name)
	if err != nil || !exists {
		return err
	}
	if err = ch.Exec(ctx, "SYSTEM STOP VIEW "+db+"."+name); err != nil {
		return fmt.Errorf("stop MV %s: %w", name, err)
	}
	if err = ch.Exec(ctx, "SYSTEM WAIT VIEW "+db+"."+name); err != nil {
		var ex *clickhouse.Exception
		if !errors.As(err, &ex) || ex.Code != 730 {
			return fmt.Errorf("wait for MV %s: %w", name, err)
		}
	}
	return nil
}

func pausePublisherRefreshes(ctx context.Context, ch clickhouse.Conn, db string) error {
	for _, mv := range []string{
		"mv_impressions_to_fact", "mv_clicks_wins_to_fact", "mv_conversions_to_fact", "mv_clicks_to_fact",
		"mv_publisher_pop_revenue_checkpoint", "mv_publisher_pop_revenue_to_agg_stats",
		"mv_recover_pop_impressions_v2", // independent hourly writer to fact_impressions
	} {
		if err := stopPublisherRefreshMV(ctx, ch, db, mv); err != nil {
			return err
		}
	}
	return nil
}

// Recreate consumers before producers, so no facts can be appended while their
// billing/Publisher registry consumer is missing. Caller must pause ALL other
// producers (and recurring writers outside this Loader) during schema cutover.
func recreatePublisherFinancialMVs(ctx context.Context, ch clickhouse.Conn, db string) error {
	producers := []string{
		"mv_impressions_to_fact", "mv_clicks_wins_to_fact", "mv_conversions_to_fact", "mv_clicks_to_fact",
	}
	// Detach all fact-producing MVs first, before any fact consumer. Replacements
	// for missing MVs are also created (DROP IF EXISTS is idempotent).
	for _, mv := range producers {
		if err := dropPublisherMV(ctx, ch, db, mv); err != nil {
			return err
		}
	}
	// Refreshable dependency: checkpoint depends on the money view. Both were
	// stopped and waited before any schema changes; drop dependent first.
	for _, mv := range []string{"mv_publisher_pop_revenue_checkpoint", "mv_publisher_pop_revenue_to_agg_stats"} {
		if err := dropPublisherMV(ctx, ch, db, mv); err != nil {
			return err
		}
	}
	// Fact consumers have no active upstream producer at this point.
	for _, mv := range []string{
		"mv_fact_impressions_to_agg_stats", "mv_fact_clicks_to_agg_stats",
		"mv_fact_clicks_wins_to_agg_stats", "mv_fact_conversions_to_agg_stats",
		"mv_ortb_to_publisher_inventory", "mv_fact_clicks_to_publisher_pop_revenue",
	} {
		if err := replacePublisherMV(ctx, ch, db, mv); err != nil {
			return err
		}
	}
	// The two Publisher POP refreshes are now installed in dependency order.
	for _, mv := range []string{"mv_publisher_pop_revenue_to_agg_stats", "mv_publisher_pop_revenue_checkpoint"} {
		if err := createPublisherMV(ctx, ch, db, mv); err != nil {
			return err
		}
	}
	// Resume fact production last; all its consumers already exist.
	for _, mv := range producers {
		if err := createPublisherMV(ctx, ch, db, mv); err != nil {
			return err
		}
	}
	// Restore an independent POP recovery writer only after its fact consumers
	// have been recreated. This recovery MV itself is NEVER dropped.
	if exists, err := tableExists(ctx, ch, db, "mv_recover_pop_impressions_v2"); err != nil {
		return err
	} else if exists {
		if err := ch.Exec(ctx, "SYSTEM START VIEW "+db+".mv_recover_pop_impressions_v2"); err != nil {
			return fmt.Errorf("resume POP recovery writer: %w", err)
		}
	}
	return nil
}

func createPublisherMV(ctx context.Context, ch clickhouse.Conn, db, name string) error {
	stmt, err := ddlStatement("CREATE MATERIALIZED VIEW {db}." + name)
	if err != nil {
		stmt, err = ddlStatement("CREATE MATERIALIZED VIEW IF NOT EXISTS {db}." + name)
	}
	if err != nil {
		return err
	}
	if err = ch.Exec(ctx, strings.ReplaceAll(stmt, "{db}", db)); err != nil {
		return fmt.Errorf("create MV %s: %w", name, err)
	}
	return nil
}

// Mark the completed MV replacement. A restart after a partial cutover can
// retry the schema and MV creation (with ingestion still paused).
func ensurePublisherProductionSchema(ctx context.Context, ch clickhouse.Conn, db string) error {
	// Durable marker avoids touching production MV definitions on later Loader
	// restarts after Publisher traffic has begun.
	if err := ch.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+db+".publisher_schema_versions (version UInt64) ENGINE=ReplacingMergeTree ORDER BY version"); err != nil {
		return err
	}
	var done uint64
	if err := ch.QueryRow(ctx, "SELECT count() FROM "+db+".publisher_schema_versions WHERE version=?", publisherSchemaVersion).Scan(&done); err != nil {
		return err
	}
	if done > 0 {
		sch, err := diagnosticSchema(ctx, ch, db, "agg_stats")
		if err != nil {
			return err
		}
		expected := normalizedKey(strings.Join(append(append([]string{}, publisherAggPrefix...), publisherAggSuffix...), ","))
		if normalizedKey(sch.key) != expected {
			return fmt.Errorf("Publisher migration marker exists, but agg_stats ORDER BY differs; refusing unsafe reload")
		}
		for _, mv := range []string{"mv_fact_impressions_to_agg_stats", "mv_fact_clicks_to_agg_stats", "mv_fact_clicks_wins_to_agg_stats", "mv_fact_conversions_to_agg_stats", "mv_fact_clicks_to_publisher_pop_revenue", "mv_publisher_pop_revenue_to_agg_stats", "mv_publisher_pop_revenue_checkpoint"} {
			present, e := tableExists(ctx, ch, db, mv)
			if e != nil {
				return e
			}
			if !present {
				return fmt.Errorf("Publisher migration marker exists but %s is missing; financial views are not silently recreated", mv)
			}
		}
		return nil
	}
	agg, keyed, err := publisherPreflight(ctx, ch, db)
	if err != nil {
		return err
	}
	// Quiesce refreshable writers before changing agg_stats sorting key or
	// updating consumer schemas. Even a stopped Loader process does not stop
	// ClickHouse-managed REFRESH EVERY tasks.
	if err = pausePublisherRefreshes(ctx, ch, db); err != nil {
		return err
	}
	// Make sure current ORTB/Fact sources have the publisher attribution fields
	// BEFORE installing queries which select them.
	for _, col := range publisherSourceColumns {
		switch col.name {
		case "pub_user_id", "ad_code_id", "zone_id", "request_id", "site_page", "request_error", "error_reason", "logical_event_id":
			if err := ch.Exec(ctx, "ALTER TABLE "+db+".ortb ADD COLUMN IF NOT EXISTS "+col.name+" "+col.typ+" DEFAULT "+col.def); err != nil {
				return err
			}
		}
	}
	for _, fact := range []string{"fact_impressions", "fact_clicks", "fact_clicks_wins", "fact_conversions"} {
		for _, col := range []string{"pub_user_id", "ad_code_id", "zone_id"} {
			if err = ch.Exec(ctx, "ALTER TABLE "+db+"."+fact+" ADD COLUMN IF NOT EXISTS "+col+" String DEFAULT ''"); err != nil {
				return err
			}
		}
	}
	if err = ensurePublisherAggKey(ctx, ch, db, agg, keyed); err != nil {
		return err
	}
	// No historical Publisher rows are imported, and no IPP balances are rewritten.
	for _, tbl := range []string{"publisher_inventory", "publisher_pop_revenue_events_v2", "publisher_pop_revenue_checkpoints"} {
		prefix := "CREATE TABLE IF NOT EXISTS {db}." + tbl
		if err = execPublisherDDL(ctx, ch, db, prefix); err != nil {
			return err
		}
	}
	if err = ch.Exec(ctx, "ALTER TABLE "+db+".agg_stats MODIFY SETTING deduplicate_merge_projection_mode='rebuild'"); err != nil {
		return err
	}
	if err = execPublisherDDL(ctx, ch, db, "ALTER TABLE {db}.agg_stats ADD PROJECTION IF NOT EXISTS publisher_pop_by_bucket"); err != nil {
		return err
	}
	for _, name := range []string{"publisher_pop_revenue_dirty", "publisher_pop_revenue_delta"} {
		exists, e := tableExists(ctx, ch, db, name)
		if e != nil {
			return e
		}
		if !exists {
			if err = execPublisherDDL(ctx, ch, db, "CREATE VIEW {db}."+name+" AS"); err != nil {
				return err
			}
		}
	}
	// Financial refreshes are recreated exactly once, never on routine restarts.
	// Source ingestion must be paused externally for this first cutover.
	if err = recreatePublisherFinancialMVs(ctx, ch, db); err != nil {
		return err
	}
	// Record only the schema upgrade, so later Loader starts leave money MVs untouched.
	return ch.Exec(ctx, "INSERT INTO "+db+".publisher_schema_versions VALUES(?)", publisherSchemaVersion)
}
