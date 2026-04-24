package food

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

var validSources = map[Source]bool{
	SourceManual: true,
	SourceOFF:    true,
	SourceFDC:    true,
}

// ImageDeleter removes stored image files. Used for orphan cleanup on Delete.
type ImageDeleter interface {
	Delete(category string, id int64) error
}

// Service holds business logic for foods (both leaf and composed).
type Service struct {
	repo            Repository
	images          ImageDeleter    // optional; nil-safe
	portionProvider PortionProvider // optional; SyncPortionsFromFDC returns ErrLookupFailed when nil
}

// NewService creates a food service backed by the given repository.
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

// ErrNoFdcID is returned when portion sync is requested for a food without
// an fdc_id (manual or OFF-sourced).
var ErrNoFdcID = errors.New("food has no fdc_id")

// Create validates and persists a new food (leaf or composed).
func (s *Service) Create(ctx context.Context, f *Food) error {
	if err := s.validate(f); err != nil {
		return err
	}
	if f.Kind == KindComposed {
		if err := s.resolveGrams(ctx, f.Children); err != nil {
			return err
		}
	}
	return s.repo.Create(ctx, f)
}

// Get retrieves a food by ID with its children, instructions, tags, and portions.
func (s *Service) Get(ctx context.Context, id int64) (*Food, error) {
	return s.repo.Get(ctx, id)
}

// Update validates and persists changes, replacing all children. Detects
// cycles on composed foods before writing.
func (s *Service) Update(ctx context.Context, f *Food) error {
	if err := s.validate(f); err != nil {
		return err
	}
	if f.Kind == KindComposed {
		if err := s.resolveGrams(ctx, f.Children); err != nil {
			return err
		}
		if err := s.detectCycles(ctx, f); err != nil {
			return err
		}
	}
	return s.repo.Update(ctx, f)
}

// Delete removes a food by ID and best-effort deletes its stored image.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.images != nil {
		_ = s.images.Delete("foods", id)
	}
	return nil
}

// List returns a page of foods matching the query.
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

// SetFavorite toggles the favorite flag. Favorites surface first in listings
// and bias the kitchen agent's fill-empty picks.
func (s *Service) SetFavorite(ctx context.Context, id int64, favorite bool) (*Food, error) {
	return s.repo.SetFavorite(ctx, id, favorite)
}

// MarkCooked bumps cook_count and stamps last_cooked_at. Only meaningful for
// composed foods but the repo accepts any food_id (unused on leaf is harmless).
func (s *Service) MarkCooked(ctx context.Context, id int64, at time.Time) error {
	return s.repo.MarkCooked(ctx, id, at)
}

// ── Portions ──────────────────────────────────────────────────────────

// ListPortions returns all unit→grams overrides for a leaf food.
func (s *Service) ListPortions(ctx context.Context, foodID int64) ([]Portion, error) {
	f, err := s.repo.Get(ctx, foodID)
	if err != nil {
		return nil, err
	}
	if f.Kind != KindLeaf {
		return nil, fmt.Errorf("%w: portions only apply to leaf foods", domain.ErrInvalidInput)
	}
	return s.repo.ListPortions(ctx, foodID)
}

// UpsertPortion creates or updates a portion override for a leaf food.
func (s *Service) UpsertPortion(ctx context.Context, p *Portion) error {
	if p.Unit == "" {
		return fmt.Errorf("%w: unit required", domain.ErrInvalidInput)
	}
	if p.Grams <= 0 {
		return fmt.Errorf("%w: grams must be positive", domain.ErrInvalidInput)
	}
	f, err := s.repo.Get(ctx, p.FoodID)
	if err != nil {
		return err
	}
	if f.Kind != KindLeaf {
		return fmt.Errorf("%w: portions only apply to leaf foods", domain.ErrInvalidInput)
	}
	return s.repo.UpsertPortion(ctx, p)
}

// DeletePortion removes a portion override.
func (s *Service) DeletePortion(ctx context.Context, foodID int64, unit string) error {
	return s.repo.DeletePortion(ctx, foodID, unit)
}

// SyncPortionsFromFDC fetches foodPortions from FDC for the food's fdc_id,
// normalizes unit names, groups by canonical key, takes the median gram weight
// per unit, and upserts each into food_portions. Only valid for leaf foods
// with an fdc_id. Returns the count of portions written.
func (s *Service) SyncPortionsFromFDC(ctx context.Context, id int64) (int, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	if f.Kind != KindLeaf {
		return 0, fmt.Errorf("%w: portion sync only applies to leaf foods", domain.ErrInvalidInput)
	}
	if f.FdcID == nil || *f.FdcID == "" {
		return 0, fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrNoFdcID)
	}
	if s.portionProvider == nil {
		return 0, fmt.Errorf("%w: no portion provider configured", domain.ErrLookupFailed)
	}
	fdcID, err := strconv.Atoi(*f.FdcID)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid fdc_id %q", domain.ErrInvalidInput, *f.FdcID)
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
		m := median(grams)
		if m <= 0 {
			continue
		}
		if err := s.repo.UpsertPortion(ctx, &Portion{
			FoodID: id,
			Unit:   unit,
			Grams:  m,
		}); err != nil {
			return count, fmt.Errorf("upsert portion %s: %w", unit, err)
		}
		count++
	}
	return count, nil
}

