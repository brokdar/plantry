// Package kitchen holds the foods the household always has at home (KIT-4)
// and the foods it never eats (TASTE-1). Both are lists of foods by
// reference; the kitchen settings themselves live on the profile.
package kitchen

import "context"

// Food is a listed food.
type Food struct {
	ID   int64
	Name string
}

// Repository is the port adapters implement. Add is idempotent and returns
// domain.ErrNotFound for an unknown food; Remove of an absent food is not an
// error.
type Repository interface {
	Basics(ctx context.Context) ([]Food, error)
	AddBasic(ctx context.Context, foodID int64) error
	RemoveBasic(ctx context.Context, foodID int64) error
	NeverItems(ctx context.Context) ([]Food, error)
	AddNeverItem(ctx context.Context, foodID int64) error
	RemoveNeverItem(ctx context.Context, foodID int64) error
}

// Service manages the basics at home. The never items are read and written
// by later work through the repository.
type Service struct{ repo Repository }

// NewService creates a kitchen service.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Basics lists the foods at home, by name.
func (s *Service) Basics(ctx context.Context) ([]Food, error) { return s.repo.Basics(ctx) }

// AddBasic marks a food as always at home.
func (s *Service) AddBasic(ctx context.Context, foodID int64) error {
	return s.repo.AddBasic(ctx, foodID)
}

// RemoveBasic takes a food off the list.
func (s *Service) RemoveBasic(ctx context.Context, foodID int64) error {
	return s.repo.RemoveBasic(ctx, foodID)
}
