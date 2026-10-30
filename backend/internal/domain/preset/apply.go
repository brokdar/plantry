package preset

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// ConflictMode controls Apply behaviour when a target (date, slot) cell is
// already occupied.
type ConflictMode string

const (
	// ConflictSkip leaves existing plates untouched (default).
	ConflictSkip ConflictMode = "skip"
	// ConflictOverwrite replaces existing plates; the result's Removed
	// restores them.
	ConflictOverwrite ConflictMode = "overwrite"
)

// ParseConflict parses a JSON string into a ConflictMode. Empty maps to Skip.
func ParseConflict(s string) (ConflictMode, error) {
	switch s {
	case "":
		return ConflictSkip, nil
	case string(ConflictSkip):
		return ConflictSkip, nil
	case string(ConflictOverwrite):
		return ConflictOverwrite, nil
	default:
		return "", fmt.Errorf("%w: conflict must be skip or overwrite", domain.ErrInvalidInput)
	}
}

func (c ConflictMode) plateMode() plate.ConflictMode {
	if c == ConflictOverwrite {
		return plate.ConflictOverwrite
	}
	return plate.ConflictSkip
}

// ApplyRequest is the input to preset.Service.Apply.
type ApplyRequest struct {
	TargetDate    time.Time
	OnConflict    ConflictMode
	SlotIDsFilter []int64 // optional; if non-empty, only apply plates with these slot_ids
}

// ApplyResult is what Apply and CopyWeek return. Undo is the caller's: delete
// CreatedPlateIDs, restore Removed (POST /api/plan/restore) and clear
// CreatedOverrides.
type ApplyResult struct {
	CreatedPlateIDs []int64
	// SkippedOccupied: target cells holding a plate (ConflictSkip), a second
	// preset plate for one slot, or copy week's "none" cells that hold a
	// plate.
	SkippedOccupied []plate.CellKey
	// SkippedUnavailable: target cells that resolve to "none" (D12).
	SkippedUnavailable []plate.CellKey
	// SkippedNoSlot: target cells whose slot is not active.
	SkippedNoSlot []plate.CellKey
	// Removed restores the plates ConflictOverwrite replaced.
	Removed plate.PlanSnapshot
	// CreatedOverrides are the "none" overrides copy week wrote.
	CreatedOverrides []plate.CellKey
}

func newApplyResult(out *plate.ApplyResult) *ApplyResult {
	return &ApplyResult{
		CreatedPlateIDs:    out.PlateIDs,
		SkippedOccupied:    out.SkippedOccupied,
		SkippedUnavailable: out.SkippedUnavailable,
		Removed:            out.Removed,
	}
}

// NoPlateOverrides reads and writes the "none" overrides copy week carries
// from the source week to the target week.
type NoPlateOverrides interface {
	// NoneOverride returns the note of the cell's "none" override; ok is
	// false when the cell has none.
	NoneOverride(ctx context.Context, date time.Time, slotID int64) (note *string, ok bool, err error)
	// SetNoneIfUnset writes a "none" override on a cell that has no override
	// and is not already "none", and reports whether it wrote one. A plate in
	// the cell fails with domain.ErrCellHasPlate.
	SetNoneIfUnset(ctx context.Context, date time.Time, slotID int64, note *string) (bool, error)
}

// WithAvailability lets copy week carry "none" overrides.
func (s *Service) WithAvailability(a NoPlateOverrides) *Service {
	s.avail = a
	return s
}

// SlotLister is the minimal time_slots surface the apply pipeline needs:
// list all slots so we can determine active state per slot_id.
type SlotLister interface {
	List(ctx context.Context, activeOnly bool) ([]slot.TimeSlot, error)
}

// WithSlots injects a SlotLister so Apply can detect inactive/missing slots.
// Called from the DI graph; existing callers can leave it nil (no slot check).
func (s *Service) WithSlots(sl SlotLister) *Service {
	s.slots = sl
	return s
}