// ── Variants ──────────────────────────────────────────────────────────

// CreateVariant creates a skeleton composed food in the same variant group as
// parent. If parent has no group, one is created + assigned first.
func (s *Service) CreateVariant(ctx context.Context, parentID int64) (*Food, error) {
	parent, err := s.repo.Get(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if parent.Kind != KindComposed {
		return nil, fmt.Errorf("%w: variants only apply to composed foods", domain.ErrInvalidInput)
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
	ref := float64(1)
	if parent.ReferencePortions != nil {
		ref = *parent.ReferencePortions
	}
	variant := &Food{
		Name:              parent.Name + " (variant)",
		Kind:              KindComposed,
		Role:              parent.Role,
		VariantGroupID:    parent.VariantGroupID,
		ReferencePortions: &ref,
	}
	if err := s.repo.Create(ctx, variant); err != nil {
		return nil, fmt.Errorf("create variant: %w", err)
	}
	return variant, nil
}

// ListVariants returns sibling composed foods in the same variant group,
// excluding the given id.
func (s *Service) ListVariants(ctx context.Context, id int64) ([]Food, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.VariantGroupID == nil {
		return []Food{}, nil
	}
	return s.repo.Siblings(ctx, *f.VariantGroupID, id)
}

// ── Insights ──────────────────────────────────────────────────────────

// Insights returns rotation signals (forgotten / most-cooked) for composed foods.
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

// ── Validation ────────────────────────────────────────────────────────

func (s *Service) validate(f *Food) error {
	if f.Name == "" {
		return fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	switch f.Kind {
	case KindLeaf:
		if f.Source == nil || *f.Source == "" {
			src := SourceManual
			f.Source = &src
		}
		if !validSources[*f.Source] {
			return fmt.Errorf("%w: invalid source %q", domain.ErrInvalidInput, *f.Source)
		}
		if err := validateLeafNutrition(f); err != nil {
			return err
		}
		if len(f.Children) > 0 {
			return fmt.Errorf("%w: leaf foods cannot have children", domain.ErrInvalidInput)
		}
		if len(f.Instructions) > 0 {
			return fmt.Errorf("%w: leaf foods cannot have instructions", domain.ErrInvalidInput)
		}
		if f.ReferencePortions != nil {
			return fmt.Errorf("%w: leaf foods cannot have reference_portions", domain.ErrInvalidInput)
		}
		if f.Role != nil {
			return fmt.Errorf("%w: leaf foods cannot have a role", domain.ErrInvalidInput)
		}
		if f.VariantGroupID != nil {
			return fmt.Errorf("%w: leaf foods cannot have a variant group", domain.ErrInvalidInput)
		}
	case KindComposed:
		if f.Role == nil || !ValidRole(*f.Role) {
			return fmt.Errorf("%w: invalid or missing role", domain.ErrInvalidInput)
		}
		if f.ReferencePortions == nil || *f.ReferencePortions <= 0 {
			r := float64(1)
			f.ReferencePortions = &r
		}
		if len(f.Children) == 0 {
			return fmt.Errorf("%w: composed foods require at least one child", domain.ErrInvalidInput)
		}
		if f.Source != nil {
			return fmt.Errorf("%w: composed foods cannot have a source", domain.ErrInvalidInput)
		}
		if f.Barcode != nil || f.OffID != nil || f.FdcID != nil {
			return fmt.Errorf("%w: composed foods cannot have provenance ids", domain.ErrInvalidInput)
		}
		for i, ch := range f.Children {
			if ch.ChildID == f.ID && f.ID != 0 {
				return fmt.Errorf("%w: child[%d] references its parent (self-loop)", domain.ErrInvalidInput, i)
			}
			if ch.Amount <= 0 {
				return fmt.Errorf("%w: child[%d] amount must be positive", domain.ErrInvalidInput, i)
			}
			if ch.Unit == "" {
				return fmt.Errorf("%w: child[%d] unit required", domain.ErrInvalidInput, i)
			}
		}
		for i, inst := range f.Instructions {
			if inst.Text == "" {
				return fmt.Errorf("%w: instruction[%d] text required", domain.ErrInvalidInput, i)
			}
		}
	default:
		return fmt.Errorf("%w: invalid kind %q", domain.ErrInvalidInput, f.Kind)
	}
	return nil
}

func validateLeafNutrition(f *Food) error {
	checks := []*float64{
		f.Kcal100g, f.Protein100g, f.Fat100g, f.Carbs100g, f.Fiber100g, f.Sodium100g,
	}
	for _, v := range checks {
		if v != nil && *v < 0 {
			return fmt.Errorf("%w: nutrition values must not be negative", domain.ErrInvalidInput)
		}
	}
	return nil
}

// ── Grams resolver ────────────────────────────────────────────────────

// resolveGrams populates Grams + GramsSource for each child using the same
// layered fallback chain as the old component service:
//
//  1. Food-specific portion (FDC/OFF-sourced or user-added) — exact.
//  2. Universal default (mass direct, volume water-density) — exact for mass,
//     approximate for volume.
//  3. User-supplied explicit Grams on a count unit or unknown unit — manual.
//
// Leaf foods are looked up for portion overrides; composed foods only support
// mass/volume-default units + manual grams (they don't have own portions).
func (s *Service) resolveGrams(ctx context.Context, children []FoodComponent) error {
	for i := range children {
		ch := &children[i]
		normalized := units.Normalize(ch.Unit)
		if normalized == "" {
			return fmt.Errorf("%w: child[%d] unit required", domain.ErrInvalidInput, i)
		}
		ch.Unit = normalized
		manualGrams := ch.Grams

		// 1. Food-specific portion lookup (skip bare mass units).
		if normalized != "g" && normalized != "kg" && normalized != "mg" {
			portions, err := s.repo.ListPortions(ctx, ch.ChildID)
			if err != nil {
				return fmt.Errorf("resolve food %d portions: %w", ch.ChildID, err)
			}
			matched := false
			for _, p := range portions {
				if units.Normalize(p.Unit) == normalized {
					ch.Grams = ch.Amount * p.Grams
					ch.GramsSource = GramsSourcePortion
					matched = true
					break
				}
			}
			if matched {
				continue
			}
		}

		// 2. Universal default.
		if def, ok := units.LookupDefault(normalized); ok {
			ch.Grams = ch.Amount * def.Grams
			switch {
			case def.Kind == units.KindMass && normalized == "g":
				ch.GramsSource = GramsSourceDirect
			case def.Kind == units.KindMass:
				ch.GramsSource = GramsSourceDefault
			default:
				ch.GramsSource = GramsSourceFallback
			}
			continue
		}

		// 3. Manual fallback.
		if manualGrams > 0 {
			ch.Grams = manualGrams
			ch.GramsSource = GramsSourceManual
			continue
		}

		if units.IsCount(normalized) {
			return fmt.Errorf("%w: child %d: unit %q requires a portion or manual grams",
				domain.ErrInvalidInput, ch.ChildID, normalized)
		}
		return fmt.Errorf("%w: unknown unit %q for child %d",
			domain.ErrInvalidInput, normalized, ch.ChildID)
	}
	return nil
}

// ── Cycle detection ───────────────────────────────────────────────────

// detectCycles walks the proposed children's reachable sets to make sure none
// can reach the parent (which would create a cycle after the update lands).
func (s *Service) detectCycles(ctx context.Context, f *Food) error {
	if f.ID == 0 {
		return nil // Create — no existing row, nothing can reach back yet.
	}
	for _, ch := range f.Children {
		if ch.ChildID == f.ID {
			return fmt.Errorf("%w: child references parent (self-loop)", domain.ErrInvalidInput)
		}
		reach, err := s.repo.Reachable(ctx, ch.ChildID)
		if err != nil {
			return fmt.Errorf("reachability for %d: %w", ch.ChildID, err)
		}
		if _, ok := reach[f.ID]; ok {
			return fmt.Errorf("%w: adding child %d would create a cycle", domain.ErrInvalidInput, ch.ChildID)
		}
	}
	return nil
}

// ── Helpers: FDC portion grouping (preserved from ingredient service) ──

func groupPortions(portions []PortionInfo) map[string][]float64 {
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

func simplifyFDCUnit(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || s == "undetermined" {
		return ""
	}
	if i := strings.Index(s, "("); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	if i := strings.Index(s, ","); i > 0 {
		head := strings.TrimSpace(s[:i])
		if normed := units.Normalize(head); isKnownUnit(normed) {
			return normed
		}
	}
	switch {
	case s == "small", s == "medium", s == "large", s == "extra small",
		s == "extra large", s == "jumbo", s == "mini":
		return "piece"
	case strings.HasPrefix(s, "nlea serving"):
		return "serving"
	}
	if fields := strings.Fields(s); len(fields) > 1 {
		if normed := units.Normalize(fields[0]); isKnownUnit(normed) {
			return normed
		}
	}
	normed := units.Normalize(s)
	if !isKnownUnit(normed) {
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
