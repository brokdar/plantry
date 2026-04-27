package template

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Service holds business logic for templates.
type Service struct {
	repo   Repository
	foods  FoodChecker
	plates PlateComponentSource
	tx     TxRunner
}

// NewService creates a Service.
func NewService(r Repository, f FoodChecker, p PlateComponentSource, tx TxRunner) *Service {
	return &Service{repo: r, foods: f, plates: p, tx: tx}
}

// Create persists a new template. Exactly one of fromPlateID or components may
// be provided; both nil creates an empty template. Both set returns
// ErrInvalidInput.
func (s *Service) Create(ctx context.Context, name string, fromPlateID *int64, components []TemplateComponent) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	if fromPlateID != nil && len(components) > 0 {
		return nil, fmt.Errorf("%w: provide either from_plate_id or components, not both", domain.ErrInvalidInput)
	}

	t := &Template{Name: name}

	if fromPlateID != nil {
		src, err := s.plates.ListComponentsByPlate(ctx, *fromPlateID)
		if err != nil {
			return nil, err
		}
		t.Components = make([]TemplateComponent, len(src))
		for i, pc := range src {
			t.Components[i] = TemplateComponent{
				FoodID:    pc.FoodID,
				Portions:  pc.Portions,
				SortOrder: i,
			}
		}
	} else {
		t.Components = make([]TemplateComponent, len(components))
		for i, c := range components {
			if c.FoodID <= 0 {
				return nil, fmt.Errorf("%w: components[%d] food_id required", domain.ErrInvalidInput, i)
			}
			exists, err := s.foods.Exists(ctx, c.FoodID)
			if err != nil {
				return nil, fmt.Errorf("check food %d: %w", c.FoodID, err)
			}
			if !exists {
				return nil, fmt.Errorf("%w: food %d does not exist", domain.ErrNotFound, c.FoodID)
			}
			portions := c.Portions
			if portions <= 0 {
				portions = 1
			}
			t.Components[i] = TemplateComponent{
				FoodID:    c.FoodID,
				Portions:  portions,
				SortOrder: i,
			}
		}
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Get returns a template with its components loaded.
func (s *Service) Get(ctx context.Context, id int64) (*Template, error) {
	return s.repo.Get(ctx, id)
}

// List returns all templates with components loaded.
func (s *Service) List(ctx context.Context) ([]Template, error) {
	return s.repo.List(ctx)
}

// UpdateName renames an existing template.
func (s *Service) UpdateName(ctx context.Context, id int64, name string) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	return s.repo.UpdateName(ctx, id, name)
}

// Delete removes a template (cascades to template_components via FK).
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ApplyToPlate copies the template's components onto the given plate, transactionally.
//
//	merge=false: replaces plate components with the template's (sort_order
//	             reassigned from 0).
//	merge=true:  appends the template's components after existing ones,
//	             continuing the existing plate's max(sort_order)+1.
func (s *Service) ApplyToPlate(ctx context.Context, templateID, plateID int64, merge bool) error {
	t, err := s.repo.Get(ctx, templateID)
	if err != nil {
		return err
	}
	return s.tx.RunInTemplateTx(ctx, func(tr Repository, pr plate.Repository) error {
		p, err := pr.Get(ctx, plateID)
		if err != nil {
			return err
		}
		if !merge {
			for _, pc := range p.Components {
				if err := pr.DeleteComponent(ctx, pc.ID); err != nil {
					return err
				}
			}
			for i, tc := range t.Components {
				pc := &plate.PlateComponent{
					PlateID:   p.ID,
					FoodID:    tc.FoodID,
					Portions:  tc.Portions,
					SortOrder: i,
				}
				if err := pr.CreateComponent(ctx, pc); err != nil {
					return fmt.Errorf("add template component: %w", err)
				}
			}
			return nil
		}
		next := 0
		for _, pc := range p.Components {
			if pc.SortOrder >= next {
				next = pc.SortOrder + 1
			}
		}
		for i, tc := range t.Components {
			pc := &plate.PlateComponent{
				PlateID:   p.ID,
				FoodID:    tc.FoodID,
				Portions:  tc.Portions,
				SortOrder: next + i,
			}
			if err := pr.CreateComponent(ctx, pc); err != nil {
				return fmt.Errorf("append template component: %w", err)
			}
		}
		_ = tr
		return nil
	})
}

