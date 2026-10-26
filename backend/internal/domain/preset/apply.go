package preset

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
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
	// ConflictOverwrite replaces existing plates and captures a pre-replace
	// snapshot for undo.
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

// ApplyRequest is the input to preset.Service.Apply.
type ApplyRequest struct {
	TargetDate    time.Time
	OnConflict    ConflictMode
	SlotIDsFilter []int64 // optional; if non-empty, only apply plates with these slot_ids
}

// ApplyResult is what Apply returns. The Snapshot allows the caller to undo
// the operation via UndoApply.
type ApplyResult struct {
	Created         []plate.Plate  // new plates materialised
	Replaced        []ReplacedItem // overwritten plates (only with ConflictOverwrite)
	SkippedOccupied []SkipItem     // {Date, SlotID} for plates skipped due to conflict
	SkippedNoSlot   []SkipItem     // {SlotID} for plates whose slot was inactive/missing on the day
	// SkippedUnavailable lists {Date, SlotID} target cells that resolve to
	// "none", so no plate was placed there (D12).
	SkippedUnavailable []SkipItem
	Snapshot           ApplySnapshot // for UndoApply
}

// ReplacedItem captures one overwrite: the new plate and the old plate that
// was removed to make room. The old plate's Components are populated so the
// snapshot can fully restore it.
type ReplacedItem struct {
	NewPlate plate.Plate
	OldPlate plate.Plate
}

// SkipItem identifies a (date, slot) that was not materialised.
type SkipItem struct {
	Date   time.Time
	SlotID int64
}

// ApplySnapshot is the server-side blob that UndoApply consumes to reverse a
// successful Apply. It contains the IDs of the plates that were created by
// the apply (so they can be deleted) and the pre-replace plates (so they can
// be restored). Single-user LAN context — not signed.
type ApplySnapshot struct {
	CreatedPlateIDs []int64
	ReplacedPlates  []plate.Plate
	// CreatedOverrides are the "none" overrides copy week wrote; undo clears them.
	CreatedOverrides []SkipItem
}

// cellNone is the CellChecker mode of a No-plate cell.
const cellNone = "none"

// CellChecker resolves a cell's availability mode ("plan", "routine" or
// "none"). Same shape as plate.CellChecker, so the availability service
// satisfies both (D12).
type CellChecker interface {
	CellMode(ctx context.Context, date time.Time, slotID int64) (string, error)
}

// NoPlateOverrides reads and writes the "none" overrides copy week carries
// from the source week to the target week.
type NoPlateOverrides interface {
	// NoneOverride returns the note of the cell's "none" override; ok is
	// false when the cell has none.
	NoneOverride(ctx context.Context, date time.Time, slotID int64) (note *string, ok bool, err error)
	// SetNoneIfUnset writes a "none" override on a cell that has no override
	// and is not already "none", and reports whether it wrote one.
	SetNoneIfUnset(ctx context.Context, date time.Time, slotID int64, note *string) (bool, error)
	// ClearOverride removes the cell's override; a missing one is no error.
	ClearOverride(ctx context.Context, date time.Time, slotID int64) error
}

// Availability is everything apply and copy week need from availability.
type Availability interface {
	CellChecker
	NoPlateOverrides
}

