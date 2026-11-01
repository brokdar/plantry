package availability

import (
	"context"
	"errors"
	"fmt"
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

// SetOverride writes the override of one cell. Setting "none" on a cell that
// holds a plate fails with ErrCellHasPlate unless in.RemovePlate is set; then
// the plate is deleted through the plate service in the same unit of work and
// the snapshot that restores it is returned (nil when nothing was removed).
func (s *Service) SetOverride(ctx context.Context, in OverrideInput) (Cell, *plate.PlanSnapshot, error) {
	if in.Date.IsZero() {
		return Cell{}, nil, fmt.Errorf("%w: date required", domain.ErrInvalidInput)
	}
	date := dateOnly(in.Date)
	presetName, err := s.validateCell(ctx, in.SlotID, in.Mode, in.PresetID, domain.ErrNotFound)
	if err != nil {
		return Cell{}, nil, err
	}
	var note *string
	if in.Mode == ModeNone && in.Note != nil {
		n := strings.TrimSpace(*in.Note)
		if utf8.RuneCountInString(n) > MaxNoteLen {
			return Cell{}, nil, fmt.Errorf("%w: note longer than %d characters", domain.ErrInvalidInput, MaxNoteLen)
		}
		if n != "" {
			note = &n
		}
	}

	var removed *plate.PlanSnapshot
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		if in.Mode == ModeNone {
			r, err := s.removeCellPlate(ctx, date, in.SlotID, in.RemovePlate)
			if err != nil {
				return err
			}
			removed = r
		}
		return s.repo.UpsertOverride(ctx, Override{
			Date: date, SlotID: in.SlotID, Mode: in.Mode, PresetID: in.PresetID, Note: note,
		})
	})
	if err != nil {
		return Cell{}, nil, err
	}
	return Cell{
		Date: date, SlotID: in.SlotID, Mode: in.Mode, Source: SourceOverride,
		PresetID: in.PresetID, PresetName: presetName, Note: note,
	}, removed, nil
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

// removeCellPlate deletes the cell's plate when remove is set and returns the
// snapshot that restores it, or nil when the cell is empty. Without remove, a
// plate in the cell is an error.
func (s *Service) removeCellPlate(ctx context.Context, date time.Time, slotID int64, remove bool) (*plate.PlanSnapshot, error) {
	if s.plates == nil {
		return nil, errors.New("availability: plate service not wired")
	}
	day, err := s.plates.Range(ctx, date, date)
	if err != nil {
		return nil, err
	}
	for _, p := range day {
		if p.SlotID != slotID {
			continue
		}
		if !remove {
			return nil, fmt.Errorf("%w: plate %d", domain.ErrCellHasPlate, p.ID)
		}
		snap, err := s.plates.Delete(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		return &snap, nil
	}
	return nil, nil
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
