package preset

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// Service holds business logic for presets.
type Service struct {
	repo       Repository
	foods      FoodLookup
	plateSvc   PlateService
	plateRange PlateRange
	tx         TxRunner
	portions   food.PortionLookup
	foodGet    FoodGetter
	slots      SlotLister
}

// PlateService is the subset of plate.Service we depend on (to read existing
// plates when creating a preset from a plate id).
type PlateService interface {
	Get(ctx context.Context, plateID int64) (*plate.Plate, error)
}

// PlateRange reads plates over a date range (used by CopyWeek).
type PlateRange interface {
	Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

// FoodGetter is the food.Repository.Get surface we need to drive grams
// resolution for leaf components (mirrors plate.FoodLookup).
type FoodGetter interface {
	Get(ctx context.Context, foodID int64) (*food.Food, error)
}

// NewService creates a Service.
func NewService(
	r Repository,
	f FoodLookup,
	plateSvc PlateService,
	tx TxRunner,
	portions food.PortionLookup,
	foodGet FoodGetter,
) *Service {
	return &Service{repo: r, foods: f, plateSvc: plateSvc, tx: tx, portions: portions, foodGet: foodGet}
}

// WithPlateRange injects a plate range reader so CopyWeek can list the source
// week's plates.
func (s *Service) WithPlateRange(pr PlateRange) *Service {
	s.plateRange = pr
	return s
}

// CreateFromPlatesInput is the input to CreateFromPlates: a name, optional
// tags, and ≥1 plate ids whose components will become the preset's plate
// compositions.
type CreateFromPlatesInput struct {
	Name     string
	Tags     []string
	PlateIDs []int64
}

// CreateFromPlates creates a preset from one or more existing planner plates.
// Each plate becomes a PresetPlate bound to its slot_id, with components copied
// verbatim. PresetPlates are ordered by the plate's (Date, SlotID) tuple
// stable-sorted, so a 3-day breakfast / lunch / dinner selection produces
// presets ordered Mon-breakfast, Mon-lunch, Mon-dinner, Tue-breakfast, …
func (s *Service) CreateFromPlates(ctx context.Context, in CreateFromPlatesInput) (*Preset, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	if len(in.PlateIDs) == 0 {
		return nil, fmt.Errorf("%w: plate_ids must not be empty", domain.ErrInvalidInput)
	}

	plates := make([]*plate.Plate, 0, len(in.PlateIDs))
	for _, pid := range in.PlateIDs {
		if pid <= 0 {
			return nil, fmt.Errorf("%w: plate id must be positive", domain.ErrInvalidInput)
		}
		p, err := s.plateSvc.Get(ctx, pid)
		if err != nil {
			return nil, fmt.Errorf("load plate %d: %w", pid, err)
		}
		plates = append(plates, p)
	}

	// Stable-sort by date then slot id so the preset's plate order is
	// deterministic and matches the user's chronological selection.
	sort.SliceStable(plates, func(i, j int) bool {
		if !plates[i].Date.Equal(plates[j].Date) {
			return plates[i].Date.Before(plates[j].Date)
		}
		return plates[i].SlotID < plates[j].SlotID
	})

	pp := make([]Plate, len(plates))
	for i, src := range plates {
		comps := make([]Component, len(src.Components))
		for j, pc := range src.Components {
			comps[j] = Component{
				FoodID:      pc.FoodID,
				Portions:    pc.Portions,
				Amount:      pc.Amount,
				Unit:        pc.Unit,
				Grams:       pc.Grams,
				GramsSource: pc.GramsSource,
				SortOrder:   j,
			}
		}
		pp[i] = Plate{
			SlotID:     src.SlotID,
			SortOrder:  i,
			Components: comps,
		}
	}

	tags := normalizeTags(in.Tags)
	p := &Preset{Name: name, Plates: pp, Tags: tags}
	if err := s.validate(ctx, p); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Get returns a preset by id with plates, components, and tags loaded.
func (s *Service) Get(ctx context.Context, id int64) (*Preset, error) {
	return s.repo.Get(ctx, id)
}

// List returns presets matching the filter, paginated.
func (s *Service) List(ctx context.Context, filter ListFilter) (*ListResult, error) {
	if filter.Limit < 0 {
		return nil, fmt.Errorf("%w: limit must not be negative", domain.ErrInvalidInput)
	}
	if filter.Offset < 0 {
		return nil, fmt.Errorf("%w: offset must not be negative", domain.ErrInvalidInput)
	}
	return s.repo.List(ctx, filter)
}

// UpdateName renames a preset.
func (s *Service) UpdateName(ctx context.Context, id int64, name string) (*Preset, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	return s.repo.UpdateName(ctx, id, name)
}

// UpdateInput is the editor's full-edit payload.
type UpdateInput struct {
	Name   *string
	Tags   *[]string // when non-nil replaces the full tag list
	Plates *[]Plate  // when non-nil replaces the full plate list
}

// Update applies an editor save: rename, replace tags, replace plates.
// All fields are optional; nil pointer means "leave as-is".
func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (*Preset, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
		}
		if _, err := s.repo.UpdateName(ctx, id, name); err != nil {
			return nil, err
		}
	}

	if in.Tags != nil {
		tags := normalizeTags(*in.Tags)
		if err := s.repo.ReplaceTags(ctx, id, tags); err != nil {
			return nil, err
		}
	}

	if in.Plates != nil {
		next := &Preset{ID: existing.ID, Name: existing.Name, Plates: *in.Plates, Tags: existing.Tags}
		if in.Name != nil {
			next.Name = strings.TrimSpace(*in.Name)
		}
		if err := s.validatePlates(ctx, next); err != nil {
			return nil, err
		}
		if err := s.repo.ReplacePlates(ctx, id, *in.Plates); err != nil {
			return nil, err
		}
	}

	return s.repo.Get(ctx, id)
}

