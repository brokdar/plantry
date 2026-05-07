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

// ApplyConflict is how Apply behaves when a target (date, slot) already has a plate.
type ApplyConflict string

const (
	// ConflictSkip leaves existing plates untouched (default for day/week scope).
	ConflictSkip ApplyConflict = "skip"
	// ConflictOverwrite deletes the existing plate before creating the new one.
	ConflictOverwrite ApplyConflict = "overwrite"
)

// ApplyPayload is the scope-aware input to Apply. Required fields per scope:
//
//	slot:  Date + SlotID
//	day:   Date  (Conflict optional, defaults to ConflictSkip)
//	week:  StartDate (Conflict optional, defaults to ConflictSkip)
type ApplyPayload struct {
	Date      *time.Time
	SlotID    *int64
	StartDate *time.Time
	Conflict  ApplyConflict
}

// SkippedConflict reports a (date, slot) pair that Apply did not write because
// a plate already existed and the policy was ConflictSkip.
type SkippedConflict struct {
	Date   time.Time
	SlotID int64
}

// ApplyResult is what Apply returns.
type ApplyResult struct {
	Created []plate.Plate
	Skipped []SkippedConflict
}

// Create persists a new template. Exactly one of fromPlateID or entries may
// be provided; both nil creates an empty template. Both set returns
// ErrInvalidInput. Entries must satisfy scope rules:
//
//	slot:  every entry has DayOffset=0 and SlotID=nil
//	day:   every entry has DayOffset=0 and SlotID!=nil
//	week:  every entry has DayOffset in [0,maxDayOffset] and SlotID!=nil
func (s *Service) Create(ctx context.Context, name string, scope Scope, fromPlateID *int64, entries []TemplateEntry) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	if scope == "" {
		scope = ScopeSlot
	}
	if !scope.IsValid() {
		return nil, fmt.Errorf("%w: unknown scope %q", domain.ErrInvalidInput, string(scope))
	}
	if fromPlateID != nil && len(entries) > 0 {
		return nil, fmt.Errorf("%w: provide either from_plate_id or entries, not both", domain.ErrInvalidInput)
	}

	t := &Template{Name: name, Scope: scope}

	if fromPlateID != nil {
		if scope != ScopeSlot {
			return nil, fmt.Errorf("%w: from_plate_id is only valid for slot scope", domain.ErrInvalidInput)
		}
		src, err := s.plates.ListComponentsByPlate(ctx, *fromPlateID)
		if err != nil {
			return nil, err
		}
		t.Entries = make([]TemplateEntry, len(src))
		for i, pc := range src {
			t.Entries[i] = TemplateEntry{
				FoodID:    pc.FoodID,
				Portions:  pc.Portions,
				SortOrder: i,
			}
		}
	} else {
		t.Entries = make([]TemplateEntry, len(entries))
		for i, e := range entries {
			if e.FoodID <= 0 {
				return nil, fmt.Errorf("%w: entries[%d] food_id required", domain.ErrInvalidInput, i)
			}
			if err := validateEntryForScope(scope, e, i); err != nil {
				return nil, err
			}
			exists, err := s.foods.Exists(ctx, e.FoodID)
			if err != nil {
				return nil, fmt.Errorf("check food %d: %w", e.FoodID, err)
			}
			if !exists {
				return nil, fmt.Errorf("%w: food %d does not exist", domain.ErrNotFound, e.FoodID)
			}
			portions := e.Portions
			if portions <= 0 {
				portions = 1
			}
			t.Entries[i] = TemplateEntry{
				FoodID:    e.FoodID,
				Portions:  portions,
				SortOrder: i,
				DayOffset: e.DayOffset,
				SlotID:    e.SlotID,
				Note:      e.Note,
			}
		}
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func validateEntryForScope(scope Scope, e TemplateEntry, i int) error {
	switch scope {
	case ScopeSlot:
		if e.DayOffset != 0 {
			return fmt.Errorf("%w: entries[%d] slot scope requires day_offset=0", domain.ErrInvalidInput, i)
		}
		if e.SlotID != nil {
			return fmt.Errorf("%w: entries[%d] slot scope must not set slot_id", domain.ErrInvalidInput, i)
		}
	case ScopeDay:
		if e.DayOffset != 0 {
			return fmt.Errorf("%w: entries[%d] day scope requires day_offset=0", domain.ErrInvalidInput, i)
		}
		if e.SlotID == nil || *e.SlotID <= 0 {
			return fmt.Errorf("%w: entries[%d] day scope requires slot_id", domain.ErrInvalidInput, i)
		}
	case ScopeWeek:
		if e.DayOffset < 0 || e.DayOffset > maxDayOffset {
			return fmt.Errorf("%w: entries[%d] day_offset out of range", domain.ErrInvalidInput, i)
		}
		if e.SlotID == nil || *e.SlotID <= 0 {
			return fmt.Errorf("%w: entries[%d] week scope requires slot_id", domain.ErrInvalidInput, i)
		}
	}
	return nil
}

// Get returns a template with its entries loaded.
func (s *Service) Get(ctx context.Context, id int64) (*Template, error) {
	return s.repo.Get(ctx, id)
}

// List returns all templates with entries loaded.
func (s *Service) List(ctx context.Context) ([]Template, error) {
	return s.repo.List(ctx)
}

// ListByScope returns templates filtered to a single scope.
func (s *Service) ListByScope(ctx context.Context, scope Scope) ([]Template, error) {
	if !scope.IsValid() {
		return nil, fmt.Errorf("%w: unknown scope %q", domain.ErrInvalidInput, string(scope))
	}
	return s.repo.ListByScope(ctx, scope)
}

// UpdateName renames an existing template.
func (s *Service) UpdateName(ctx context.Context, id int64, name string) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	return s.repo.UpdateName(ctx, id, name)
}

