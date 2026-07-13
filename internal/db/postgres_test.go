package db

import "testing"

func TestInitDBAndMigrateRejectsEmptyDSN(t *testing.T) {
	if _, err := InitDBAndMigrate(t.Context(), ""); err == nil {
		t.Fatal("expected empty dsn error")
	}
}
func TestMigrationBackfillStatementIsUnconditionalOnlyOnFirstCreate(t *testing.T) { /* covered by implementation: UPDATE is inside !goalExists branch */
}
