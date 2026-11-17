package availability

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

const dateLayout = "2006-01-02"

// Service holds availability business logic.
type Service struct {
	repo    Repository
	tx      domain.Transactor
	presets PresetLookup
	plates  CellPlates
}

// NewService creates an availability service. WithPlates must be called
// before SetOverride can mark a cell "none".
func NewService(repo Repository, tx domain.Transactor, presets PresetLookup) *Service {
	return &Service{repo: repo, tx: tx, presets: presets}
}

// WithPlates injects the plate service. It is a setter because the plate
// service itself depends on availability (CellMode).
func (s *Service) WithPlates(p CellPlates) *Service {
	s.plates = p
	return s
}

// Defaults returns the full weekday × active-slot grid, weekday-major in slot
// order. Cells without a stored row are "plan".
func (s *Service) Defaults(ctx context.Context) ([]Default, error) {
	slots, err := s.repo.ActiveSlotIDs(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListDefaults(ctx)
	if err != nil {
		return nil, err
	}
	stored := make(map[defaultKey]Default, len(rows))
	for _, r := range rows {
		stored[defaultKey{r.Weekday, r.SlotID}] = r
	}
	out := make([]Default, 0, 7*len(slots))
	for wd := 0; wd < 7; wd++ {
		for _, slotID := range slots {
			if d, ok := stored[defaultKey{wd, slotID}]; ok {
				out = append(out, d)
				continue
			}
			out = append(out, Default{Weekday: wd, SlotID: slotID, Mode: ModePlan})
		}
	}
	return out, nil
}

// SetDefaults validates every row and replaces the defaults of all active
// slots in one transaction. "plan" rows are not stored. Unlike SetOverride,
// a default change never deletes or blocks on plates already in its cells,
// even when it makes them "none": explicit plates are user content, and the
// Phase 3 checker reports plates in "none" cells.
func (s *Service) SetDefaults(ctx context.Context, rows []Default) error {
	seen := make(map[defaultKey]bool, len(rows))
	keep := make([]Default, 0, len(rows))
	for _, r := range rows {
		if r.Weekday < 0 || r.Weekday > 6 {
			return fmt.Errorf("%w: weekday %d out of range 0-6", domain.ErrInvalidInput, r.Weekday)
		}
		k := defaultKey{r.Weekday, r.SlotID}
		if seen[k] {
			return fmt.Errorf("%w: duplicate default for weekday %d slot %d", domain.ErrInvalidInput, r.Weekday, r.SlotID)
		}
		seen[k] = true
		if _, err := s.validateCell(ctx, r.SlotID, r.Mode, r.PresetID, domain.ErrAvailabilitySlotUnknown); err != nil {
			return err
		}
		if r.Mode != ModePlan {
			keep = append(keep, Default{Weekday: r.Weekday, SlotID: r.SlotID, Mode: r.Mode, PresetID: r.PresetID})
		}
	}
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		return s.repo.ReplaceActiveDefaults(ctx, keep)
	})
}

// Resolve returns every date in [from, to] × active slot, date-major in slot
// order, with override > default > plan. It reads each table once. The
// range is not capped here: callers must bound it (the HTTP handler allows
// at most 62 days).
func (s *Service) Resolve(ctx context.Context, from, to time.Time) ([]Cell, error) {
	from, to = dateOnly(from), dateOnly(to)
	if to.Before(from) {
		return nil, fmt.Errorf("%w: to before from", domain.ErrInvalidInput)
	}
	slots, err := s.repo.ActiveSlotIDs(ctx)
	if err != nil {
		return nil, err
	}
	defaults, err := s.repo.ListDefaults(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := s.repo.ListOverrides(ctx, from, to)
	if err != nil {
		return nil, err
	}
	defByKey := make(map[defaultKey]Default, len(defaults))
	for _, d := range defaults {
		defByKey[defaultKey{d.Weekday, d.SlotID}] = d
	}
	ovByKey := make(map[cellKey]Override, len(overrides))
	for _, o := range overrides {
		ovByKey[cellKey{o.Date.Format(dateLayout), o.SlotID}] = o
	}

	days := int(to.Sub(from).Hours()/24) + 1
	out := make([]Cell, 0, days*len(slots))
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		ds, wd := d.Format(dateLayout), WeekdayOf(d)
		for _, slotID := range slots {
			var o *Override
			if v, ok := ovByKey[cellKey{ds, slotID}]; ok {
				o = &v
			}
			var def *Default
			if v, ok := defByKey[defaultKey{wd, slotID}]; ok {
				def = &v
			}
			out = append(out, resolve(d, slotID, o, def))
		}
	}
	return out, nil
}

