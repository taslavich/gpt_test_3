package clickhouse_loader

import (
	"strings"
	"testing"
	"time"
)

func ddlSection(t *testing.T, start, end string) string {
	t.Helper()
	startPos := strings.Index(ddlTemplate, start)
	if startPos < 0 {
		t.Fatalf("cannot locate DDL section start %q", start)
	}
	endPos := strings.Index(ddlTemplate[startPos+len(start):], end)
	if endPos < 0 {
		t.Fatalf("cannot locate DDL section end %q after %q", end, start)
	}
	return ddlTemplate[startPos : startPos+len(start)+endPos]
}

func TestPopRecoveryDDLContract(t *testing.T) {
	t.Parallel()

	required := []string{
		"is_pop_recovered       UInt8 DEFAULT 0",
		"pop_recovered_at       Nullable(DateTime64(3, 'UTC')) DEFAULT NULL",
		"pop_recovery_key       String DEFAULT ''",
		"pop_recovery_source_key String DEFAULT ''",
		"pop_recovered_spend Float64 DEFAULT 0",
		"CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_recover_pop_impressions_v2",
		"REFRESH EVERY 1 HOUR OFFSET 10 MINUTE",
		"concat('pop-click:', toString(c.clicks_uuid)) AS recovery_source_key",
		"toUInt8(1) AS is_pop_recovered",
		"CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_pop_recovered_to_agg_stats",
		"sum(win_dsp_price / 1000) AS pop_recovered_spend",
		"POP RECOVERY TRANSITION CATCH-UP",
		"toStartOfHour(now('UTC') - INTERVAL 2 HOUR) AS dt_from",
		"WHERE c.recovery_rank <= c.missing_cnt",
		"WHERE s.recovery_rank <= s.missing_cnt",
	}
	for _, want := range required {
		if !strings.Contains(ddlTemplate, want) {
			t.Fatalf("POP recovery DDL is missing %q", want)
		}
	}

	if strings.Contains(ddlTemplate, "OFFSET 20 MINUTE") {
		t.Fatal("POP recovery cadence must remain OFFSET 10 MINUTE, not 20 MINUTE")
	}

	if strings.Count(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_recover_pop_impressions_v2") != 1 {
		t.Fatal("replacement POP recovery view must be created exactly once")
	}
}