// Delete removes a template (cascades to template_entries via FK).
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ApplyToPlate copies the template's entries onto the given plate, transactionally.
//
//	merge=false: replaces plate components with the template's entries.
//	merge=true:  appends the template's entries after existing components.
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
			for i, te := range t.Entries {
				pc := &plate.PlateComponent{
					PlateID:   p.ID,
					FoodID:    te.FoodID,
					Portions:  te.Portions,
					SortOrder: i,
				}
				if err := pr.CreateComponent(ctx, pc); err != nil {
					return fmt.Errorf("add template entry: %w", err)
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
		for i, te := range t.Entries {
			pc := &plate.PlateComponent{
				PlateID:   p.ID,
				FoodID:    te.FoodID,
				Portions:  te.Portions,
				SortOrder: next + i,
			}
			if err := pr.CreateComponent(ctx, pc); err != nil {
				return fmt.Errorf("append template entry: %w", err)
			}
		}
		_ = tr
		return nil
	})
}

// Apply creates new dated plates from a template, branching on the template's
// scope. See ApplyPayload for required fields per scope. day/week scope honour
// payload.Conflict (default ConflictSkip).
func (s *Service) Apply(ctx context.Context, templateID int64, payload ApplyPayload) (*ApplyResult, error) {
	t, err := s.repo.Get(ctx, templateID)
	if err != nil {
		return nil, err
	}

	switch t.Scope {
	case ScopeSlot, "":
		return s.applySlot(ctx, t, payload)
	case ScopeDay:
		return s.applyDay(ctx, t, payload)
	case ScopeWeek:
		return s.applyWeek(ctx, t, payload)
	default:
		return nil, fmt.Errorf("%w: template has unknown scope %q", domain.ErrInvalidInput, string(t.Scope))
	}
}