// ModeAt resolves the mode of one cell. It does not check the slot.
func (s *Service) ModeAt(ctx context.Context, date time.Time, slotID int64) (Mode, error) {
	date = dateOnly(date)
	o, err := s.repo.GetOverride(ctx, date, slotID)
	if err != nil {
		return "", err
	}
	if o != nil {
		return o.Mode, nil
	}
	d, err := s.repo.GetDefault(ctx, WeekdayOf(date), slotID)
	if err != nil {
		return "", err
	}
	return resolve(date, slotID, nil, d).Mode, nil
}

// CellMode is ModeAt as a plain string ("plan", "routine" or "none"), so a
// port in another domain (plate.CellChecker, D10) can be satisfied without
// importing this package.
func (s *Service) CellMode(ctx context.Context, date time.Time, slotID int64) (string, error) {
	m, err := s.ModeAt(ctx, date, slotID)
	return string(m), err
}

// SetOverride writes the override of one cell: SetOverrideMeals with the
// removal's snapshot only (nil when no plate was removed).
func (s *Service) SetOverride(ctx context.Context, in OverrideInput) (Cell, *plate.PlanSnapshot, error) {
	cell, res, err := s.SetOverrideMeals(ctx, in)
	if err != nil || res == nil || len(res.Removed.Plates) == 0 {
		return cell, nil, err
	}
	return cell, &res.Removed, nil
}

// SetOverrideMeals writes the override of one cell. Setting "none" on a cell
// that holds a plate fails with ErrCellHasPlate unless in.RemovePlate is set;
// then the plate is removed through plate.Service.RemoveMealsAt in the same
// unit of work, which also re-anchors the batches cooking there (R1). The
// result is nil unless the mode is "none".
func (s *Service) SetOverrideMeals(ctx context.Context, in OverrideInput) (Cell, *plate.MealResult, error) {
	if in.Date.IsZero() {
		return Cell{}, nil, fmt.Errorf("%w: date required", domain.ErrInvalidInput)
	}
	date := dateOnly(in.Date)
	presetName, err := s.validateCell(ctx, in.SlotID, in.Mode, in.PresetID, domain.ErrNotFound)
	if err != nil {
		return Cell{}, nil, err
	}
	var note *string
	if in.Mode == ModeNone {
		if note, err = normaliseNote(in.Note); err != nil {
			return Cell{}, nil, err
		}
	}

	var res *plate.MealResult
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		if in.Mode == ModeNone && !in.RemovePlate {
			if err := s.refuseCellPlate(ctx, date, in.SlotID); err != nil {
				return err
			}
		}
		if err := s.repo.UpsertOverride(ctx, Override{
			Date: date, SlotID: in.SlotID, Mode: in.Mode, PresetID: in.PresetID, Note: note,
		}); err != nil {
			return err
		}
		if in.Mode != ModeNone {
			return nil
		}
		r, err := s.removeMeals(ctx, []plate.CellKey{{Date: date, SlotID: in.SlotID}})
		res = &r
		return err
	})
	if err != nil {
		return Cell{}, nil, err
	}
	return Cell{
		Date: date, SlotID: in.SlotID, Mode: in.Mode, Source: SourceOverride,
		PresetID: in.PresetID, PresetName: presetName, Note: note,
	}, res, nil
}

// AwayInput names the cells to mark away: Cells, or with no cells every
// active slot of Date whose mode is not routine. Today bounds them: a cell
// before it is ErrPastCell.
type AwayInput struct {
	Cells []plate.CellKey
	Date  *time.Time
	Note  *string
	Today time.Time
}

// CellOverride is a cell's override as it was before a change; Override nil
// means the cell had none.
type CellOverride struct {
	Date     time.Time
	SlotID   int64
	Override *Override
}

