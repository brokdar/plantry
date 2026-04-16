package slot

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Service holds business logic for time slots.
type Service struct {
	repo Repository
}

// NewService creates a slot service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) validate(t *TimeSlot) error {
	if t.NameKey == "" {
		return fmt.Errorf("%w: name_key required", domain.ErrInvalidInput)
	}
	if t.Icon == "" {
		return fmt.Errorf("%w: icon required", domain.ErrInvalidInput)
	}
	return nil
}

// Create validates and persists a new time slot.
func (s *Service) Create(ctx context.Context, t *TimeSlot) error {
	if err := s.validate(t); err != nil {
		return err
	}
	return s.repo.Create(ctx, t)
}

// Get retrieves a time slot by ID.
func (s *Service) Get(ctx context.Context, id int64) (*TimeSlot, error) {
	return s.repo.Get(ctx, id)
}

// Update validates and persists changes.
func (s *Service) Update(ctx context.Context, t *TimeSlot) error {
	if err := s.validate(t); err != nil {
		return err
	}
	return s.repo.Update(ctx, t)
}

// Delete removes a time slot by ID. Returns ErrInUse if any plate references it.
func (s *Service) Delete(ctx context.Context, id int64) error {
	count, err := s.repo.CountPlatesUsing(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: time slot %d is used by %d plates", domain.ErrInUse, id, count)
	}
	return s.repo.Delete(ctx, id)
}

// List returns time slots, ordered by sort_order then id.
func (s *Service) List(ctx context.Context, activeOnly bool) ([]TimeSlot, error) {
	return s.repo.List(ctx, activeOnly)
}