// PatchInput is the agent's cheap-edit payload: rename and add/remove tags.
type PatchInput struct {
	Name       *string
	AddTags    []string
	RemoveTags []string
}

// Patch applies a partial update: rename, add tags, remove tags. Plates and
// components are not touched. Used by the agent.
func (s *Service) Patch(ctx context.Context, id int64, in PatchInput) (*Preset, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
		}
		if _, err := s.repo.UpdateName(ctx, id, name); err != nil {
			return nil, err
		}
	}
	for _, t := range in.AddTags {
		n := NormalizeTag(t)
		if n == "" {
			continue
		}
		if err := s.repo.AddTag(ctx, id, n); err != nil {
			return nil, err
		}
	}
	for _, t := range in.RemoveTags {
		n := NormalizeTag(t)
		if n == "" {
			continue
		}
		if err := s.repo.RemoveTag(ctx, id, n); err != nil {
			return nil, err
		}
	}
	return s.repo.Get(ctx, id)
}

// Delete removes a preset (cascades to plates, components, tags via FK).
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Duplicate creates an independent copy of preset id with a "(copy)" suffix.
// The new preset has fresh IDs, no last_used_at, and copied tags.
func (s *Service) Duplicate(ctx context.Context, id int64) (*Preset, error) {
	src, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	dup := &Preset{
		Name:   src.Name + " (copy)",
		Tags:   append([]string(nil), src.Tags...),
		Plates: make([]Plate, len(src.Plates)),
	}
	for i, p := range src.Plates {
		comps := make([]Component, len(p.Components))
		for j, c := range p.Components {
			comps[j] = Component{
				FoodID:      c.FoodID,
				Portions:    c.Portions,
				Amount:      c.Amount,
				Unit:        c.Unit,
				Grams:       c.Grams,
				GramsSource: c.GramsSource,
				Note:        c.Note,
				SortOrder:   j,
			}
		}
		dup.Plates[i] = Plate{
			SlotID:     p.SlotID,
			SortOrder:  i,
			Components: comps,
		}
	}
	if err := s.repo.Create(ctx, dup); err != nil {
		return nil, err
	}
	return dup, nil
}

// KnownTags returns the top-N tags across all presets, ranked by usage.
func (s *Service) KnownTags(ctx context.Context, limit int) ([]TagUsage, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.KnownTags(ctx, limit)
}

