package clickhouse_loader

import (
	"strings"
	"testing"
)

func TestSplitClickHouseStatementsIgnoresSemicolonsInCommentsAndLiterals(t *testing.T) {
	t.Parallel()

	sql := `
-- comment before first statement; this semicolon must not split
CREATE TABLE test.one (value String DEFAULT 'a;b');

-- Kafka replay comment; unlike the old strings.Split implementation this is safe.
CREATE VIEW test.one_logical AS
SELECT "x;y" AS value, ` + "`z;w`" + ` AS ident;

/* block comment; also not a statement */
-- trailing comment only; must not be executed
`

	statements := splitClickHouseStatements(sql)
	if len(statements) != 2 {
		t.Fatalf("expected 2 executable statements, got %d: %#v", len(statements), statements)
	}

	if !strings.Contains(statements[0], "CREATE TABLE test.one") {
		t.Fatalf("first statement was split incorrectly: %q", statements[0])
	}
	if !strings.Contains(statements[0], "'a;b'") {
		t.Fatalf("semicolon in single-quoted literal was not preserved: %q", statements[0])
	}
	if !strings.Contains(statements[1], "CREATE VIEW test.one_logical") {
		t.Fatalf("second statement was split incorrectly: %q", statements[1])
	}
	if !strings.Contains(statements[1], `"x;y"`) || !strings.Contains(statements[1], "`z;w`") {
		t.Fatalf("semicolon in quoted identifiers/literals was not preserved: %q", statements[1])
	}
}

func TestSplitClickHouseStatementsRegressionORTBLogicalComment(t *testing.T) {
	t.Parallel()

	sql := `
ALTER TABLE ads.ortb ADD INDEX IF NOT EXISTS idx_ortb_uuid uuid TYPE bloom_filter(0.01) GRANULARITY 1;

-- Kafka offset commit happens after the ClickHouse insert, so a process crash
-- can replay the same ORTB message. Keep the physical append-only table, but
-- expose one row per stable logical event synchronously at read time; unlike a
-- ReplacingMergeTree-only solution this does not wait for a background merge.
CREATE VIEW ads.ortb_logical AS
SELECT *
FROM ads.ortb
ORDER BY created_at ASC
LIMIT 1 BY logical_event_id;
`

	statements := splitClickHouseStatements(sql)
	if len(statements) != 2 {
		t.Fatalf("expected ALTER + CREATE VIEW, got %d statements: %#v", len(statements), statements)
	}
	if strings.TrimSpace(statements[1]) == "" || !strings.Contains(statements[1], "CREATE VIEW ads.ortb_logical") {
		t.Fatalf("ortb_logical CREATE VIEW must stay attached to its leading comments: %q", statements[1])
	}
}
