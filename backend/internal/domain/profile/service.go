package profile

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Service holds business logic for the user profile.
type Service struct {
	repo Repository
}

// NewService creates a profile service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Get returns the current profile.
func (s *Service) Get(ctx context.Context) (*Profile, error) {
	return s.repo.Get(ctx)
}

// Update validates and persists the profile.
func (s *Service) Update(ctx context.Context, p *Profile) (*Profile, error) {
	if err := validate(p); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, p)
}

// MaxHouseholdPortions caps Profile.HouseholdPortions.
const MaxHouseholdPortions = 20

func validate(p *Profile) error {
	if !p.DietType.Valid() {
		return fmt.Errorf("%w: %q", domain.ErrInvalidDietType, p.DietType)
	}
	if p.MeatDaysPerWeekMax != nil && (*p.MeatDaysPerWeekMax < 0 || *p.MeatDaysPerWeekMax > 7) {
		return fmt.Errorf("%w: must be between 0 and 7", domain.ErrInvalidMeatDays)
	}
	if p.HouseholdPortions <= 0 || p.HouseholdPortions > MaxHouseholdPortions {
		return fmt.Errorf("%w: must be > 0 and <= %d", domain.ErrInvalidHouseholdPortions, MaxHouseholdPortions)
	}

	if p.KcalTarget != nil && *p.KcalTarget <= 0 {
		return fmt.Errorf("%w: kcal_target must be positive", domain.ErrInvalidMacros)
	}

	var sum float64
	if p.ProteinPct != nil {
		if *p.ProteinPct < 0 {
			return fmt.Errorf("%w: protein_pct must be non-negative", domain.ErrInvalidMacros)
		}
		sum += *p.ProteinPct
	}
	if p.FatPct != nil {
		if *p.FatPct < 0 {
			return fmt.Errorf("%w: fat_pct must be non-negative", domain.ErrInvalidMacros)
		}
		sum += *p.FatPct
	}
	if p.CarbsPct != nil {
		if *p.CarbsPct < 0 {
			return fmt.Errorf("%w: carbs_pct must be non-negative", domain.ErrInvalidMacros)
		}
		sum += *p.CarbsPct
	}
	if sum > 100 {
		return fmt.Errorf("%w: macro percentages sum to %.1f (max 100)", domain.ErrInvalidMacros, sum)
	}

	return nil
}
