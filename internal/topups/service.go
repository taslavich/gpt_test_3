package topups

import (
	"context"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/models"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/profile"
)

type Service struct {
	repo         *Repository
	profiles     *profile.Repository
	lowThreshold float64
}

func NewService(r *Repository, p *profile.Repository, threshold float64) *Service {
	return &Service{repo: r, profiles: p, lowThreshold: threshold}
}
func (s *Service) Approve(ctx context.Context, topupID int64) (*models.User, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	uid, amount, err := s.repo.ApproveTx(ctx, tx, topupID)
	if err != nil {
		return nil, err
	}
	u, err := s.profiles.IncrementGoalTx(ctx, tx, uid, amount, s.lowThreshold)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return u, nil
}
