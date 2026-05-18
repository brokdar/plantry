package plate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// Quantity-validation sentinel errors. All wrap domain.ErrInvalidInput so the
// existing HTTP error map turns them into 400s; specific keys are surfaced by
// the handler error map for i18n.
var (
	// ErrInvalidQuantityShape is returned when a request has neither portions
	// nor (amount + unit), or has both at once.
	ErrInvalidQuantityShape = errors.New("plate component must specify exactly one of portions or (amount, unit)")
	// ErrInvalidQuantityForComposed is returned when a composed-food component
	// carries amount/unit instead of integer portions.
	ErrInvalidQuantityForComposed = errors.New("composed food on a plate requires integer portions, not amount/unit")
	// ErrInvalidQuantityForLeaf is returned when a leaf-food component carries
	// portions instead of (amount + unit).
	ErrInvalidQuantityForLeaf = errors.New("leaf food on a plate requires amount + unit, not portions")
	// ErrUnitRequiresPortion is returned when a leaf component's unit is a
	// count unit (e.g., "slice") that the food has no portion override for and
	// no universal mass/volume default exists.
	ErrUnitRequiresPortion = errors.New("unit requires a per-food portion override")
)

// SlotChecker reports whether a time slot exists.
type SlotChecker interface {
	Exists(ctx context.Context, slotID int64) (bool, error)
}

// FoodLookup is the food-side surface the plate service depends on:
// kind discrimination + portion overrides for grams resolution.
// food.Repository satisfies it directly via its Get / ListPortions methods;
// the SQLite FoodRepo also exposes a cheaper KindOf.
type FoodLookup interface {
	Get(ctx context.Context, foodID int64) (*food.Food, error)
	ListPortions(ctx context.Context, foodID int64) ([]food.Portion, error)
}

// Service holds business logic for plates and their components.
type Service struct {
	repo  Repository
	slots SlotChecker
	foods FoodLookup
}

// NewService creates a plate service.
func NewService(repo Repository, slots SlotChecker, foods FoodLookup) *Service {
	return &Service{repo: repo, slots: slots, foods: foods}
}

func (s *Service) validatePlate(ctx context.Context, p *Plate) error {
	if p.Date.IsZero() {
		return fmt.Errorf("%w: date required", domain.ErrInvalidInput)
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
	for i := range p.Components {
		pc := &p.Components[i]
		if pc.FoodID <= 0 {
			return fmt.Errorf("%w: component[%d] food_id required", domain.ErrInvalidInput, i)
		}
		if err := s.normaliseComponent(ctx, pc); err != nil {
			return fmt.Errorf("component[%d]: %w", i, err)
		}
	}
	return nil
}

// normaliseComponent validates the quantity shape against the food's kind and
// resolves leaf-component grams in place. It must be called before any write.
func (s *Service) normaliseComponent(ctx context.Context, pc *PlateComponent) error {
	hasPortions := pc.Portions != nil
	hasLeaf := pc.Amount != nil || pc.Unit != nil
	if hasPortions && hasLeaf {
		return fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrInvalidQuantityShape)
	}
	if !hasPortions && !hasLeaf {
		return fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrInvalidQuantityShape)
	}

	f, err := s.foods.Get(ctx, pc.FoodID)
	if err != nil {
		// food.Repository wraps not-found as domain.ErrNotFound already.
		return err
	}

	switch f.Kind {
	case food.KindComposed:
		if hasLeaf {
			return fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrInvalidQuantityForComposed)
		}
		if *pc.Portions <= 0 {
			return fmt.Errorf("%w: portions must be positive", domain.ErrInvalidInput)
		}
		// Composed quantity is integer-only; nothing else to fill in.
		pc.Amount = nil
		pc.Unit = nil
		pc.Grams = nil
		pc.GramsSource = nil
		return nil

	case food.KindLeaf:
		if hasPortions {
			return fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrInvalidQuantityForLeaf)
		}
		if pc.Amount == nil || *pc.Amount <= 0 {
			return fmt.Errorf("%w: amount must be positive", domain.ErrInvalidInput)
		}
		if pc.Unit == nil || *pc.Unit == "" {
			return fmt.Errorf("%w: unit required", domain.ErrInvalidInput)
		}
		manualGrams := float64(0)
		if pc.Grams != nil {
			manualGrams = *pc.Grams
		}
		grams, source, err := food.ResolveGrams(ctx, s.foods, pc.FoodID, *pc.Amount, *pc.Unit, manualGrams)
		if err != nil {
			// Surface a specific key when the failure is the count-unit /
			// missing-portion case; otherwise let the generic ErrInvalidInput
			// flow through.
			if errors.Is(err, domain.ErrInvalidInput) {
				return fmt.Errorf("%w: %w", err, ErrUnitRequiresPortion)
			}
			return err
		}
		// Persist the canonical unit key the resolver used.
		canonical := *pc.Unit
		if normalized := units.Normalize(canonical); normalized != "" {
			canonical = normalized
		}
		pc.Unit = &canonical
		pc.Grams = &grams
		src := source
		pc.GramsSource = &src
		pc.Portions = nil
		return nil

	default:
		return fmt.Errorf("%w: food %d has unknown kind %q", domain.ErrInvalidInput, pc.FoodID, f.Kind)
	}
}

