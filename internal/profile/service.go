package profile

import (
	"context"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/models"
)

type Service struct{ repo *Repository }

func NewService(r *Repository) *Service { return &Service{repo: r} }
func (s *Service) Get(ctx context.Context, id int64) (*models.User, error) {
	return s.repo.Get(ctx, id)
}
func (s *Service) Patch(ctx context.Context, id int64, req PatchUserRequest) (*models.User, error) {
	return s.repo.Patch(ctx, id, req)
}
