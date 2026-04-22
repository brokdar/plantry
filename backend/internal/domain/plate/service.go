package plate

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// SlotChecker reports whether a time slot exists.
type SlotChecker interface {
	Exists(ctx context.Context, slotID int64) (bool, error)
}

// ComponentChecker reports whether a component exists.
type ComponentChecker interface {
	Exists(ctx context.Context, componentID int64) (bool, error)
}

// Service holds business logic for plates and their components.
type Service struct {
	repo       Repository
	slots      SlotChecker
	components ComponentChecker
}

// NewService creates a plate service.
func NewService(repo Repository, slots SlotChecker, components ComponentChecker) *Service {
	return &Service{repo: repo, slots: slots, components: components}
}

func (s *Service) validatePlate(ctx context.Context, p *Plate) error {
	if !ValidDay(p.Day) {
		return fmt.Errorf("%w: day must be between 0 and 6", domain.ErrInvalidDay)
	}
	if p.WeekID <= 0 {
		return fmt.Errorf("%w: week_id required", domain.ErrInvalidInput)
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
		if pc.ComponentID <= 0 {
			return fmt.Errorf("%w: component[%d] id required", domain.ErrInvalidInput, i)
		}
		if pc.Portions <= 0 {
			return fmt.Errorf("%w: component[%d] portions must be positive", domain.ErrInvalidInput, i)
		}
		exists, err := s.components.Exists(ctx, pc.ComponentID)
		if err != nil {
			return fmt.Errorf("check component %d: %w", pc.ComponentID, err)
		}
		if !exists {
			return fmt.Errorf("%w: component %d does not exist", domain.ErrNotFound, pc.ComponentID)
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

// AddComponent appends a component to a plate at the next sort_order.
func (s *Service) AddComponent(ctx context.Context, plateID, componentID int64, portions float64) (*PlateComponent, error) {
	if portions <= 0 {
		portions = 1
	}
	if _, err := s.repo.Get(ctx, plateID); err != nil {
		return nil, err
	}
	exists, err := s.components.Exists(ctx, componentID)
	if err != nil {
		return nil, fmt.Errorf("check component: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: component %d does not exist", domain.ErrNotFound, componentID)
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
		PlateID:     plateID,
		ComponentID: componentID,
		Portions:    portions,
		SortOrder:   next,
	}
	if err := s.repo.CreateComponent(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// SwapComponent replaces the component on an existing plate_component row, preserving sort_order.
// If portionsOverride is nil, the existing portions are kept.
func (s *Service) SwapComponent(ctx context.Context, plateComponentID, newComponentID int64, portionsOverride *float64) (*PlateComponent, error) {
	pc, err := s.repo.GetComponent(ctx, plateComponentID)
	if err != nil {
		return nil, err
	}
	exists, err := s.components.Exists(ctx, newComponentID)
	if err != nil {
		return nil, fmt.Errorf("check component: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: component %d does not exist", domain.ErrNotFound, newComponentID)
	}
	pc.ComponentID = newComponentID
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
// Clears attached components atomically when enabling skip; a component-add
// later implicitly un-skips via AddComponent unhandled — callers must unset
// skip first.
func (s *Service) SetSkipped(ctx context.Context, plateID int64, skipped bool, note *string) (*Plate, error) {
	return s.repo.SetSkipped(ctx, plateID, skipped, note)
}

// DeleteByWeek clears every plate in a week. Used by the Fill-empty revert flow
// to restore the pre-snapshot state.
func (s *Service) DeleteByWeek(ctx context.Context, weekID int64) (int64, error) {
	return s.repo.DeleteByWeek(ctx, weekID)
}
