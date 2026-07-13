package app

import (
	"context"
	"database/sql"
	dbpkg "gitlab.com/twinbid-exchange/RTB-exchange/internal/db"
)

type App struct{ DB *sql.DB }

func New(ctx context.Context, dsn string) (*App, error) {
	db, err := dbpkg.InitDBAndMigrate(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &App{DB: db}, nil
}
func (a *App) Close() error {
	if a == nil || a.DB == nil {
		return nil
	}
	return a.DB.Close()
}