// AwayResult is what SetAway did: the cells after, each cell's override
// before, the plates removed (Removed restores them through Apply) and the
// batches whose cook moved.
type AwayResult struct {
	Cells       []Cell
	Before      []CellOverride
	Removed     plate.PlanSnapshot
	AlsoChanged []plate.AlsoChanged
	// Undo reverts the whole call (Revert): the plate events and every
	// cell's override.
	Undo plate.Undo
}

// SetAway marks cells "none" and removes their plates in one unit of work
// (plate.Service.RemoveMealsAt, so a batch cooking there re-anchors to its
// earliest remaining meal). A cell that already has a "none" override keeps
// its note unless in.Note is given. Confirmed plates follow the plate
// service's actor guard.
func (s *Service) SetAway(ctx context.Context, in AwayInput) (AwayResult, error) {
	note, err := normaliseNote(in.Note)
	if err != nil {
		return AwayResult{}, err
	}
	today := dateOnly(in.Today)
	var res AwayResult
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		cells, err := s.awayCells(ctx, in)
		if err != nil {
			return err
		}
		res = AwayResult{Cells: make([]Cell, 0, len(cells)), Before: make([]CellOverride, 0, len(cells))}
		undoCells := make([]plate.CellUndo, 0, len(cells))
		for _, c := range cells {
			if c.Date.Before(today) {
				return fmt.Errorf("%w: %s", domain.ErrPastCell, c.Date.Format(dateLayout))
			}
			if _, err := s.validateCell(ctx, c.SlotID, ModeNone, nil, domain.ErrAvailabilitySlotUnknown); err != nil {
				return err
			}
			prev, err := s.repo.GetOverride(ctx, c.Date, c.SlotID)
			if err != nil {
				return err
			}
			res.Before = append(res.Before, CellOverride{Date: c.Date, SlotID: c.SlotID, Override: prev})
			n := note
			if n == nil && prev != nil && prev.Mode == ModeNone {
				n = prev.Note
			}
			if err := s.repo.UpsertOverride(ctx, Override{Date: c.Date, SlotID: c.SlotID, Mode: ModeNone, Note: n}); err != nil {
				return err
			}
			res.Cells = append(res.Cells, Cell{Date: c.Date, SlotID: c.SlotID, Mode: ModeNone, Source: SourceOverride, Note: n})
			undoCells = append(undoCells, plate.CellUndo{
				Date: c.Date.Format(dateLayout), SlotID: c.SlotID, Before: undoOverride(prev),
				After: &plate.CellOverride{Mode: string(ModeNone), Note: n},
			})
		}
		removed, err := s.removeMeals(ctx, cells)
		if err != nil {
			return err
		}
		res.Removed, res.AlsoChanged, res.Undo = removed.Removed, removed.AlsoChanged, removed.Undo
		res.Undo.Cells = undoCells
		return nil
	})
	if err != nil {
		return AwayResult{}, err
	}
	return res, nil
}