// Create validates and persists a new plate, optionally with initial components.
func (s *Service) Create(ctx context.Context, p *Plate) error {
	if err := s.validatePlate(ctx, p); err != nil {
		return err
	}
	for i := range p.Components {
		p.Components[i].SortOrder = i
	}
	return s.repo.Create(ctx, p)
}

// Get returns a plate with its components.
func (s *Service) Get(ctx context.Context, id int64) (*Plate, error) {
	return s.repo.Get(ctx, id)
}

// Update persists changes to date/slot/note. Child mutations go through their own methods.
func (s *Service) Update(ctx context.Context, p *Plate) error {
	if p.Date.IsZero() {
		return fmt.Errorf("%w: date required", domain.ErrInvalidInput)
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

// AddComponent appends a food to a plate at the next sort_order. The supplied
// component's quantity (portions xor amount+unit) is validated against the
// food's kind, and grams are resolved server-side for leaf components.
func (s *Service) AddComponent(ctx context.Context, plateID int64, pc *PlateComponent) (*PlateComponent, error) {
	if pc == nil {
		return nil, fmt.Errorf("%w: component required", domain.ErrInvalidInput)
	}
	if _, err := s.repo.Get(ctx, plateID); err != nil {
		return nil, err
	}
	if pc.FoodID <= 0 {
		return nil, fmt.Errorf("%w: food_id required", domain.ErrInvalidInput)
	}
	if err := s.normaliseComponent(ctx, pc); err != nil {
		return nil, err
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
	pc.PlateID = plateID
	pc.SortOrder = next
	if err := s.repo.CreateComponent(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// SwapComponent replaces the food on an existing plate_component row, preserving
// sort_order. The new quantity must match the new food's kind. If quantityOverride
// is nil the existing quantity is reused, but only if it still matches the new
// food's kind — otherwise the caller must supply a valid quantity for the swap.
func (s *Service) SwapComponent(ctx context.Context, plateComponentID, newFoodID int64, quantityOverride *PlateComponent) (*PlateComponent, error) {
	pc, err := s.repo.GetComponent(ctx, plateComponentID)
	if err != nil {
		return nil, err
	}
	pc.FoodID = newFoodID
	if quantityOverride != nil {
		pc.Portions = quantityOverride.Portions
		pc.Amount = quantityOverride.Amount
		pc.Unit = quantityOverride.Unit
		// Grams is a candidate manual value the resolver may use.
		pc.Grams = quantityOverride.Grams
		pc.GramsSource = nil
	}
	if err := s.normaliseComponent(ctx, pc); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateComponent(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// GetComponent returns the stored plate_component row by id. Used by callers
// (notably the agent tools) that need to discover the component's food before
// constructing a kind-aware quantity for an update.
func (s *Service) GetComponent(ctx context.Context, plateComponentID int64) (*PlateComponent, error) {
	return s.repo.GetComponent(ctx, plateComponentID)
}

// UpdateComponentQuantity changes the quantity (portions or amount+unit) on a
// plate_component row. The new quantity must match the food's kind and grams
// are re-resolved for leaf updates.
func (s *Service) UpdateComponentQuantity(ctx context.Context, plateComponentID int64, q PlateComponent) (*PlateComponent, error) {
	pc, err := s.repo.GetComponent(ctx, plateComponentID)
	if err != nil {
		return nil, err
	}
	pc.Portions = q.Portions
	pc.Amount = q.Amount
	pc.Unit = q.Unit
	pc.Grams = q.Grams
	pc.GramsSource = nil
	if err := s.normaliseComponent(ctx, pc); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateComponent(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// RemoveComponent removes a plate_component row.
func (s *Service) RemoveComponent(ctx context.Context, plateComponentID int64) error {
	return s.repo.DeleteComponent(ctx, plateComponentID)
}

// RecentUnitForFood returns the most-recently-used canonical unit for the
// given leaf food across any plate, or "" if none exists. Used by the
// planner picker to default the quantity-unit input to whatever the user
// last picked for this ingredient.
func (s *Service) RecentUnitForFood(ctx context.Context, foodID int64) (string, error) {
	return s.repo.RecentUnitForFood(ctx, foodID)
}

// SetSkipped marks the slot as prospectively skipped (eating out / canteen).
// Clears attached components atomically when enabling skip.
func (s *Service) SetSkipped(ctx context.Context, plateID int64, skipped bool, note *string) (*Plate, error) {
	return s.repo.SetSkipped(ctx, plateID, skipped, note)
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
