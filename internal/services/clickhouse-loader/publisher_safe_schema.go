package clickhouse_loader

// This file upgrades a populated ClickHouse installation without running the
// destructive, first-install DDL. In particular, it never drops, renames or
// recreates agg_stats, never replays advertiser facts, and never drops a money
// materialized view. No historical Publisher POP revenue is migrated.

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
)

const publisherSchemaVersion = 246

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
	// The existing advertiser rows can never be re-attributed after the fact.
	// Refuse to remove any existing Publisher value, including nonzero buckets.
	checks := make([]string, 0, 4)
	for _, col := range publisherAggSuffix {
		if _, ok := agg.columns[col]; !ok {
			continue
		}
		want := "String"
		if col == "pop_revenue_bucket" {
			want = "UInt64"
		}
		if diagnosticSQL(agg.columns[col]) != diagnosticSQL(want) || (agg.kinds[col] != "" && agg.kinds[col] != "DEFAULT") {
			return agg, false, fmt.Errorf("%s.agg_stats.%s incompatible type/kind; no changes made", db, col)
		}
		if col == "pop_revenue_bucket" {
			checks = append(checks, col+" != 0")
		} else {
			checks = append(checks, col+" != ''")
		}
	}
	if current == oldKey && len(checks) != 0 {
		var populated uint64
		if err = ch.QueryRow(ctx, "SELECT countIf("+strings.Join(checks, " OR ")+") FROM "+db+".agg_stats").Scan(&populated); err != nil {
			return agg, false, err
		}
		if populated != 0 {
			return agg, false, fmt.Errorf("%s.agg_stats has %d Publisher-attributed rows; refusing to DROP ANY column or change ORDER BY", db, populated)
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
	// The optional Publisher-only derived projection may refer to the four
	// empty columns. Remove that projection's metadata (NOT agg_stats or any
	// data part) before readding key columns; it is recreated below.
	if err := ch.Exec(ctx, "ALTER TABLE "+db+".agg_stats DROP PROJECTION IF EXISTS publisher_pop_by_bucket"); err != nil {
		return fmt.Errorf("cannot detach Publisher-only projection safely: %w", err)
	}
	// Only the four empty Publisher columns are ever dropped; no historic
	// advertiser, time, price, statistics or settlement column is modified.
	for _, col := range publisherAggSuffix {
		if _, ok := agg.columns[col]; ok {
			if err := ch.Exec(ctx, "ALTER TABLE "+db+".agg_stats DROP COLUMN "+col); err != nil {
				return fmt.Errorf("cannot remove empty, not-in-ORDER-BY Publisher column %s (agg_stats itself was NOT dropped): %w", col, err)
			}
		}
	}
	target := "(" + strings.Join(append(append([]string{}, publisherAggPrefix...), publisherAggSuffix...), ",") + ")"
	// All new ORDER BY columns have NO DEFAULT in this atomic ADD+MODIFY.
	alter := "ALTER TABLE " + db + ".agg_stats " +
		"ADD COLUMN pub_user_id String, " +
		"ADD COLUMN ad_code_id String, " +
		"ADD COLUMN zone_id String, " +
		"ADD COLUMN pop_revenue_bucket UInt64, " +
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

// Change the SELECT of an existing TO-target MV, preserving the MV object and
// target table. ClickHouse explicitly supports MODIFY QUERY without stopping
// ingestion; it changes only NEW inserts, not historical advertiser totals.
func upsertPublisherMV(ctx context.Context, ch clickhouse.Conn, db, name string) error {
	prefix := "CREATE MATERIALIZED VIEW {db}." + name
	stmt, err := ddlStatement(prefix)
	if err != nil {
		prefix = "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}." + name
		stmt, err = ddlStatement(prefix)
	}
	if err != nil {
		return err
	}
	stmt = strings.ReplaceAll(stmt, "{db}", db)
	exists, err := tableExists(ctx, ch, db, name)
	if err != nil {
		return err
	}
	if !exists {
		// No replay/backfill. Preserve the original schedule and target.
		if err = ch.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
		return nil
	}
	// The CREATE header can use either "AS SELECT", " AS\nSELECT"
	// or "\nAS\nSELECT". Match only the SQL keyword, not aliases.
	as := regexp.MustCompile(`(?i)\bAS\s+`)
	loc := as.FindStringIndex(stmt)
	if loc == nil {
		return fmt.Errorf("cannot find AS SELECT in MV %s", name)
	}
	query := strings.TrimSpace(stmt[loc[1]:])
	// All target MVs are either TO or refreshable APPEND TO; only SELECT changes.
	if err = ch.Exec(ctx, "ALTER TABLE "+db+"."+name+" MODIFY QUERY "+query); err != nil {
		return fmt.Errorf("modify Publisher query %s in place: %w", name, err)
	}
	return nil
}

// The last step writes the migration marker. A restart after a crash retries
// metadata-only CREATE IF NOT EXISTS and MODIFY QUERY; no financial replay.
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
	// No historical Publisher rows exist: there is nothing to import, reset or
	// repair. In particular: NEVER execute the IPP DELETE/INSERT cutover here.
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
	// Upgrade existing financial MVs IN PLACE. Their FROM/TARGET and existing
	// advertiser stats remain unchanged; new Publisher dimensions appear only
	// on subsequently ingested facts.
	for _, mv := range []string{
		"mv_impressions_to_fact", "mv_clicks_wins_to_fact", "mv_conversions_to_fact", "mv_clicks_to_fact",
		"mv_fact_impressions_to_agg_stats", "mv_fact_clicks_to_agg_stats", "mv_fact_clicks_wins_to_agg_stats", "mv_fact_conversions_to_agg_stats",
	} {
		if err = upsertPublisherMV(ctx, ch, db, mv); err != nil {
			return err
		}
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
	for _, mv := range []string{"mv_ortb_to_publisher_inventory", "mv_fact_clicks_to_publisher_pop_revenue", "mv_publisher_pop_revenue_to_agg_stats", "mv_publisher_pop_revenue_checkpoint"} {
		if err = upsertPublisherMV(ctx, ch, db, mv); err != nil {
			return err
		}
	}
	// Record only the schema upgrade, so later Loader starts leave money MVs untouched.
	return ch.Exec(ctx, "INSERT INTO "+db+".publisher_schema_versions VALUES(?)", publisherSchemaVersion)
}
