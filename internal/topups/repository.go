package topups

import (
	"context"
	"database/sql"
	"errors"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository                         { return &Repository{db: db} }
func (r *Repository) BeginTx(ctx context.Context) (*sql.Tx, error) { return r.db.BeginTx(ctx, nil) }
func (r *Repository) ApproveTx(ctx context.Context, tx *sql.Tx, id int64) (userID int64, amount float64, err error) {
	if tx == nil {
		return 0, 0, errors.New("tx is nil")
	}
	err = tx.QueryRowContext(ctx, `UPDATE topups SET status='approved', approved_at=NOW(), updated_at=NOW() WHERE id=$1 AND status='pending' RETURNING user_id, amount`, id).Scan(&userID, &amount)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrTopupNotPending
	}
	return userID, amount, err
}

var ErrTopupNotPending = errors.New("topup is not pending")
