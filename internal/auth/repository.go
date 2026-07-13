package auth

import (
	"context"
	"database/sql"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/models"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) UserByID(ctx context.Context, id int64) (*models.User, error) {
	var u models.User
	err := r.db.QueryRowContext(ctx, `SELECT id,email,name,goal,spent,low_balance_notified,created_at,updated_at FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Email, &u.Name, &u.Goal, &u.Spent, &u.LowBalanceNotified, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.ComputeBalance()
	return &u, nil
}
func (r *Repository) UserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := r.db.QueryRowContext(ctx, `SELECT id,email,name,goal,spent,low_balance_notified,created_at,updated_at FROM users WHERE email=$1`, email).Scan(&u.ID, &u.Email, &u.Name, &u.Goal, &u.Spent, &u.LowBalanceNotified, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.ComputeBalance()
	return &u, nil
}
