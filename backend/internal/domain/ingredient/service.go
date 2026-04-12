package ingredient

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

var validSources = map[Source]bool{
	SourceManual: true,
	SourceOFF:    true,
	SourceFDC:    true,
}

func validateNutrition(i *Ingredient) error {
	if i.Kcal100g < 0 || i.Protein100g < 0 || i.Fat100g < 0 ||
		i.Carbs100g < 0 || i.Fiber100g < 0 || i.Sodium100g < 0 {
		return fmt.Errorf("%w: nutrition values must not be negative", domain.ErrInvalidInput)
	}
	return nil
}

// Service holds all business logic for ingredients.
type Service struct {
	repo Repository
}

// NewService creates an ingredient service backed by the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates and persists a new ingredient.
func (s *Service) Create(ctx context.Context, i *Ingredient) error {
	if i.Name == "" {
		return fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	if i.Source == "" {
		i.Source = SourceManual
	}
	if !validSources[i.Source] {
		return fmt.Errorf("%w: invalid source", domain.ErrInvalidInput)
	}
	if err := validateNutrition(i); err != nil {
		return err
	}
	return s.repo.Create(ctx, i)
}

// Get retrieves an ingredient by ID.
func (s *Service) Get(ctx context.Context, id int64) (*Ingredient, error) {
	return s.repo.Get(ctx, id)
}

// Update validates and persists changes to an existing ingredient.
func (s *Service) Update(ctx context.Context, i *Ingredient) error {
	if i.Name == "" {
		return fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	if i.Source == "" {
		i.Source = SourceManual
	}
	if !validSources[i.Source] {
		return fmt.Errorf("%w: invalid source", domain.ErrInvalidInput)
	}
	if err := validateNutrition(i); err != nil {
		return err
	}
	return s.repo.Update(ctx, i)
}

// Delete removes an ingredient by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListPortions returns all portions for the given ingredient.
func (s *Service) ListPortions(ctx context.Context, ingredientID int64) ([]Portion, error) {
	if _, err := s.repo.Get(ctx, ingredientID); err != nil {
		return nil, err
	}
	return s.repo.ListPortions(ctx, ingredientID)
}

// UpsertPortion validates and creates or updates a portion for an ingredient.
func (s *Service) UpsertPortion(ctx context.Context, p *Portion) error {
	if p.Unit == "" {
		return fmt.Errorf("%w: unit required", domain.ErrInvalidInput)
	}
	if p.Grams <= 0 {
		return fmt.Errorf("%w: grams must be positive", domain.ErrInvalidInput)
	}
	if _, err := s.repo.Get(ctx, p.IngredientID); err != nil {
		return err
	}
	return s.repo.UpsertPortion(ctx, p)
}

// DeletePortion removes a portion by ingredient ID and unit.
func (s *Service) DeletePortion(ctx context.Context, ingredientID int64, unit string) error {
	return s.repo.DeletePortion(ctx, ingredientID, unit)
}

// List returns a page of ingredients matching the query.
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