// validate runs all validation rules on a preset (used on create).
func (s *Service) validate(ctx context.Context, p *Preset) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	return s.validatePlates(ctx, p)
}

// validatePlates re-validates the plate composition of a preset (used on
// create and on full edit).
func (s *Service) validatePlates(ctx context.Context, p *Preset) error {
	if len(p.Plates) == 0 {
		return fmt.Errorf("%w: preset must have at least one plate", domain.ErrInvalidInput)
	}
	for i := range p.Plates {
		pp := &p.Plates[i]
		if pp.SlotID <= 0 {
			return fmt.Errorf("%w: plates[%d].slot_id required", domain.ErrInvalidInput, i)
		}
		if len(pp.Components) == 0 {
			return fmt.Errorf("%w: plates[%d] must have at least one component", domain.ErrInvalidInput, i)
		}
		for j := range pp.Components {
			c := &pp.Components[j]
			if c.FoodID <= 0 {
				return fmt.Errorf("%w: plates[%d].components[%d].food_id required", domain.ErrInvalidInput, i, j)
			}
			if err := s.normaliseComponent(ctx, c); err != nil {
				return fmt.Errorf("plates[%d].components[%d]: %w", i, j, err)
			}
		}
	}
	return nil
}

// normaliseComponent validates a single component's quantity shape against
// its food's kind and resolves leaf-component grams in place. Mirrors
// plate.Service.normaliseComponent so a preset's components are storage-shape
// compatible with plate components from day one.
func (s *Service) normaliseComponent(ctx context.Context, c *Component) error {
	hasPortions := c.Portions != nil
	hasLeaf := c.Amount != nil || c.Unit != nil
	if hasPortions && hasLeaf {
		return fmt.Errorf("%w: %w", domain.ErrInvalidInput, plate.ErrInvalidQuantityShape)
	}
	if !hasPortions && !hasLeaf {
		return fmt.Errorf("%w: %w", domain.ErrInvalidInput, plate.ErrInvalidQuantityShape)
	}

	f, err := s.foodGet.Get(ctx, c.FoodID)
	if err != nil {
		return err
	}

	switch f.Kind {
	case food.KindComposed:
		if hasLeaf {
			return fmt.Errorf("%w: %w", domain.ErrInvalidInput, plate.ErrInvalidQuantityForComposed)
		}
		if *c.Portions <= 0 {
			return fmt.Errorf("%w: portions must be positive", domain.ErrInvalidInput)
		}
		c.Amount = nil
		c.Unit = nil
		c.Grams = nil
		c.GramsSource = nil
		return nil
	case food.KindLeaf:
		if hasPortions {
			return fmt.Errorf("%w: %w", domain.ErrInvalidInput, plate.ErrInvalidQuantityForLeaf)
		}
		if c.Amount == nil || *c.Amount <= 0 {
			return fmt.Errorf("%w: amount must be positive", domain.ErrInvalidInput)
		}
		if c.Unit == nil || *c.Unit == "" {
			return fmt.Errorf("%w: unit required", domain.ErrInvalidInput)
		}
		manualGrams := 0.0
		if c.Grams != nil {
			manualGrams = *c.Grams
		}
		grams, source, err := food.ResolveGrams(ctx, s.portions, c.FoodID, *c.Amount, *c.Unit, manualGrams)
		if err != nil {
			if errors.Is(err, domain.ErrInvalidInput) {
				return fmt.Errorf("%w: %w", err, plate.ErrUnitRequiresPortion)
			}
			return err
		}
		canonical := *c.Unit
		if normalized := units.Normalize(canonical); normalized != "" {
			canonical = normalized
		}
		c.Unit = &canonical
		c.Grams = &grams
		src := source
		c.GramsSource = &src
		c.Portions = nil
		return nil
	default:
		return fmt.Errorf("%w: food %d has unknown kind %q", domain.ErrInvalidInput, c.FoodID, f.Kind)
	}
}

// normalizeTags lowercases, trims, dedups and returns tags in stable order.
func normalizeTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		n := NormalizeTag(t)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