// Apply places a preset's plates onto target_date through plate.Service.Apply:
// one plate per preset slot, composed components on fresh sessions. "none"
// cells are skipped; occupied ones follow OnConflict.
func (s *Service) Apply(ctx context.Context, presetID int64, req ApplyRequest) (*ApplyResult, error) {
	if req.TargetDate.IsZero() {
		return nil, fmt.Errorf("%w: target_date required", domain.ErrInvalidInput)
	}
	conflict, err := ParseConflict(string(req.OnConflict))
	if err != nil {
		return nil, err
	}
	p, err := s.repo.Get(ctx, presetID)
	if err != nil {
		return nil, err
	}
	activeSlots, err := s.activeSlotIDSet(ctx)
	if err != nil {
		return nil, err
	}
	snap, noSlot, duplicate := PresetSnapshot(filterPresetPlates(p.Plates, req.SlotIDsFilter), req.TargetDate, activeSlots)

	var res *ApplyResult
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		out, err := s.plateSvc.Apply(ctx, snap, plate.ApplyOptions{OnConflict: conflict.plateMode()})
		if err != nil {
			return err
		}
		res = newApplyResult(out)
		if len(out.PlateIDs) == 0 {
			return nil
		}
		if err := s.repo.TouchLastUsed(ctx, presetID); err != nil {
			return fmt.Errorf("touch last_used_at: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	res.SkippedNoSlot = noSlot
	res.SkippedOccupied = append(res.SkippedOccupied, duplicate...)
	return res, nil
}

// CopyWeekRequest copies the plates from [source_start, source_start+6 days]
// onto [target_start, target_start+6 days], preserving slot bindings.
type CopyWeekRequest struct {
	SourceStart time.Time
	TargetStart time.Time
	OnConflict  ConflictMode
}

// CopyWeek copies the source window's plates onto the target window through
// plate.Service.Apply (see CopySnapshot for sessions). No preset is created
// or consumed.
//
// Availability (D12): a source plate whose target cell resolves to "none" is
// not copied and is reported in SkippedUnavailable. The source week's "none"
// overrides are copied onto the matching target cells; a target cell that
// holds a plate keeps it and is reported in SkippedOccupied (whatever the
// conflict mode), and a target cell that already has an override or is
// already "none" is left alone. Routine overrides are not copied.
func (s *Service) CopyWeek(ctx context.Context, req CopyWeekRequest) (*ApplyResult, error) {
	if req.SourceStart.IsZero() || req.TargetStart.IsZero() {
		return nil, fmt.Errorf("%w: source_start and target_start required", domain.ErrInvalidInput)
	}
	conflict, err := ParseConflict(string(req.OnConflict))
	if err != nil {
		return nil, err
	}
	activeSlots, err := s.activeSlotIDSet(ctx)
	if err != nil {
		return nil, err
	}
	noPlate, err := s.sourceNoPlateCells(ctx, req.SourceStart, activeSlots)
	if err != nil {
		return nil, err
	}
	shiftDays := int(req.TargetStart.Sub(req.SourceStart).Hours() / 24)

	var res *ApplyResult
	if err := s.tx.InTx(ctx, func(ctx context.Context) error {
		sources, err := s.plateSvc.Range(ctx, req.SourceStart, req.SourceStart.AddDate(0, 0, 6))
		if err != nil {
			return fmt.Errorf("load source week: %w", err)
		}
		sessions, err := s.sourceSessions(ctx, sources)
		if err != nil {
			return err
		}
		snap, noSlot := CopySnapshot(sources, sessions, req.SourceStart, shiftDays, activeSlots)
		out, err := s.plateSvc.Apply(ctx, snap, plate.ApplyOptions{OnConflict: conflict.plateMode()})
		if err != nil {
			return err
		}
		res = newApplyResult(out)
		res.SkippedNoSlot = noSlot
		return nil
	}); err != nil {
		return nil, err
	}

	// The plates are committed, so the result (and what undoes it) must reach
	// the caller: copying an override is best-effort. A cell holding a plate
	// is reported as occupied; other failures are logged and the cell is
	// left as it was.
	for _, c := range noPlate {
		td := c.Date.AddDate(0, 0, shiftDays)
		created, err := s.avail.SetNoneIfUnset(ctx, td, c.SlotID, c.Note)
		switch {
		case errors.Is(err, domain.ErrCellHasPlate):
			res.SkippedOccupied = append(res.SkippedOccupied, plate.CellKey{Date: td, SlotID: c.SlotID})
		case err != nil:
			slog.WarnContext(ctx, "copy_week.override_copy_failed",
				"date", td.Format(dateLayout), "slot_id", c.SlotID, "error", err)
		case created:
			res.CreatedOverrides = append(res.CreatedOverrides, plate.CellKey{Date: td, SlotID: c.SlotID})
		}
	}
	return res, nil
}

// sourceSessions loads the sessions the sources' composed components eat from.
func (s *Service) sourceSessions(ctx context.Context, sources []plate.Plate) (map[int64]plate.CookSession, error) {
	out := map[int64]plate.CookSession{}
	for _, p := range sources {
		for _, c := range p.Components {
			if c.CookSessionID == nil {
				continue
			}
			if _, ok := out[*c.CookSessionID]; ok {
				continue
			}
			v, err := s.plateSvc.Session(ctx, *c.CookSessionID)
			if err != nil {
				return nil, fmt.Errorf("load session %d: %w", *c.CookSessionID, err)
			}
			out[v.ID] = v.CookSession
		}
	}
	return out, nil
}

// noPlateCell is a source "none" override to copy.
type noPlateCell struct {
	Date   time.Time
	SlotID int64
	Note   *string
}

// sourceNoPlateCells returns the "none" overrides of the active slots in the
// 7 days from start, date-major. It returns nothing without an Availability
// or a SlotLister (the active slots are unknown then).
func (s *Service) sourceNoPlateCells(ctx context.Context, start time.Time, activeSlots map[int64]struct{}) ([]noPlateCell, error) {
	if s.avail == nil || activeSlots == nil {
		return nil, nil
	}
	slotIDs := make([]int64, 0, len(activeSlots))
	for id := range activeSlots {
		slotIDs = append(slotIDs, id)
	}
	sort.Slice(slotIDs, func(i, j int) bool { return slotIDs[i] < slotIDs[j] })
	var out []noPlateCell
	for i := range 7 {
		d := start.AddDate(0, 0, i)
		for _, slotID := range slotIDs {
			note, ok, err := s.avail.NoneOverride(ctx, d, slotID)
			if err != nil {
				return nil, fmt.Errorf("read no-plate cell: %w", err)
			}
			if ok {
				out = append(out, noPlateCell{Date: d, SlotID: slotID, Note: note})
			}
		}
	}
	return out, nil
}

func filterPresetPlates(in []Plate, slotIDs []int64) []Plate {
	if len(slotIDs) == 0 {
		return in
	}
	keep := make(map[int64]struct{}, len(slotIDs))
	for _, id := range slotIDs {
		keep[id] = struct{}{}
	}
	out := make([]Plate, 0, len(in))
	for _, p := range in {
		if _, ok := keep[p.SlotID]; ok {
			out = append(out, p)
		}
	}
	return out
}

// activeSlotIDSet returns the set of active slot IDs. When no SlotLister is
// injected (tests), returns nil; callers must treat nil as "all slot IDs are
// considered active" via isSlotActive.
func (s *Service) activeSlotIDSet(ctx context.Context) (map[int64]struct{}, error) {
	if s.slots == nil {
		return nil, nil
	}
	all, err := s.slots.List(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list slots: %w", err)
	}
	out := make(map[int64]struct{}, len(all))
	for _, sl := range all {
		if sl.Active {
			out[sl.ID] = struct{}{}
		}
	}
	return out, nil
}

// isSlotActive returns true when slot id should be treated as active. A nil
// map means "no slot service was injected" — treat everything as active.
func isSlotActive(set map[int64]struct{}, id int64) bool {
	if set == nil {
		return true
	}
	_, ok := set[id]
	return ok
}
