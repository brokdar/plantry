package component

import (
	"context"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
)

// PortionLookup resolves custom portion units to grams.
type PortionLookup interface {
	ListPortions(ctx context.Context, ingredientID int64) ([]ingredient.Portion, error)
}

// NutritionLookup fetches ingredient nutrition data by ID.
type NutritionLookup interface {
	LookupForNutrition(ctx context.Context, ids []int64) (map[int64]*ingredient.Ingredient, error)
}

// ImageDeleter removes stored image files. Used for orphan cleanup on Delete.
type ImageDeleter interface {
	Delete(category string, id int64) error
}

// Service holds business logic for components.
type Service struct {
	repo            Repository
	portions        PortionLookup
	nutritionLookup NutritionLookup
	images          ImageDeleter // optional; nil-safe
}

// NewService creates a component service.
func NewService(repo Repository, portions PortionLookup, nl NutritionLookup) *Service {
	return &Service{repo: repo, portions: portions, nutritionLookup: nl}
}

// WithImageStore wires an image deleter so Delete cleans up orphaned image files.
func (s *Service) WithImageStore(img ImageDeleter) *Service {
	s.images = img
	return s
}

func (s *Service) validate(c *Component) error {
	if c.Name == "" {
		return fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	if !ValidRole(c.Role) {
		return fmt.Errorf("%w: invalid role %q", domain.ErrInvalidInput, c.Role)
	}
	if c.ReferencePortions <= 0 {
		return fmt.Errorf("%w: reference_portions must be positive", domain.ErrInvalidInput)
	}
	for i, ci := range c.Ingredients {
		if ci.Amount <= 0 {
			return fmt.Errorf("%w: ingredient[%d] amount must be positive", domain.ErrInvalidInput, i)
		}
		if ci.Unit == "" {
			return fmt.Errorf("%w: ingredient[%d] unit required", domain.ErrInvalidInput, i)
		}
	}
	for i, inst := range c.Instructions {
		if inst.Text == "" {
			return fmt.Errorf("%w: instruction[%d] text required", domain.ErrInvalidInput, i)
		}
	}
	return nil
}

// resolveGrams populates the Grams field for each ingredient.
// If the unit is "g" or "ml", grams = amount. Otherwise, look up the portion.
func (s *Service) resolveGrams(ctx context.Context, ingredients []ComponentIngredient) error {
	for i := range ingredients {
		ci := &ingredients[i]
		switch ci.Unit {
		case "g", "ml":
			ci.Grams = ci.Amount
		default:
			portions, err := s.portions.ListPortions(ctx, ci.IngredientID)
			if err != nil {
				return fmt.Errorf("resolve ingredient %d portions: %w", ci.IngredientID, err)
			}
			found := false
			for _, p := range portions {
				if p.Unit == ci.Unit {
					ci.Grams = ci.Amount * p.Grams
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: unknown unit %q for ingredient %d", domain.ErrInvalidInput, ci.Unit, ci.IngredientID)
			}
		}
	}
	return nil
}

// Create validates and persists a new component with its children.
func (s *Service) Create(ctx context.Context, c *Component) error {
	if c.ReferencePortions == 0 {
		c.ReferencePortions = 1
	}
	if err := s.validate(c); err != nil {
		return err
	}
	if err := s.resolveGrams(ctx, c.Ingredients); err != nil {
		return err
	}
	return s.repo.Create(ctx, c)
}

// Get retrieves a component by ID with all children.
func (s *Service) Get(ctx context.Context, id int64) (*Component, error) {
	return s.repo.Get(ctx, id)
}

// Update validates and persists changes, replacing all children.
func (s *Service) Update(ctx context.Context, c *Component) error {
	if c.ReferencePortions == 0 {
		c.ReferencePortions = 1
	}
	if err := s.validate(c); err != nil {
		return err
	}
	if err := s.resolveGrams(ctx, c.Ingredients); err != nil {
		return err
	}
	return s.repo.Update(ctx, c)
}

// Delete removes a component by ID and best-effort deletes its stored image.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.images != nil {
		_ = s.images.Delete("components", id)
	}
	return nil
}

