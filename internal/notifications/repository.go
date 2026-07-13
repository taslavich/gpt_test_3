package notifications

import (
	"context"
	"database/sql"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) LowBalanceUsers(ctx context.Context, threshold float64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM users WHERE goal - spent < $1 AND low_balance_notified=false`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (r *Repository) IsLowBalance(ctx context.Context, userID int64, threshold float64) (bool, error) {
	var low bool
	err := r.db.QueryRowContext(ctx, `SELECT goal - spent < $1 FROM users WHERE id=$2`, threshold, userID).Scan(&low)
	return low, err
}