// WithAvailability makes Apply and CopyWeek respect "none" cells and lets
// copy week carry "none" overrides. Without it every cell counts as "plan".
func (s *Service) WithAvailability(a Availability) *Service {
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

// Apply materialises a preset's plates onto target_date. See feature.md §6.5.1.
func (s *Service) Apply(ctx context.Context, presetID int64, req ApplyRequest) (*ApplyResult, error) {
	if req.TargetDate.IsZero() {
		return nil, fmt.Errorf("%w: target_date required", domain.ErrInvalidInput)
	}
	conflict, err := ParseConflict(string(req.OnConflict))
	if err != nil {
		return nil, err
	}
	req.OnConflict = conflict

	p, err := s.repo.Get(ctx, presetID)
	if err != nil {
		return nil, err
	}
	plates := filterPresetPlates(p.Plates, req.SlotIDsFilter)
	if len(plates) == 0 {
		return &ApplyResult{}, nil
	}

	activeSlots, err := s.activeSlotIDSet(ctx)
	if err != nil {
		return nil, err
	}
	// Read cell modes before the plate transaction: the availability service
	// uses its own connection, and the pool holds only one.
	unavailable := map[int64]struct{}{}
	for _, pp := range plates {
		none, err := s.isNone(ctx, req.TargetDate, pp.SlotID)
		if err != nil {
			return nil, err
		}
		if none {
			unavailable[pp.SlotID] = struct{}{}
		}
	}

	return s.materialiseOnto(ctx, presetID, req.TargetDate, conflict, plates, activeSlots, unavailable)
}

// isNone reports whether the cell resolves to "none". Without an injected
// Availability no cell does.
func (s *Service) isNone(ctx context.Context, date time.Time, slotID int64) (bool, error) {
	if s.avail == nil {
		return false, nil
	}
	mode, err := s.avail.CellMode(ctx, date, slotID)
	if err != nil {
		return false, fmt.Errorf("cell mode: %w", err)
	}
	return mode == cellNone, nil
}

// CopyWeekRequest copies the plates from [source_start, source_start+6 days]
// onto [target_start, target_start+6 days], preserving slot bindings.
type CopyWeekRequest struct {
	SourceStart time.Time
	TargetStart time.Time
	OnConflict  ConflictMode
}

// CopyWeek implements the past-week-copy flow described in feature.md §6.7.
// It is decoupled from Preset records: source is the planner's history, target
// is the planner. No preset is created or consumed.
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

	// Pull the week's plates outside the transaction so the plate.Service can
	// load them with the right validation surface.
	sourceEnd := req.SourceStart.AddDate(0, 0, 6)
	sources, err := s.plateRange.Range(ctx, req.SourceStart, sourceEnd)
	if err != nil {
		return nil, fmt.Errorf("load source week: %w", err)
	}

	// Reify each source plate as a preset-shaped "plate" so we can reuse the
	// apply pipeline. Each source plate's date is preserved as an offset from
	// the target_start, expressed as a synthetic per-plate target_date.
	activeSlots, err := s.activeSlotIDSet(ctx)
	if err != nil {
		return nil, err
	}

	// Availability reads go before the plate transaction: the availability
	// service uses its own connection, and the pool holds only one.
	noPlate, err := s.sourceNoPlateCells(ctx, req.SourceStart, activeSlots)
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 && len(noPlate) == 0 {
		return &ApplyResult{}, nil
	}
	targetOf := func(d time.Time) time.Time {
		return req.TargetStart.AddDate(0, 0, int(d.Sub(req.SourceStart).Hours()/24))
	}
	unavailable := map[string]bool{}
	for _, src := range sources {
		td := targetOf(src.Date)
		none, err := s.isNone(ctx, td, src.SlotID)
		if err != nil {
			return nil, err
		}
		unavailable[occupancyKey(td, src.SlotID)] = none
	}

	result := &ApplyResult{
		Snapshot: ApplySnapshot{},
	}
	var toMark []noPlateCell
	if err := s.tx.RunInPresetTx(ctx, func(_ Repository, plates plate.Repository) error {
		// Bucket existing plates on the target week to detect occupancy once.
		targetEnd := req.TargetStart.AddDate(0, 0, 6)
		existing, err := plates.ListByDateRange(ctx, req.TargetStart, targetEnd)
		if err != nil {
			return fmt.Errorf("load target week: %w", err)
		}
		occupancy := make(map[string]plate.Plate, len(existing))
		for _, ep := range existing {
			occupancy[occupancyKey(ep.Date, ep.SlotID)] = ep
		}

		// Sort sources by date ascending then slot for deterministic order.
		sort.SliceStable(sources, func(i, j int) bool {
			if !sources[i].Date.Equal(sources[j].Date) {
				return sources[i].Date.Before(sources[j].Date)
			}
			return sources[i].SlotID < sources[j].SlotID
		})

		for _, src := range sources {
			targetDate := targetOf(src.Date)

			if !isSlotActive(activeSlots, src.SlotID) {
				result.SkippedNoSlot = append(result.SkippedNoSlot, SkipItem{Date: targetDate, SlotID: src.SlotID})
				continue
			}

			key := occupancyKey(targetDate, src.SlotID)
			if unavailable[key] {
				result.SkippedUnavailable = append(result.SkippedUnavailable, SkipItem{Date: targetDate, SlotID: src.SlotID})
				continue
			}
			if existingPlate, taken := occupancy[key]; taken {
				if conflict == ConflictSkip {
					result.SkippedOccupied = append(result.SkippedOccupied, SkipItem{Date: targetDate, SlotID: src.SlotID})
					continue
				}
				// Overwrite: snapshot the old plate (load its components) and delete.
				oldFull, err := plates.Get(ctx, existingPlate.ID)
				if err != nil {
					return fmt.Errorf("load plate %d for snapshot: %w", existingPlate.ID, err)
				}
				if err := plates.Delete(ctx, existingPlate.ID); err != nil {
					return fmt.Errorf("delete plate %d: %w", existingPlate.ID, err)
				}
				newPlate, err := materialiseFromPlate(ctx, plates, targetDate, src.SlotID, src.Components)
				if err != nil {
					return err
				}
				result.Replaced = append(result.Replaced, ReplacedItem{NewPlate: *newPlate, OldPlate: *oldFull})
				result.Snapshot.CreatedPlateIDs = append(result.Snapshot.CreatedPlateIDs, newPlate.ID)
				result.Snapshot.ReplacedPlates = append(result.Snapshot.ReplacedPlates, *oldFull)
				occupancy[key] = *newPlate
				continue
			}
			newPlate, err := materialiseFromPlate(ctx, plates, targetDate, src.SlotID, src.Components)
			if err != nil {
				return err
			}
			result.Created = append(result.Created, *newPlate)
			result.Snapshot.CreatedPlateIDs = append(result.Snapshot.CreatedPlateIDs, newPlate.ID)
			occupancy[key] = *newPlate
		}

		for _, c := range noPlate {
			td := targetOf(c.Date)
			if _, taken := occupancy[occupancyKey(td, c.SlotID)]; taken {
				result.SkippedOccupied = append(result.SkippedOccupied, SkipItem{Date: td, SlotID: c.SlotID})
				continue
			}
			toMark = append(toMark, noPlateCell{Date: td, SlotID: c.SlotID, Note: c.Note})
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// The plates are committed, so the result (and its undo snapshot) must
	// reach the caller: copying an override is best-effort. A cell that
	// gained a plate meanwhile is reported as occupied; other failures are
	// logged and the cell is left as it was.
	for _, c := range toMark {
		created, err := s.avail.SetNoneIfUnset(ctx, c.Date, c.SlotID, c.Note)
		if err != nil {
			slog.WarnContext(ctx, "copy_week.override_copy_failed",
				"date", c.Date.Format("2006-01-02"), "slot_id", c.SlotID, "error", err)
			if errors.Is(err, domain.ErrCellHasPlate) {
				result.SkippedOccupied = append(result.SkippedOccupied, SkipItem{Date: c.Date, SlotID: c.SlotID})
			}
			continue
		}
		if created {
			result.Snapshot.CreatedOverrides = append(result.Snapshot.CreatedOverrides, SkipItem{Date: c.Date, SlotID: c.SlotID})
		}
	}
	return result, nil
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

// UndoApply reverses a previous Apply or CopyWeek by deleting the plates it
// created, restoring the plates it replaced and then clearing the overrides
// it created. The plates go first, so a failed plate undo leaves the
// overrides intact; clearing is idempotent, so a retry is safe.
func (s *Service) UndoApply(ctx context.Context, snap ApplySnapshot) error {
	if err := s.undoPlates(ctx, snap); err != nil {
		return err
	}
	if s.avail == nil {
		return nil
	}
	for _, c := range snap.CreatedOverrides {
		if err := s.avail.ClearOverride(ctx, c.Date, c.SlotID); err != nil {
			return fmt.Errorf("clear copied override (date=%s slot=%d): %w", c.Date.Format("2006-01-02"), c.SlotID, err)
		}
	}
	return nil
}

func (s *Service) undoPlates(ctx context.Context, snap ApplySnapshot) error {
	return s.tx.RunInPresetTx(ctx, func(_ Repository, plates plate.Repository) error {
		// Delete the plates that the apply created. Order doesn't matter; FK
		// cascade handles components.
		for _, id := range snap.CreatedPlateIDs {
			if err := plates.Delete(ctx, id); err != nil {
				// Tolerate already-gone plates so undo is idempotent.
				if errIsNotFound(err) {
					continue
				}
				return fmt.Errorf("delete created plate %d: %w", id, err)
			}
		}
		// Recreate the replaced plates with their original components.
		for i := range snap.ReplacedPlates {
			p := snap.ReplacedPlates[i]
			// Drop the original ID so SQLite allocates a fresh one.
			p.ID = 0
			for j := range p.Components {
				p.Components[j].ID = 0
				p.Components[j].PlateID = 0
			}
			if err := plates.Create(ctx, &p); err != nil {
				return fmt.Errorf("restore replaced plate (date=%s slot=%d): %w", p.DateString(), p.SlotID, err)
			}
		}
		return nil
	})
}

// materialiseOnto is the shared apply implementation: for each preset plate
// determine the target slot on target_date, then create/skip/overwrite per
// policy. Runs entirely inside RunInPresetTx so all writes are atomic with
// last_used_at.
func (s *Service) materialiseOnto(
	ctx context.Context,
	presetID int64,
	targetDate time.Time,
	conflict ConflictMode,
	plates []Plate,
	activeSlots map[int64]struct{},
	unavailable map[int64]struct{},
) (*ApplyResult, error) {
	result := &ApplyResult{}
	if err := s.tx.RunInPresetTx(ctx, func(presets Repository, plateRepo plate.Repository) error {
		existing, err := plateRepo.ListByDateRange(ctx, targetDate, targetDate)
		if err != nil {
			return fmt.Errorf("load existing plates: %w", err)
		}

		plan := PlanApply(ApplyInput{
			Plates:           plates,
			TargetDate:       targetDate,
			ExistingPlates:   existing,
			ActiveSlots:      activeSlots,
			UnavailableSlots: unavailable,
			OnConflict:       conflict,
		})

		result.SkippedNoSlot = plan.SkippedNoSlot
		result.SkippedOccupied = plan.SkippedOccupied
		result.SkippedUnavailable = plan.SkippedUnavailable

		// Execute the plan: delete old plates, create new ones.
		for i, entry := range plan.ToCreate {
			deleteID := plan.ToDeleteBeforeCreate[i]
			if deleteID != 0 {
				// Overwrite path: load full plate for snapshot before deleting.
				oldFull, err := plateRepo.Get(ctx, deleteID)
				if err != nil {
					return fmt.Errorf("load plate %d for snapshot: %w", deleteID, err)
				}
				if err := plateRepo.Delete(ctx, deleteID); err != nil {
					return fmt.Errorf("delete plate %d: %w", deleteID, err)
				}
				newPlate, err := materialiseFromPresetComponents(ctx, plateRepo, targetDate, entry.SlotID, entry.Components)
				if err != nil {
					return err
				}
				result.Replaced = append(result.Replaced, ReplacedItem{NewPlate: *newPlate, OldPlate: *oldFull})
				result.Snapshot.CreatedPlateIDs = append(result.Snapshot.CreatedPlateIDs, newPlate.ID)
				result.Snapshot.ReplacedPlates = append(result.Snapshot.ReplacedPlates, *oldFull)
				continue
			}
			newPlate, err := materialiseFromPresetComponents(ctx, plateRepo, targetDate, entry.SlotID, entry.Components)
			if err != nil {
				return err
			}
			result.Created = append(result.Created, *newPlate)
			result.Snapshot.CreatedPlateIDs = append(result.Snapshot.CreatedPlateIDs, newPlate.ID)
		}

		// Only bump last_used_at if at least one plate was created or replaced.
		if len(result.Created) > 0 || len(result.Replaced) > 0 {
			if err := presets.TouchLastUsed(ctx, presetID); err != nil {
				return fmt.Errorf("touch last_used_at: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
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

func materialiseFromPresetComponents(
	ctx context.Context,
	plates plate.Repository,
	date time.Time,
	slotID int64,
	source []Component,
) (*plate.Plate, error) {
	comps := make([]plate.PlateComponent, len(source))
	for i, c := range source {
		comps[i] = plate.PlateComponent{
			FoodID:      c.FoodID,
			Portions:    c.Portions,
			Amount:      c.Amount,
			Unit:        c.Unit,
			Grams:       c.Grams,
			GramsSource: c.GramsSource,
			SortOrder:   i,
		}
	}
	p := &plate.Plate{Date: date, SlotID: slotID, Components: comps}
	if err := plates.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create plate (date=%s slot=%d): %w", date.Format("2006-01-02"), slotID, err)
	}
	return p, nil
}

func materialiseFromPlate(
	ctx context.Context,
	plates plate.Repository,
	date time.Time,
	slotID int64,
	source []plate.PlateComponent,
) (*plate.Plate, error) {
	comps := make([]plate.PlateComponent, len(source))
	for i, c := range source {
		comps[i] = plate.PlateComponent{
			FoodID:      c.FoodID,
			Portions:    c.Portions,
			Amount:      c.Amount,
			Unit:        c.Unit,
			Grams:       c.Grams,
			GramsSource: c.GramsSource,
			SortOrder:   i,
		}
	}
	p := &plate.Plate{Date: date, SlotID: slotID, Components: comps}
	if err := plates.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create plate (date=%s slot=%d): %w", date.Format("2006-01-02"), slotID, err)
	}
	return p, nil
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

func occupancyKey(date time.Time, slotID int64) string {
	return date.Format("2006-01-02") + "/" + strconv.FormatInt(slotID, 10)
}

func errIsNotFound(err error) bool {
	return errors.Is(err, domain.ErrNotFound)
}
