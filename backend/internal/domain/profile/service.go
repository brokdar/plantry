package profile

import (
	"context"
	"fmt"
	"slices"
	"time"

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
	if p.Goal == "" {
		p.Goal = GoalMaintain
	}
	if err := validate(p); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, p)
}

// ReplaceInput is a full replace of the profile, except that a nil DietType,
// HouseholdPortions or Goal keeps the stored value. Profile.DietType,
// Profile.HouseholdPortions and Profile.Goal are ignored.
type ReplaceInput struct {
	Profile           Profile
	DietType          *DietType
	HouseholdPortions *float64
	Goal              *Goal
}

// Replace merges in with the stored profile, then validates and persists it.
func (s *Service) Replace(ctx context.Context, in ReplaceInput) (*Profile, error) {
	p := in.Profile
	if in.DietType == nil || in.HouseholdPortions == nil || in.Goal == nil {
		stored, err := s.repo.Get(ctx)
		if err != nil {
			return nil, err
		}
		p.DietType = stored.DietType
		p.HouseholdPortions = stored.HouseholdPortions
		p.Goal = stored.Goal
	}
	if in.Goal != nil {
		p.Goal = *in.Goal
	}
	if in.DietType != nil {
		p.DietType = *in.DietType
	}
	if in.HouseholdPortions != nil {
		p.HouseholdPortions = *in.HouseholdPortions
	}

	return s.Update(ctx, &p)
}

// HouseholdPortions returns the default eaters of a new plate (Phase 2 D10).
func (s *Service) HouseholdPortions(ctx context.Context) (float64, error) {
	p, err := s.Get(ctx)
	if err != nil {
		return 0, err
	}
	return p.HouseholdPortions, nil
}

// MaxHouseholdPortions caps Profile.HouseholdPortions.
const MaxHouseholdPortions float64 = 20

func validate(p *Profile) error {
	if !p.DietType.Valid() {
		return fmt.Errorf("%w: %q", domain.ErrInvalidDietType, p.DietType)
	}
	if p.MeatDaysPerWeekMax != nil && (*p.MeatDaysPerWeekMax < 0 || *p.MeatDaysPerWeekMax > 7) {
		return fmt.Errorf("%w: must be between 0 and 7", domain.ErrInvalidMeatDays)
	}
	if p.HouseholdPortions <= 0 || p.HouseholdPortions > MaxHouseholdPortions {
		return fmt.Errorf("%w: must be > 0 and <= %g", domain.ErrInvalidHouseholdPortions, MaxHouseholdPortions)
	}

	if !p.Goal.Valid() {
		return fmt.Errorf("%w: %q", domain.ErrInvalidGoal, p.Goal)
	}
	if err := validateBody(p); err != nil {
		return err
	}

	if p.KcalTarget != nil && *p.KcalTarget <= 0 {
		return fmt.Errorf("%w: kcal_target must be positive", domain.ErrInvalidMacros)
	}
	// A target below the safety floor could never be planned: the floor is a
	// hard rule.
	if p.KcalTarget != nil && *p.KcalTarget < MinDailyKcal(p.Sex) {
		return fmt.Errorf("%w: kcal_target below %.0f", domain.ErrKcalBelowFloor, MinDailyKcal(p.Sex))
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

// Bounds of the optional body data: generous, they only catch typos.
const (
	MinBirthYear = 1900
	maxWeightKg  = 500
	maxHeightCm  = 280
	// MinProteinGPerKg is the DGE adult reference value; MaxProteinGPerKg is an
	// upper bound that was not verified against a primary source.
	MinProteinGPerKg = 0.8
	MaxProteinGPerKg = 2.2
)

func validateBody(p *Profile) error {
	bad := func(msg string) error { return fmt.Errorf("%w: %s", domain.ErrInvalidBodyData, msg) }
	if p.WeightKg != nil && (*p.WeightKg <= 0 || *p.WeightKg > maxWeightKg) {
		return bad("weight_kg out of range")
	}
	if p.ProteinGPerKg != nil && (*p.ProteinGPerKg < MinProteinGPerKg || *p.ProteinGPerKg > MaxProteinGPerKg) {
		return bad("protein_g_per_kg must be between 0.8 and 2.2")
	}
	if p.HeightCm != nil && (*p.HeightCm <= 0 || *p.HeightCm > maxHeightCm) {
		return bad("height_cm out of range")
	}
	if p.BirthYear != nil && (*p.BirthYear < MinBirthYear || *p.BirthYear > time.Now().Year()) {
		return bad("birth_year out of range")
	}
	if p.Sex != nil && !p.Sex.Valid() {
		return bad("sex must be female or male")
	}
	if p.ActivityLevel != nil && !slices.Contains(ActivityLevels, *p.ActivityLevel) {
		return bad("activity_level must be 1.4, 1.6 or 1.8")
	}
	return nil
}
