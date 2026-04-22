package ingredient

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
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

// ImageDeleter removes stored image files. Used for orphan cleanup on Delete.
type ImageDeleter interface {
	Delete(category string, id int64) error
}

// Service holds all business logic for ingredients.
type Service struct {
	repo            Repository
	images          ImageDeleter    // optional; nil-safe
	portionProvider PortionProvider // optional; nil-safe (sync becomes a no-op)
}

// NewService creates an ingredient service backed by the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// WithImageStore wires an image deleter so Delete cleans up orphaned image files.
func (s *Service) WithImageStore(img ImageDeleter) *Service {
	s.images = img
	return s
}

// WithPortionProvider wires an external source for per-unit gram weights
// (typically FDC). When unset, SyncPortionsFromFDC returns ErrLookupFailed.
func (s *Service) WithPortionProvider(p PortionProvider) *Service {
	s.portionProvider = p
	return s
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

// Delete removes an ingredient by ID and best-effort deletes its stored image.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.images != nil {
		_ = s.images.Delete("ingredients", id)
	}
	return nil
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

// ErrNoFdcID is returned when portion sync is requested for an ingredient
// that has no fdc_id set (i.e., manual or OFF-sourced ingredients).
var ErrNoFdcID = errors.New("ingredient has no fdc_id")

// SyncPortionsFromFDC fetches foodPortions from FDC for the ingredient's
// FDC ID, normalizes the unit names, groups by canonical key, takes the
// median gram weight per unit, and upserts each into ingredient_portions.
// Returns the count of portions written.
func (s *Service) SyncPortionsFromFDC(ctx context.Context, id int64) (int, error) {
	ing, err := s.repo.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	if ing.FdcID == nil || *ing.FdcID == "" {
		return 0, fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrNoFdcID)
	}
	if s.portionProvider == nil {
		return 0, fmt.Errorf("%w: no portion provider configured", domain.ErrLookupFailed)
	}
	fdcID, err := strconv.Atoi(*ing.FdcID)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid fdc_id %q", domain.ErrInvalidInput, *ing.FdcID)
	}

	portions, err := s.portionProvider.GetFoodPortions(ctx, fdcID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return 0, fmt.Errorf("%w: fdc food %d not found", domain.ErrNotFound, fdcID)
		}
		return 0, fmt.Errorf("%w: fdc portions: %v", domain.ErrLookupFailed, err)
	}

	grouped := groupPortions(portions)
	count := 0
	for unit, grams := range grouped {
		median := median(grams)
		if median <= 0 {
			continue
		}
		if err := s.repo.UpsertPortion(ctx, &Portion{
			IngredientID: id,
			Unit:         unit,
			Grams:        median,
		}); err != nil {
			return count, fmt.Errorf("upsert portion %s: %w", unit, err)
		}
		count++
	}
	return count, nil
}

// groupPortions collapses raw FDC portions to one gram list per canonical
// unit. FDC encodes count items in several ways: size modifiers ("large",
// "medium"), volume variants ("cup, sliced", "cup, mashed"), and NLEA-style
// per-serving weights. We simplify common patterns and accept anything else
// as-is so users don't silently lose real portion data.
func groupPortions(portions []FoodPortion) map[string][]float64 {
	grouped := map[string][]float64{}
	for _, p := range portions {
		if p.GramWeight <= 0 {
			continue
		}
		raw := p.RawUnit
		if raw == "" || raw == "undetermined" {
			raw = p.Modifier
		}
		unit := simplifyFDCUnit(raw)
		if unit == "" {
			continue
		}
		grouped[unit] = append(grouped[unit], p.GramWeight)
	}
	return grouped
}

// simplifyFDCUnit folds FDC's verbose unit labels into our canonical
// vocabulary. It recognizes size modifiers as "piece", strips parenthesized
// ranges (e.g., "large (8 inches)" → "large"), and collapses "cup, sliced"
// variants back to "cup". Returns "" for labels we can't map — FDC
// occasionally emits regulatory shorthand like "RACC" that isn't user-facing.
func simplifyFDCUnit(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || s == "undetermined" {
		return ""
	}
	// Strip parenthesized detail: "large (8 inches or longer)" → "large".
	if i := strings.Index(s, "("); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	// "cup, sliced" / "cup, mashed" → "cup"; same pattern for tbsp/tsp/oz.
	if i := strings.Index(s, ","); i > 0 {
		head := strings.TrimSpace(s[:i])
		if normed := units.Normalize(head); isKnownUnit(normed) {
			return normed
		}
	}
	// Size labels map to a single "piece"; median gram weight across sizes
	// becomes the canonical per-piece weight.
	switch {
	case s == "small", s == "medium", s == "large", s == "extra small",
		s == "extra large", s == "jumbo", s == "mini":
		return "piece"
	case strings.HasPrefix(s, "nlea serving"):
		return "serving"
	}
	// Try first-token match for labels like "tbsp chopped" or "oz, boneless".
	if fields := strings.Fields(s); len(fields) > 1 {
		if normed := units.Normalize(fields[0]); isKnownUnit(normed) {
			return normed
		}
	}
	normed := units.Normalize(s)
	if !isKnownUnit(normed) {
		// Regulatory shorthand (RACC, etc.) and other unknown labels are
		// dropped — exposing them as units would confuse the recipe editor.
		return ""
	}
	return normed
}

func isKnownUnit(normalized string) bool {
	if normalized == "" {
		return false
	}
	if _, ok := units.LookupDefault(normalized); ok {
		return true
	}
	return units.IsCount(normalized)
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
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
