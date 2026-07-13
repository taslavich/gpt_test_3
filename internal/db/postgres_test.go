package db

import (
	"os"
	"strings"
	"testing"
)

func TestInitDBAndMigrateRejectsEmptyDSN(t *testing.T) {
	if _, err := InitDBAndMigrate(t.Context(), ""); err == nil {
		t.Fatal("expected empty dsn error")
	}
}

func TestMigrateUserGoalSpentIsTransactional(t *testing.T) {
	src := readPostgresSource(t)
	for _, want := range []string{"db.BeginTx", "defer tx.Rollback()", "tx.Commit()"} {
		if !strings.Contains(src, want) {
			t.Fatalf("migration source missing %q", want)
		}
	}
}

func TestMigrateUserGoalSpentBackfillsOnlyWhenGoalIsCreated(t *testing.T) {
	src := readPostgresSource(t)
	goalBranch := strings.Index(src, "if !goalExists {")
	backfill := strings.Index(src, "UPDATE users SET goal = COALESCE(balance, 0) WHERE balance IS NOT NULL")
	spentCheck := strings.Index(src, "spentExists, err := columnExists")
	if goalBranch < 0 || backfill < 0 || spentCheck < 0 {
		t.Fatal("migration source missing goal branch, backfill, or spent check")
	}
	if !(goalBranch < backfill && backfill < spentCheck) {
		t.Fatal("goal backfill must run only inside first-create goal branch before spent migration")
	}
}

func TestMigrateUserGoalSpentRepeatedStartupDoesNotOverwriteGoal(t *testing.T) {
	src := readPostgresSource(t)
	if strings.Count(src, "UPDATE users SET goal = COALESCE(balance, 0) WHERE balance IS NOT NULL") != 1 {
		t.Fatal("expected exactly one goal backfill statement")
	}
	if strings.Contains(src, "ON CONFLICT") {
		t.Fatal("migration should not upsert or overwrite existing goal values")
	}
}

func readPostgresSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("postgres.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