// Apply creates new dated plates from a template. One plate is created per
// unique day_offset value in the template's components. The plate is placed at
// startDate + day_offset days, using slotID as the time slot.
// Returns the list of created plates.
func (s *Service) Apply(ctx context.Context, templateID int64, startDate time.Time, slotID int64) ([]plate.Plate, error) {
	if slotID <= 0 {
		return nil, fmt.Errorf("%w: slot_id required", domain.ErrInvalidInput)
	}
	t, err := s.repo.Get(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if len(t.Components) == 0 {
		return []plate.Plate{}, nil
	}

	// Group components by day_offset.
	type offsetGroup struct {
		offset int
		comps  []TemplateComponent
	}
	seen := make(map[int]int) // offset -> index in groups
	var groups []offsetGroup
	for _, tc := range t.Components {
		idx, ok := seen[tc.DayOffset]
		if !ok {
			idx = len(groups)
			seen[tc.DayOffset] = idx
			groups = append(groups, offsetGroup{offset: tc.DayOffset})
		}
		groups[idx].comps = append(groups[idx].comps, tc)
	}

	var created []plate.Plate
	if err := s.tx.RunInTemplateTx(ctx, func(_ Repository, pr plate.Repository) error {
		for _, g := range groups {
			date := startDate.AddDate(0, 0, g.offset)
			pcs := make([]plate.PlateComponent, len(g.comps))
			for i, tc := range g.comps {
				pcs[i] = plate.PlateComponent{
					FoodID:    tc.FoodID,
					Portions:  tc.Portions,
					SortOrder: i,
				}
			}
			p := &plate.Plate{
				Date:       date,
				SlotID:     slotID,
				Components: pcs,
			}
			if err := pr.Create(ctx, p); err != nil {
				return fmt.Errorf("create plate at offset %d: %w", g.offset, err)
			}
			created = append(created, *p)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return created, nil
}

// maxDayOffset is the maximum allowed day_offset when building a template from plates.
const maxDayOffset = 30

// SaveAsTemplate creates a new template from a set of plates anchored at anchorDate.
// Each plate component becomes a template component with day_offset = floor((plate.Date - anchorDate) / 24h).
// All plate dates must be in [anchorDate, anchorDate+maxDayOffset days].
func (s *Service) SaveAsTemplate(ctx context.Context, name string, plates []plate.Plate, anchorDate time.Time) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	if len(plates) == 0 {
		return nil, fmt.Errorf("%w: plates must not be empty", domain.ErrInvalidInput)
	}

	var comps []TemplateComponent
	for _, p := range plates {
		diff := p.Date.Truncate(24 * time.Hour).Sub(anchorDate.Truncate(24 * time.Hour))
		offsetDays := int(diff.Hours() / 24)
		if offsetDays < 0 {
			return nil, fmt.Errorf("%w: plate date %s is before anchorDate %s",
				domain.ErrInvalidInput, p.Date.Format("2006-01-02"), anchorDate.Format("2006-01-02"))
		}
		if offsetDays > maxDayOffset {
			return nil, fmt.Errorf("%w: plate date %s exceeds anchor by more than %d days",
				domain.ErrInvalidInput, p.Date.Format("2006-01-02"), maxDayOffset)
		}
		for i, pc := range p.Components {
			comps = append(comps, TemplateComponent{
				FoodID:    pc.FoodID,
				Portions:  pc.Portions,
				SortOrder: i,
				DayOffset: offsetDays,
			})
		}
	}

	t := &Template{
		Name:       name,
		Components: comps,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}
