package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/models"
	"strings"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

const userCols = `id, email, name, goal, spent, low_balance_notified, created_at, updated_at`

func scanUser(s interface{ Scan(...any) error }) (*models.User, error) {
	var u models.User
	err := s.Scan(&u.ID, &u.Email, &u.Name, &u.Goal, &u.Spent, &u.LowBalanceNotified, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.ComputeBalance()
	return &u, nil
}
func (r *Repository) Get(ctx context.Context, id int64) (*models.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
}
func (r *Repository) Patch(ctx context.Context, id int64, req PatchUserRequest) (*models.User, error) {
	sets := []string{}
	args := []any{}
	n := 1
	if req.Email != nil {
		sets = append(sets, fmt.Sprintf("email=$%d", n))
		args = append(args, *req.Email)
		n++
	}
	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name=$%d", n))
		args = append(args, *req.Name)
		n++
	}
	// Intentionally ignore Balance/Goal/Spent in generic profile patch.
	if len(sets) == 0 {
		return r.Get(ctx, id)
	}
	sets = append(sets, "updated_at=NOW()")
	args = append(args, id)
	q := `UPDATE users SET ` + strings.Join(sets, ", ") + fmt.Sprintf(` WHERE id=$%d RETURNING `+userCols, n)
	return scanUser(r.db.QueryRowContext(ctx, q, args...))
}
func (r *Repository) IncrementGoalTx(ctx context.Context, tx *sql.Tx, userID int64, amount float64, lowThreshold float64) (*models.User, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if amount <= 0 {
		return nil, errors.New("amount must be positive")
	}
	return scanUser(tx.QueryRowContext(ctx, `UPDATE users SET goal=goal+$1, low_balance_notified=CASE WHEN goal + $1 - spent >= $2 THEN false ELSE low_balance_notified END, updated_at=NOW() WHERE id=$3 RETURNING `+userCols, amount, lowThreshold, userID))
}
