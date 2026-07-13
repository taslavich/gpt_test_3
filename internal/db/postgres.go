package db

import (
	"context"
	"database/sql"
)

func MigrateUserGoalSpent(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS goal NUMERIC(18,6) NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS spent NUMERIC(18,6) NOT NULL DEFAULT 0`,
		`UPDATE users SET goal = COALESCE(balance, 0) WHERE goal = 0 AND balance IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