// Revert takes back a hand edit's undo token in one unit of work: the plate
// events through plate.Service.RevertRange and each cell's override back to
// Before. A cell whose override is no longer After, or a row written after
// the events, is domain.ErrPlanChangeStale and nothing is written; a token
// without events or cells is domain.ErrInvalidInput.
func (s *Service) Revert(ctx context.Context, u plate.Undo) (plate.RowSet, error) {
	if u.FirstEventID == 0 && len(u.Cells) == 0 {
		return plate.RowSet{}, fmt.Errorf("%w: nothing to revert", domain.ErrInvalidInput)
	}
	if s.plates == nil {
		return plate.RowSet{}, errPlatesNotWired
	}
	rows := plate.RowSet{PlateIDs: []int64{}, SessionIDs: []int64{}}
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		dates := make([]time.Time, len(u.Cells))
		for i, c := range u.Cells {
			d, err := time.Parse(dateLayout, c.Date)
			if err != nil {
				return fmt.Errorf("%w: cell date %q", domain.ErrInvalidInput, c.Date)
			}
			dates[i] = d
			cur, err := s.repo.GetOverride(ctx, d, c.SlotID)
			if err != nil {
				return err
			}
			if !sameOverride(undoOverride(cur), c.After) {
				return fmt.Errorf("%w: cell %s slot %d was edited since", domain.ErrPlanChangeStale, c.Date, c.SlotID)
			}
		}
		if u.FirstEventID != 0 {
			var err error
			if rows, err = s.plates.RevertRange(ctx, u.FirstEventID, u.LastEventID); err != nil {
				return err
			}
		}
		for i, c := range u.Cells {
			if err := s.RestoreOverride(ctx, dates[i], c.SlotID, fromUndo(c.Before)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return plate.RowSet{}, err
	}
	return rows, nil
}

func undoOverride(o *Override) *plate.CellOverride {
	if o == nil {
		return nil
	}
	return &plate.CellOverride{Mode: string(o.Mode), PresetID: o.PresetID, Note: o.Note}
}

func fromUndo(o *plate.CellOverride) *Override {
	if o == nil {
		return nil
	}
	return &Override{Mode: Mode(o.Mode), PresetID: o.PresetID, Note: o.Note}
}

func sameOverride(a, b *plate.CellOverride) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Mode == b.Mode && equalPtr(a.PresetID, b.PresetID) && equalPtr(a.Note, b.Note)
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// awayCells resolves AwayInput to distinct cells: the given ones, or every
// active slot of Date that does not resolve to routine.
func (s *Service) awayCells(ctx context.Context, in AwayInput) ([]plate.CellKey, error) {
	var out []plate.CellKey
	if len(in.Cells) == 0 {
		if in.Date == nil {
			return nil, fmt.Errorf("%w: cells or date required", domain.ErrInvalidInput)
		}
		day, err := s.Resolve(ctx, *in.Date, *in.Date)
		if err != nil {
			return nil, err
		}
		for _, c := range day {
			if c.Mode != ModeRoutine {
				out = append(out, plate.CellKey{Date: c.Date, SlotID: c.SlotID})
			}
		}
		return out, nil
	}
	for _, c := range in.Cells {
		if c.Date.IsZero() {
			return nil, fmt.Errorf("%w: date required", domain.ErrInvalidInput)
		}
		k := plate.CellKey{Date: dateOnly(c.Date), SlotID: c.SlotID}
		if !slices.ContainsFunc(out, k.Equal) {
			out = append(out, k)
		}
	}
	return out, nil
}

// NoneOverride returns the note of the cell's "none" override. ok is false
// when the cell has no override or one of another mode. Copy week uses it to
// carry No-plate cells over (D12).
func (s *Service) NoneOverride(ctx context.Context, date time.Time, slotID int64) (note *string, ok bool, err error) {
	o, err := s.repo.GetOverride(ctx, dateOnly(date), slotID)
	if err != nil || o == nil || o.Mode != ModeNone {
		return nil, false, err
	}
	return o.Note, true, nil
}

// SetNoneIfUnset writes a "none" override with note on a cell that has no
// override and does not already resolve to "none", and reports whether it
// wrote one. A plate in the cell fails with ErrCellHasPlate. Copy week uses
// it so that undo can clear exactly the overrides it created (D12).
func (s *Service) SetNoneIfUnset(ctx context.Context, date time.Time, slotID int64, note *string) (bool, error) {
	o, err := s.repo.GetOverride(ctx, dateOnly(date), slotID)
	if err != nil {
		return false, err
	}
	if o != nil {
		return false, nil
	}
	mode, err := s.ModeAt(ctx, date, slotID)
	if err != nil {
		return false, err
	}
	if mode == ModeNone {
		return false, nil
	}
	if _, _, err := s.SetOverride(ctx, OverrideInput{Date: date, SlotID: slotID, Mode: ModeNone, Note: note}); err != nil {
		return false, err
	}
	return true, nil
}

// Override returns the cell's stored override, nil when it has none.
func (s *Service) Override(ctx context.Context, date time.Time, slotID int64) (*Override, error) {
	return s.repo.GetOverride(ctx, dateOnly(date), slotID)
}

// RestoreOverride puts back an override Override returned earlier: o is
// written as it is, nil clears the cell's override. It skips SetOverride's
// checks because it restores a state that was valid (the undo of an agent
// change, Phase 4 D13); the caller restores the cell's plate itself.
func (s *Service) RestoreOverride(ctx context.Context, date time.Time, slotID int64, o *Override) error {
	if o == nil {
		return s.repo.DeleteOverride(ctx, dateOnly(date), slotID)
	}
	return s.repo.UpsertOverride(ctx, Override{
		Date: dateOnly(date), SlotID: slotID, Mode: o.Mode, PresetID: o.PresetID, Note: o.Note,
	})
}

// ClearOverride removes the override of one cell. It is idempotent.
func (s *Service) ClearOverride(ctx context.Context, date time.Time, slotID int64) error {
	return s.repo.DeleteOverride(ctx, dateOnly(date), slotID)
}

// refuseCellPlate fails with ErrCellHasPlate when the cell holds a plate.
func (s *Service) refuseCellPlate(ctx context.Context, date time.Time, slotID int64) error {
	if s.plates == nil {
		return errPlatesNotWired
	}
	day, err := s.plates.Range(ctx, date, date)
	if err != nil {
		return err
	}
	for _, p := range day {
		if p.SlotID == slotID {
			return fmt.Errorf("%w: plate %d", domain.ErrCellHasPlate, p.ID)
		}
	}
	return nil
}

// removeMeals removes the plates on cells, which are "none" by now, through
// the plate service.
func (s *Service) removeMeals(ctx context.Context, cells []plate.CellKey) (plate.MealResult, error) {
	if s.plates == nil {
		return plate.MealResult{}, errPlatesNotWired
	}
	return s.plates.RemoveMealsAt(ctx, cells)
}

var errPlatesNotWired = errors.New("availability: plate service not wired")

// normaliseNote trims a "none" note; empty becomes nil, longer than
// MaxNoteLen characters is invalid.
func normaliseNote(in *string) (*string, error) {
	if in == nil {
		return nil, nil
	}
	n := strings.TrimSpace(*in)
	if utf8.RuneCountInString(n) > MaxNoteLen {
		return nil, fmt.Errorf("%w: note longer than %d characters", domain.ErrInvalidInput, MaxNoteLen)
	}
	if n == "" {
		return nil, nil
	}
	return &n, nil
}

// validateCell checks the slot, mode and preset of a default or override and
// returns the preset name for a routine. A missing slot fails with
// unknownSlot: ErrNotFound when the slot is the URL resource (override),
// ErrAvailabilitySlotUnknown when it is named in the body (defaults).
func (s *Service) validateCell(ctx context.Context, slotID int64, mode Mode, presetID *int64, unknownSlot error) (*string, error) {
	if err := validateMode(mode); err != nil {
		return nil, err
	}
	exists, active, err := s.repo.SlotState(ctx, slotID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: slot %d", unknownSlot, slotID)
	}
	if !active {
		return nil, fmt.Errorf("%w: slot %d", domain.ErrSlotInactive, slotID)
	}
	if mode != ModeRoutine {
		if presetID != nil {
			return nil, fmt.Errorf("%w: mode %q", domain.ErrPresetNotAllowed, mode)
		}
		return nil, nil
	}
	if presetID == nil {
		return nil, domain.ErrPresetRequired
	}
	name, ok, err := s.presets.PresetName(ctx, *presetID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: preset %d", domain.ErrAvailabilityPresetUnknown, *presetID)
	}
	return &name, nil
}

// resolve applies override > default > plan to one cell.
func resolve(date time.Time, slotID int64, o *Override, d *Default) Cell {
	c := Cell{Date: date, SlotID: slotID}
	switch {
	case o != nil:
		c.Mode, c.Source, c.PresetID, c.PresetName, c.Note = o.Mode, SourceOverride, o.PresetID, o.PresetName, o.Note
	case d != nil:
		c.Mode, c.Source, c.PresetID, c.PresetName = d.Mode, SourceDefault, d.PresetID, d.PresetName
	default:
		c.Mode, c.Source = ModePlan, SourceFallback
	}
	return c
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type defaultKey struct {
	weekday int
	slotID  int64
}

type cellKey struct {
	date   string
	slotID int64
}

// DryRun runs fn (a mutation of this service) in a unit of work that is
// rolled back: the away preview (?dry_run=1).
func (s *Service) DryRun(ctx context.Context, fn func(ctx context.Context) error) error {
	return domain.DryRun(ctx, s.tx, fn)
}
