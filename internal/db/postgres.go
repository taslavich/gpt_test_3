package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

func InitDBAndMigrate(ctx context.Context, dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("postgres dsn is empty")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := MigrateUserGoalSpent(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func MigrateUserGoalSpent(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("db is nil")
	}
	goalExists, err := columnExists(ctx, db, "users", "goal")
	if err != nil {
		return err
	}
	if !goalExists {
		if _, err := db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN goal NUMERIC(18,6) NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `UPDATE users SET goal = COALESCE(balance, 0) WHERE balance IS NOT NULL`); err != nil {
			return err
		}
	}
	spentExists, err := columnExists(ctx, db, "users", "spent")
	if err != nil {
		return err
	}
	if !spentExists {
		if _, err := db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN spent NUMERIC(18,6) NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return nil
}

func columnExists(ctx context.Context, db *sql.DB, tableName, columnName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = ANY (current_schemas(false)) AND table_name = $1 AND column_name = $2)`, tableName, columnName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", tableName, columnName, err)
	}
	return exists, nil
}
