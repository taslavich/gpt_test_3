package clickhouse_loader

import (
	"strings"
	"testing"
)

func TestPopRecoveryDDLContract(t *testing.T) {
	t.Parallel()

	required := []string{
		"is_pop_recovered       UInt8 DEFAULT 0",
		"pop_recovered_at       Nullable(DateTime64(3, 'UTC')) DEFAULT NULL",
		"pop_recovery_key       String DEFAULT ''",
		"pop_recovery_source_key String DEFAULT ''",
		"pop_recovered_spend Float64 DEFAULT 0",
		"CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_recover_pop_impressions_v2",
		"concat('pop-click:', toString(c.clicks_uuid)) AS recovery_source_key",
		"toUInt8(1) AS is_pop_recovered",
		"CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_pop_recovered_to_agg_stats",
		"sum(win_dsp_price / 1000) AS pop_recovered_spend",
		"POP RECOVERY TRANSITION CATCH-UP",
		"toStartOfHour(now('UTC') - INTERVAL 2 HOUR) AS dt_from",
		"WHERE c.recovery_rank <= d.missing_cnt",
	}
	for _, want := range required {
		if !strings.Contains(ddlTemplate, want) {
			t.Fatalf("POP recovery DDL is missing %q", want)
		}
	}

	createReplacement := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_recover_pop_impressions_v2")
	dropLegacy := strings.Index(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_recover_pop_impressions SYNC")
	if createReplacement < 0 || dropLegacy < 0 || createReplacement >= dropLegacy {
		t.Fatalf("legacy POP recovery must be dropped only after replacement creation: create=%d drop=%d", createReplacement, dropLegacy)
	}
	if strings.Count(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_recover_pop_impressions SYNC") != 1 {
		t.Fatalf("legacy POP recovery drop must occur exactly once after replacement creation")
	}

	if strings.Contains(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_fact_impressions_to_agg_stats SYNC") {
		t.Fatal("ordinary impression->agg_stats MV must stay live while POP recovery is cut over")
	}

	ordinaryAgg := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_impressions_to_agg_stats")
	recoveryAgg := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_pop_recovered_to_agg_stats")
	catchUp := strings.Index(ddlTemplate, "POP RECOVERY TRANSITION CATCH-UP")
	if ordinaryAgg < 0 || recoveryAgg < 0 || catchUp < 0 || ordinaryAgg >= catchUp || recoveryAgg >= catchUp {
		t.Fatalf("both aggregate MVs must be installed before transition catch-up: ordinary=%d recovery=%d catchup=%d", ordinaryAgg, recoveryAgg, catchUp)
	}
}

func TestPopLateArrivalGuardPreservesNormalSemantics(t *testing.T) {
	t.Parallel()

	guardStart := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_impressions_to_fact")
	guardEnd := strings.Index(ddlTemplate, "REFRESHABLE MV: CLICKS INPUT -> FACT CLICKS")
	if guardStart < 0 || guardEnd <= guardStart {
		t.Fatal("cannot locate impressions->fact MV in DDL")
	}
	guard := ddlTemplate[guardStart:guardEnd]

	for _, want := range []string{
		"toUInt8(0) AS is_pop_recovered",
		"CAST(NULL AS Nullable(DateTime64(3, 'UTC'))) AS pop_recovered_at",
		"'' AS pop_recovery_source_key",
		"o.format != 'POP'",
		"ifNull(fs.recovered_cnt, 0) = 0",
		"ifNull(ls.matched_source_cnt, 0) = 0",
		"ifNull(fs.impression_cnt, 0) = ifNull(fs.unique_impression_uuid_cnt, 0)",
		"ifNull(fs.impression_cnt, 0) + toUInt64(a.pop_pending_rank)",
		"<= ifNull(cs.click_cnt, 0)",
		"concat('pop-auction:', toString(o.uuid))",
	} {
		if !strings.Contains(guard, want) {
			t.Fatalf("late-arrival guard is missing %q", want)
		}
	}
}

func TestPopRecoveryStatementsStillSplitCleanly(t *testing.T) {
	t.Parallel()

	dll := strings.ReplaceAll(ddlTemplate, "{db}", "ads")
	dll = strings.ReplaceAll(dll, "{ipp_cutover_from}", "2026-10-05 00:00:00")
	statements := splitClickHouseStatements(dll)
	if len(statements) == 0 {
		t.Fatal("full ClickHouse DDL split produced no statements")
	}

	var replacementSeen, legacyDropSeen, catchUpSeen bool
	for _, statement := range statements {
		switch {
		case strings.Contains(statement, "CREATE MATERIALIZED VIEW IF NOT EXISTS ads.mv_recover_pop_impressions_v2"):
			replacementSeen = true
		case strings.Contains(statement, "DROP VIEW IF EXISTS ads.mv_recover_pop_impressions SYNC"):
			if !replacementSeen {
				t.Fatal("legacy recovery drop was split before replacement creation")
			}
			legacyDropSeen = true
		case strings.Contains(statement, "POP RECOVERY TRANSITION CATCH-UP"):
			if !legacyDropSeen {
				t.Fatal("transition catch-up must run after legacy recovery has been retired")
			}
			catchUpSeen = true
		}
	}
	if !replacementSeen || !legacyDropSeen || !catchUpSeen {
		t.Fatalf("missing recovery statements after split: replacement=%t legacy_drop=%t catchup=%t", replacementSeen, legacyDropSeen, catchUpSeen)
	}
}