func (s *Service) applySlot(ctx context.Context, t *Template, payload ApplyPayload) (*ApplyResult, error) {
	if payload.Date == nil || payload.Date.IsZero() {
		return nil, fmt.Errorf("%w: date required", domain.ErrInvalidInput)
	}
	if payload.SlotID == nil || *payload.SlotID <= 0 {
		return nil, fmt.Errorf("%w: slot_id required", domain.ErrInvalidInput)
	}
	if len(t.Entries) == 0 {
		return &ApplyResult{Created: []plate.Plate{}}, nil
	}

	// Group entries by day_offset (legacy slot templates may carry multiple).
	type offsetGroup struct {
		offset  int
		entries []TemplateEntry
	}
	seen := make(map[int]int)
	var groups []offsetGroup
	for _, te := range t.Entries {
		idx, ok := seen[te.DayOffset]
		if !ok {
			idx = len(groups)
			seen[te.DayOffset] = idx
			groups = append(groups, offsetGroup{offset: te.DayOffset})
		}
		groups[idx].entries = append(groups[idx].entries, te)
	}

	var created []plate.Plate
	if err := s.tx.RunInTemplateTx(ctx, func(_ Repository, pr plate.Repository) error {
		for _, g := range groups {
			date := payload.Date.AddDate(0, 0, g.offset)
			pcs := make([]plate.PlateComponent, len(g.entries))
			for i, te := range g.entries {
				pcs[i] = plate.PlateComponent{
					FoodID:    te.FoodID,
					Portions:  te.Portions,
					SortOrder: i,
				}
			}
			p := &plate.Plate{
				Date:       date,
				SlotID:     *payload.SlotID,
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
	return &ApplyResult{Created: created}, nil
}

func (s *Service) applyDay(ctx context.Context, t *Template, payload ApplyPayload) (*ApplyResult, error) {
	if payload.Date == nil || payload.Date.IsZero() {
		return nil, fmt.Errorf("%w: date required", domain.ErrInvalidInput)
	}
	conflict := payload.Conflict
	if conflict == "" {
		conflict = ConflictSkip
	}
	if conflict != ConflictSkip && conflict != ConflictOverwrite {
		return nil, fmt.Errorf("%w: conflict must be skip or overwrite", domain.ErrInvalidInput)
	}
	return s.applyMultiSlot(ctx, t, *payload.Date, *payload.Date, conflict, t.Entries, false)
}

func (s *Service) applyWeek(ctx context.Context, t *Template, payload ApplyPayload) (*ApplyResult, error) {
	if payload.StartDate == nil || payload.StartDate.IsZero() {
		return nil, fmt.Errorf("%w: start_date required", domain.ErrInvalidInput)
	}
	conflict := payload.Conflict
	if conflict == "" {
		conflict = ConflictSkip
	}
	if conflict != ConflictSkip && conflict != ConflictOverwrite {
		return nil, fmt.Errorf("%w: conflict must be skip or overwrite", domain.ErrInvalidInput)
	}
	end := payload.StartDate.AddDate(0, 0, 6)
	return s.applyMultiSlot(ctx, t, *payload.StartDate, end, conflict, t.Entries, true)
}

// applyMultiSlot is the shared body for day and week scope. It groups entries
// by (date, slot_id), pre-loads existing plates in [from, to] for conflict
// detection, and then creates / overwrites / skips per policy.
func (s *Service) applyMultiSlot(
	ctx context.Context,
	_ *Template,
	from, to time.Time,
	conflict ApplyConflict,
	entries []TemplateEntry,
	useDayOffset bool,
) (*ApplyResult, error) {
	if len(entries) == 0 {
		return &ApplyResult{Created: []plate.Plate{}}, nil
	}

	// Bucket entries by (date, slot_id).
	type cellKey struct {
		date   string
		slotID int64
	}
	type cell struct {
		date    time.Time
		slotID  int64
		entries []TemplateEntry
	}
	cells := make(map[cellKey]*cell)
	var order []cellKey
	for _, te := range entries {
		if te.SlotID == nil {
			return nil, fmt.Errorf("%w: entry missing slot_id", domain.ErrInvalidInput)
		}
		offset := 0
		if useDayOffset {
			offset = te.DayOffset
		}
		date := from.AddDate(0, 0, offset)
		key := cellKey{date: date.Format("2006-01-02"), slotID: *te.SlotID}
		c, ok := cells[key]
		if !ok {
			c = &cell{date: date, slotID: *te.SlotID}
			cells[key] = c
			order = append(order, key)
		}
		c.entries = append(c.entries, te)
	}

	var created []plate.Plate
	var skipped []SkippedConflict

	if err := s.tx.RunInTemplateTx(ctx, func(_ Repository, pr plate.Repository) error {
		existing, err := pr.ListByDateRange(ctx, from, to)
		if err != nil {
			return fmt.Errorf("load existing plates: %w", err)
		}
		occupied := make(map[cellKey]int64) // -> existing plate ID
		for _, p := range existing {
			occupied[cellKey{date: p.DateString(), slotID: p.SlotID}] = p.ID
		}

		for _, key := range order {
			c := cells[key]
			if existingID, taken := occupied[key]; taken {
				if conflict == ConflictSkip {
					skipped = append(skipped, SkippedConflict{Date: c.date, SlotID: c.slotID})
					continue
				}
				if err := pr.Delete(ctx, existingID); err != nil {
					return fmt.Errorf("delete existing plate %d: %w", existingID, err)
				}
			}
			pcs := make([]plate.PlateComponent, len(c.entries))
			for i, te := range c.entries {
				pcs[i] = plate.PlateComponent{
					FoodID:    te.FoodID,
					Portions:  te.Portions,
					SortOrder: i,
				}
			}
			p := &plate.Plate{
				Date:       c.date,
				SlotID:     c.slotID,
				Components: pcs,
			}
			if err := pr.Create(ctx, p); err != nil {
				return fmt.Errorf("create plate %s/slot %d: %w", c.date.Format("2006-01-02"), c.slotID, err)
			}
			created = append(created, *p)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &ApplyResult{Created: created, Skipped: skipped}, nil
}

// maxDayOffset is the maximum allowed day_offset when building a template from plates.
const maxDayOffset = 30

// SaveAsTemplate creates a new template from a set of plates anchored at
// anchorDate. Scope is inferred from the plate range: all plates on anchorDate
// → ScopeDay; spans multiple days → ScopeWeek. Each plate component becomes a
// template entry with day_offset = floor((plate.Date - anchorDate) / 24h) and
// slot_id = plate.SlotID.
func (s *Service) SaveAsTemplate(ctx context.Context, name string, plates []plate.Plate, anchorDate time.Time) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrInvalidInput)
	}
	if len(plates) == 0 {
		return nil, fmt.Errorf("%w: plates must not be empty", domain.ErrInvalidInput)
	}

	maxOffset := 0
	var entries []TemplateEntry
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
		if offsetDays > maxOffset {
			maxOffset = offsetDays
		}
		slotID := p.SlotID
		for i, pc := range p.Components {
			entries = append(entries, TemplateEntry{
				FoodID:    pc.FoodID,
				Portions:  pc.Portions,
				SortOrder: i,
				DayOffset: offsetDays,
				SlotID:    &slotID,
			})
		}
	}

	scope := ScopeWeek
	if maxOffset == 0 {
		uniqueSlots := make(map[int64]struct{})
		for _, e := range entries {
			uniqueSlots[*e.SlotID] = struct{}{}
		}
		if len(uniqueSlots) == 1 {
			scope = ScopeSlot
			for i := range entries {
				entries[i].SlotID = nil
			}
		} else {
			scope = ScopeDay
		}
	}

	for i, e := range entries {
		if err := validateEntryForScope(scope, e, i); err != nil {
			return nil, err
		}
	}

	t := &Template{
		Name:    name,
		Scope:   scope,
		Entries: entries,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}
