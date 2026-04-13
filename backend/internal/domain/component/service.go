package component

import (
	"context"
	"fmt"

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

// Service holds business logic for components.
type Service struct {
	repo            Repository
	portions        PortionLookup
	nutritionLookup NutritionLookup
}

// NewService creates a component service.
func NewService(repo Repository, portions PortionLookup, nl NutritionLookup) *Service {
	return &Service{repo: repo, portions: portions, nutritionLookup: nl}
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

// Delete removes a component by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
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
