package plate

import (
	"context"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// SlotChecker reports whether a time slot exists.
type SlotChecker interface {
	Exists(ctx context.Context, slotID int64) (bool, error)
}

// FoodChecker reports whether a food exists.
type FoodChecker interface {
	Exists(ctx context.Context, foodID int64) (bool, error)
}

// Service holds business logic for plates and their components.
type Service struct {
	repo  Repository
	slots SlotChecker
	foods FoodChecker
}

// NewService creates a plate service.
func NewService(repo Repository, slots SlotChecker, foods FoodChecker) *Service {
	return &Service{repo: repo, slots: slots, foods: foods}
}

func (s *Service) validatePlate(ctx context.Context, p *Plate) error {
	if p.Date.IsZero() {
		if !ValidDay(p.Day) {
			return fmt.Errorf("%w: day must be between 0 and 6", domain.ErrInvalidDay)
		}
		if p.WeekID <= 0 {
			return fmt.Errorf("%w: date or week_id required", domain.ErrInvalidInput)
		}
	}
	if p.SlotID <= 0 {
		return fmt.Errorf("%w: slot_id required", domain.ErrSlotUnknown)
	}
	ok, err := s.slots.Exists(ctx, p.SlotID)
	if err != nil {
		return fmt.Errorf("check slot: %w", err)
	}
	if !ok {
		return fmt.Errorf("%w: slot %d does not exist", domain.ErrSlotUnknown, p.SlotID)
	}
	for i, pc := range p.Components {
		if pc.FoodID <= 0 {
			return fmt.Errorf("%w: component[%d] food_id required", domain.ErrInvalidInput, i)
		}
		if pc.Portions <= 0 {
			return fmt.Errorf("%w: component[%d] portions must be positive", domain.ErrInvalidInput, i)
		}
		exists, err := s.foods.Exists(ctx, pc.FoodID)
		if err != nil {
			return fmt.Errorf("check food %d: %w", pc.FoodID, err)
		}
		if !exists {
			return fmt.Errorf("%w: food %d does not exist", domain.ErrNotFound, pc.FoodID)
		}
	}
	return nil
}

// Create validates and persists a new plate, optionally with initial components.
func (s *Service) Create(ctx context.Context, p *Plate) error {
	if err := s.validatePlate(ctx, p); err != nil {
		return err
	}
	for i := range p.Components {
		if p.Components[i].Portions == 0 {
			p.Components[i].Portions = 1
		}
		p.Components[i].SortOrder = i
	}
	return s.repo.Create(ctx, p)
}

// Get returns a plate with its components.
func (s *Service) Get(ctx context.Context, id int64) (*Plate, error) {
	return s.repo.Get(ctx, id)
}

// Update persists changes to day/slot/note. Child mutations go through their own methods.
func (s *Service) Update(ctx context.Context, p *Plate) error {
	if !ValidDay(p.Day) {
		return fmt.Errorf("%w: day must be between 0 and 6", domain.ErrInvalidDay)
	}
	if p.SlotID <= 0 {
		return fmt.Errorf("%w: slot_id required", domain.ErrSlotUnknown)
	}
	ok, err := s.slots.Exists(ctx, p.SlotID)
	if err != nil {
		return fmt.Errorf("check slot: %w", err)
	}
	if !ok {
		return fmt.Errorf("%w: slot %d does not exist", domain.ErrSlotUnknown, p.SlotID)
	}
	return s.repo.Update(ctx, p)
}

// Delete removes a plate (cascades to plate_components via FK).
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListByWeek returns all plates for a week, with their components loaded.
func (s *Service) ListByWeek(ctx context.Context, weekID int64) ([]Plate, error) {
	return s.repo.ListByWeek(ctx, weekID)
}

// AddComponent appends a food to a plate at the next sort_order.
func (s *Service) AddComponent(ctx context.Context, plateID, foodID int64, portions float64) (*PlateComponent, error) {
	if portions <= 0 {
		portions = 1
	}
	if _, err := s.repo.Get(ctx, plateID); err != nil {
		return nil, err
	}
	exists, err := s.foods.Exists(ctx, foodID)
	if err != nil {
		return nil, fmt.Errorf("check food: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: food %d does not exist", domain.ErrNotFound, foodID)
	}
	existing, err := s.repo.ListComponentsByPlate(ctx, plateID)
	if err != nil {
		return nil, err
	}
	next := 0
	for _, e := range existing {
		if e.SortOrder >= next {
			next = e.SortOrder + 1
		}
	}
	pc := &PlateComponent{
		PlateID:   plateID,
		FoodID:    foodID,
		Portions:  portions,
		SortOrder: next,
	}
	if err := s.repo.CreateComponent(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// SwapComponent replaces the food on an existing plate_component row, preserving sort_order.
// If portionsOverride is nil, the existing portions are kept.
func (s *Service) SwapComponent(ctx context.Context, plateComponentID, newFoodID int64, portionsOverride *float64) (*PlateComponent, error) {
	pc, err := s.repo.GetComponent(ctx, plateComponentID)
	if err != nil {
		return nil, err
	}
	exists, err := s.foods.Exists(ctx, newFoodID)
	if err != nil {
		return nil, fmt.Errorf("check food: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: food %d does not exist", domain.ErrNotFound, newFoodID)
	}
	pc.FoodID = newFoodID
	if portionsOverride != nil {
		if *portionsOverride <= 0 {
			return nil, fmt.Errorf("%w: portions must be positive", domain.ErrInvalidInput)
		}
		pc.Portions = *portionsOverride
	}
	if err := s.repo.UpdateComponent(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// UpdateComponentPortions changes the portions on a plate_component row.
func (s *Service) UpdateComponentPortions(ctx context.Context, plateComponentID int64, portions float64) (*PlateComponent, error) {
	if portions <= 0 {
		return nil, fmt.Errorf("%w: portions must be positive", domain.ErrInvalidInput)
	}
	pc, err := s.repo.GetComponent(ctx, plateComponentID)
	if err != nil {
		return nil, err
	}
	pc.Portions = portions
	if err := s.repo.UpdateComponent(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// RemoveComponent removes a plate_component row.
func (s *Service) RemoveComponent(ctx context.Context, plateComponentID int64) error {
	return s.repo.DeleteComponent(ctx, plateComponentID)
}

// SetSkipped marks the slot as prospectively skipped (eating out / canteen).
// Clears attached components atomically when enabling skip.
func (s *Service) SetSkipped(ctx context.Context, plateID int64, skipped bool, note *string) (*Plate, error) {
	return s.repo.SetSkipped(ctx, plateID, skipped, note)
}

// DeleteByWeek clears every plate in a week. Used by the Fill-empty revert flow
// to restore the pre-snapshot state.
func (s *Service) DeleteByWeek(ctx context.Context, weekID int64) (int64, error) {
	return s.repo.DeleteByWeek(ctx, weekID)
}

// Range returns all plates in [from, to] inclusive. from must be ≤ to; span must be ≤ 366 days.
func (s *Service) Range(ctx context.Context, from, to time.Time) ([]Plate, error) {
	if from.After(to) {
		return nil, fmt.Errorf("%w: from must not be after to", domain.ErrInvalidInput)
	}
	if to.Sub(from) > 366*24*time.Hour {
		return nil, fmt.Errorf("%w: range exceeds 366 days", domain.ErrInvalidInput)
	}
	return s.repo.ListByDateRange(ctx, from, to)
}

// Day returns all plates for a single date.
func (s *Service) Day(ctx context.Context, date time.Time) ([]Plate, error) {
	return s.Range(ctx, date, date)
}
