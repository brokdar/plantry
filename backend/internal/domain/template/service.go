package template

import (
	"context"
	"fmt"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Service holds business logic for templates.
type Service struct {
	repo       Repository
	components ComponentChecker
	plates     PlateComponentSource
	tx         TxRunner
}

// NewService creates a Service.
func NewService(r Repository, c ComponentChecker, p PlateComponentSource, tx TxRunner) *Service {
	return &Service{repo: r, components: c, plates: p, tx: tx}
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
				ComponentID: pc.ComponentID,
				Portions:    pc.Portions,
				SortOrder:   i,
			}
		}
	} else {
		t.Components = make([]TemplateComponent, len(components))
		for i, c := range components {
			if c.ComponentID <= 0 {
				return nil, fmt.Errorf("%w: components[%d] component_id required", domain.ErrInvalidInput, i)
			}
			exists, err := s.components.Exists(ctx, c.ComponentID)
			if err != nil {
				return nil, fmt.Errorf("check component %d: %w", c.ComponentID, err)
			}
			if !exists {
				return nil, fmt.Errorf("%w: component %d does not exist", domain.ErrNotFound, c.ComponentID)
			}
			portions := c.Portions
			if portions <= 0 {
				portions = 1
			}
			t.Components[i] = TemplateComponent{
				ComponentID: c.ComponentID,
				Portions:    portions,
				SortOrder:   i,
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

// Apply copies the template's components onto the given plate, transactionally.
//
//	merge=false: replaces plate components with the template's (sort_order
//	             reassigned from 0).
//	merge=true:  appends the template's components after existing ones,
//	             continuing the existing plate's max(sort_order)+1.
func (s *Service) Apply(ctx context.Context, templateID, plateID int64, merge bool) error {
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
					PlateID:     p.ID,
					ComponentID: tc.ComponentID,
					Portions:    tc.Portions,
					SortOrder:   i,
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
				PlateID:     p.ID,
				ComponentID: tc.ComponentID,
				Portions:    tc.Portions,
				SortOrder:   next + i,
			}
			if err := pr.CreateComponent(ctx, pc); err != nil {
				return fmt.Errorf("append template component: %w", err)
			}
		}
		// silence unused-var linter: tr is the tx-bound template repo we
		// expose for future tx-scoped template reads, kept for symmetry with
		// planner.Copy.
		_ = tr
		return nil
	})
}