func TestPopRecoveryMigrationStopsOldWritersBeforeSchemaWork(t *testing.T) {
	t.Parallel()

	dropLegacy := strings.Index(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_recover_pop_impressions SYNC")
	dropPreviousV2 := strings.Index(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_recover_pop_impressions_v2 SYNC")
	addRecoveryColumn := strings.Index(ddlTemplate, "ADD COLUMN IF NOT EXISTS is_pop_recovered UInt8 DEFAULT 0")
	catchUp := strings.Index(ddlTemplate, "POP RECOVERY TRANSITION CATCH-UP")
	createPeriodic := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_recover_pop_impressions_v2")

	if dropLegacy < 0 || dropPreviousV2 < 0 || addRecoveryColumn < 0 || catchUp < 0 || createPeriodic < 0 {
		t.Fatalf("cannot locate POP recovery migration statements: legacy=%d v2=%d column=%d catchup=%d create=%d", dropLegacy, dropPreviousV2, addRecoveryColumn, catchUp, createPeriodic)
	}
	if dropLegacy >= addRecoveryColumn || dropPreviousV2 >= addRecoveryColumn {
		t.Fatalf("all old recovery writers must be stopped before schema migration: legacy=%d v2=%d column=%d", dropLegacy, dropPreviousV2, addRecoveryColumn)
	}
	if catchUp >= createPeriodic {
		t.Fatalf("transition catch-up must complete before periodic recovery is enabled: catchup=%d create=%d", catchUp, createPeriodic)
	}
	if strings.Count(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_recover_pop_impressions SYNC") != 1 {
		t.Fatal("legacy POP recovery stop/drop must be idempotent and occur exactly once")
	}
	if strings.Count(ddlTemplate, "DROP VIEW IF EXISTS {db}.mv_recover_pop_impressions_v2 SYNC") != 1 {
		t.Fatal("previous v2 recovery stop/drop must be idempotent and occur exactly once")
	}

	ordinaryAgg := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_impressions_to_agg_stats")
	recoveryAgg := strings.Index(ddlTemplate, "CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_pop_recovered_to_agg_stats")
	if ordinaryAgg < 0 || recoveryAgg < 0 || ordinaryAgg >= catchUp || recoveryAgg >= catchUp {
		t.Fatalf("both aggregate MVs must exist before catch-up: ordinary=%d recovery=%d catchup=%d", ordinaryAgg, recoveryAgg, catchUp)
	}
}

func TestPopLateArrivalGuardIsUnboundedAndPointScoped(t *testing.T) {
	t.Parallel()

	guard := ddlSection(
		t,
		"CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_impressions_to_fact",
		"REFRESHABLE MV: CLICKS INPUT -> FACT CLICKS",
	)

	if strings.Contains(guard, "INTERVAL 2 HOUR") {
		t.Fatal("late-arrival duplicate protection must not depend on a two-hour rolling window")
	}

	for _, want := range []string{
		"pending_pop_auctions AS",
		"o.event_time AS event_time",
		"toDate(o.event_time) AS event_date",
		"PREWHERE event_date IN",
		"AND event_time IN",
		"AND (uuid, event_time) IN",
		"is_pop_recovered = 1",
		"pop_recovery_key = concat('pop-auction:', toString(uuid))",
		"i.impressions_uuid = c.clicks_uuid",
		"ifNull(fs.impression_cnt, 0) + toUInt64(a.pop_pending_rank)",
		"<= ifNull(cs.click_cnt, 0)",
	} {
		if !strings.Contains(guard, want) {
			t.Fatalf("late-arrival guard is missing %q", want)
		}
	}

	// A recovered slot whose click deficit is already closed must reject a real
	// callback regardless of elapsed wall-clock time. The SQL has no age term;
	// this small model locks the count/rank predicate used by the view.
	accept := func(recovered, legacy bool, impressionCnt, uniqueImpressionCnt, clickCnt, pendingRank uint64) bool {
		noRecoveryEvidence := !recovered && !legacy && impressionCnt == uniqueImpressionCnt
		stillUncovered := impressionCnt+pendingRank <= clickCnt
		return noRecoveryEvidence || stillUncovered
	}
	recoveredAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	lateRealAt := recoveredAt.Add(49 * time.Hour)
	if lateRealAt.Sub(recoveredAt) <= 2*time.Hour {
		t.Fatal("regression setup must exercise a callback arriving significantly later than two hours")
	}
	if accept(true, false, 1, 1, 1, 1) {
		t.Fatalf("late real impression after %s and an already-covered recovery slot would double bill", lateRealAt.Sub(recoveredAt))
	}
	if !accept(true, false, 1, 1, 2, 1) {
		t.Fatal("late-arrival guard must still allow a genuinely uncovered click slot")
	}
	if !accept(false, false, 1, 1, 1, 1) {
		t.Fatal("ordinary POP impression without recovery evidence must keep normal ingestion semantics")
	}
}

func TestPopRecoveryPreservesFullMissingAndPartialDeficitSemantics(t *testing.T) {
	t.Parallel()

	catchUp := ddlSection(
		t,
		"POP RECOVERY TRANSITION CATCH-UP",
		"REFRESHABLE MV: POP CLICK DEFICIT -> RECOVERED IMPRESSIONS",
	)
	periodic := ddlSection(
		t,
		"REFRESHABLE MV: POP CLICK DEFICIT -> RECOVERED IMPRESSIONS",
		"MV: FACT CLICKS -> AGG STATS",
	)

	for name, section := range map[string]string{"catch-up": catchUp, "periodic": periodic} {
		for _, want := range []string{
			"WHERE d.impression_cnt = 0",
			"WHERE d.impression_cnt > 0",
			"FROM full_missing_candidates AS c",
			"FROM partial_templates AS i",
			"INNER JOIN partial_recovery_sources AS s USING (uuid)",
			"c.clicks_uuid AS impressions_uuid",
			"c.win_dsp_price AS win_dsp_price",
			"c.win_cid AS win_cid",
			"c.win_user_id AS win_user_id",
			"c.site_id AS site_id",
			"c.created_at AS created_at",
			"i.impressions_uuid AS impressions_uuid",
			"i.win_dsp_price AS win_dsp_price",
			"i.win_cid AS win_cid",
			"i.win_user_id AS win_user_id",
			"i.site_id AS site_id",
			"i.format AS format",
			"i.created_at AS created_at",
			"s.recovery_source_key AS pop_recovery_source_key",
		} {
			if !strings.Contains(section, want) {
				t.Fatalf("%s recovery lost legacy field/source semantics %q", name, want)
			}
		}

		if strings.Contains(section, "now64(3, 'UTC') AS created_at") {
			t.Fatalf("%s recovery must preserve legacy created_at rather than replacing it with recovery time", name)
		}
	}
}

func TestPopRecoveredSpendOnlyCountsRecoveredRows(t *testing.T) {
	t.Parallel()

	recoveredAgg := ddlSection(
		t,
		"CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_pop_recovered_to_agg_stats",
		"MV: FACT IMPRESSIONS -> AGG STATS",
	)
	for _, want := range []string{
		"sum(win_dsp_price / 1000) AS pop_recovered_spend",
		"WHERE format = 'POP'",
		"AND is_pop_recovered = 1",
	} {
		if !strings.Contains(recoveredAgg, want) {
			t.Fatalf("recovered-spend MV is missing %q", want)
		}
	}

	ordinaryAgg := ddlSection(
		t,
		"CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_impressions_to_agg_stats",
		"POP RECOVERY TRANSITION CATCH-UP",
	)
	if !strings.Contains(ordinaryAgg, "sum(win_dsp_price / 1000) AS spend_views_table") {
		t.Fatal("ordinary impression spend semantics changed unexpectedly")
	}
	if strings.Contains(ordinaryAgg, "AS pop_recovered_spend") {
		t.Fatal("ordinary impression aggregate MV must not emit recovered spend")
	}
}

func TestPopCatchUpRemainsIdempotent(t *testing.T) {
	t.Parallel()

	catchUp := ddlSection(
		t,
		"POP RECOVERY TRANSITION CATCH-UP",
		"REFRESHABLE MV: POP CLICK DEFICIT -> RECOVERED IMPRESSIONS",
	)
	for _, want := range []string{
		"WHERE c.click_cnt > ifNull(i.impression_cnt, 0)",
		"concat('pop-click:', toString(c.clicks_uuid)) NOT IN",
		"AND is_pop_recovered = 1",
		"AND pop_recovery_source_key != ''",
		"WHERE c.recovery_rank <= c.missing_cnt",
		"WHERE s.recovery_rank <= s.missing_cnt",
	} {
		if !strings.Contains(catchUp, want) {
			t.Fatalf("catch-up idempotency contract is missing %q", want)
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

	var legacyDropSeen, previousV2DropSeen, catchUpSeen, periodicSeen bool
	for _, statement := range statements {
		switch {
		case strings.Contains(statement, "DROP VIEW IF EXISTS ads.mv_recover_pop_impressions SYNC"):
			legacyDropSeen = true
		case strings.Contains(statement, "DROP VIEW IF EXISTS ads.mv_recover_pop_impressions_v2 SYNC"):
			previousV2DropSeen = true
		case strings.Contains(statement, "POP RECOVERY TRANSITION CATCH-UP"):
			if !legacyDropSeen || !previousV2DropSeen {
				t.Fatal("transition catch-up was split before old recovery writers were stopped")
			}
			catchUpSeen = true
		case strings.Contains(statement, "CREATE MATERIALIZED VIEW IF NOT EXISTS ads.mv_recover_pop_impressions_v2"):
			if !catchUpSeen {
				t.Fatal("periodic recovery was enabled before transition catch-up")
			}
			periodicSeen = true
		}
	}
	if !legacyDropSeen || !previousV2DropSeen || !catchUpSeen || !periodicSeen {
		t.Fatalf("missing recovery statements after split: legacy_drop=%t v2_drop=%t catchup=%t periodic=%t", legacyDropSeen, previousV2DropSeen, catchUpSeen, periodicSeen)
	}
}