// SetFavorite toggles the favorite flag for a component. Favorites surface
// first in catalog listings and bias the kitchen agent's fill-empty picks.
func (s *Service) SetFavorite(ctx context.Context, id int64, favorite bool) (*Component, error) {
	return s.repo.SetFavorite(ctx, id, favorite)
}

// List returns a page of components matching the query.
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > 200 {
		q.Limit = 200
	}
	if q.SortBy == "" {
		q.SortBy = "name"
	}
	return s.repo.List(ctx, q)
}

// CreateVariant creates a new skeleton component in the same variant group as the parent.
// If the parent has no variant group, one is created and assigned to the parent first.
func (s *Service) CreateVariant(ctx context.Context, parentID int64) (*Component, error) {
	parent, err := s.repo.Get(ctx, parentID)
	if err != nil {
		return nil, err
	}

	if parent.VariantGroupID == nil {
		groupID, err := s.repo.CreateVariantGroup(ctx, parent.Name)
		if err != nil {
			return nil, fmt.Errorf("create variant group: %w", err)
		}
		parent.VariantGroupID = &groupID
		if err := s.repo.Update(ctx, parent); err != nil {
			return nil, fmt.Errorf("assign parent to group: %w", err)
		}
	}

	variant := &Component{
		Name:              parent.Name + " (variant)",
		Role:              parent.Role,
		VariantGroupID:    parent.VariantGroupID,
		ReferencePortions: parent.ReferencePortions,
	}
	if err := s.repo.Create(ctx, variant); err != nil {
		return nil, fmt.Errorf("create variant: %w", err)
	}
	return variant, nil
}

// ListVariants returns sibling components in the same variant group, excluding the given component.
func (s *Service) ListVariants(ctx context.Context, componentID int64) ([]Component, error) {
	c, err := s.repo.Get(ctx, componentID)
	if err != nil {
		return nil, err
	}
	if c.VariantGroupID == nil {
		return []Component{}, nil
	}
	return s.repo.Siblings(ctx, *c.VariantGroupID, componentID)
}

// Insights returns rotation signals: components not cooked recently (or ever)
// and the most frequently cooked. Zero-valued query fields fall back to
// defaults (4 weeks, 10 forgotten, 5 most-cooked).
func (s *Service) Insights(ctx context.Context, q InsightsQuery) (Insights, error) {
	if q.ForgottenWeeks <= 0 {
		q.ForgottenWeeks = 4
	}
	if q.ForgottenWeeks > 52 {
		q.ForgottenWeeks = 52
	}
	if q.ForgottenLimit <= 0 {
		q.ForgottenLimit = 10
	}
	if q.ForgottenLimit > 50 {
		q.ForgottenLimit = 50
	}
	if q.MostCookedLimit <= 0 {
		q.MostCookedLimit = 5
	}
	if q.MostCookedLimit > 50 {
		q.MostCookedLimit = 50
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -7*q.ForgottenWeeks)
	return s.repo.Insights(ctx, cutoff, q.ForgottenLimit, q.MostCookedLimit)
}

// Nutrition returns per-portion macros for a component.
func (s *Service) Nutrition(ctx context.Context, id int64) (*nutrition.Macros, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, len(c.Ingredients))
	for i, ci := range c.Ingredients {
		ids[i] = ci.IngredientID
	}

	ingMap, err := s.nutritionLookup.LookupForNutrition(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("lookup nutrition: %w", err)
	}

	inputs := make([]nutrition.IngredientInput, 0, len(c.Ingredients))
	for _, ci := range c.Ingredients {
		ing, ok := ingMap[ci.IngredientID]
		if !ok {
			continue
		}
		inputs = append(inputs, nutrition.IngredientInput{
			Per100g: nutrition.Macros{
				Kcal:    ing.Kcal100g,
				Protein: ing.Protein100g,
				Fat:     ing.Fat100g,
				Carbs:   ing.Carbs100g,
				Fiber:   ing.Fiber100g,
				Sodium:  ing.Sodium100g,
			},
			Grams: ci.Grams,
		})
	}

	macros := nutrition.PerPortion(nutrition.ComponentInput{
		Ingredients:       inputs,
		ReferencePortions: c.ReferencePortions,
	})
	return &macros, nil
}
