package plate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// Quantity-validation sentinel errors. All wrap domain.ErrInvalidInput so the
// HTTP error map turns them into 400s; specific keys are surfaced by the
// handler error map for i18n.
var (
	// ErrInvalidQuantityShape is returned when a request has both portions
	// and (amount + unit), or a leaf component has neither.
	ErrInvalidQuantityShape = errors.New("plate component must specify exactly one of portions or (amount, unit)")
	// ErrInvalidQuantityForComposed is returned when a composed-food component
	// carries amount/unit instead of portions.
	ErrInvalidQuantityForComposed = errors.New("composed food on a plate requires portions, not amount/unit")
	// ErrInvalidQuantityForLeaf is returned when a leaf-food component carries
	// portions instead of (amount + unit).
	ErrInvalidQuantityForLeaf = errors.New("leaf food on a plate requires amount + unit, not portions")
	// ErrUnitRequiresPortion is returned when a leaf component's unit is a
	// count unit (e.g., "slice") that the food has no portion override for and
	// no universal mass/volume default exists.
	ErrUnitRequiresPortion = errors.New("unit requires a per-food portion override")
)

// quantityKeys maps the quantity sentinels to their i18n message keys.
var quantityKeys = []struct {
	err error
	key string
}{
	{ErrInvalidQuantityShape, "error.plate.invalid_quantity_shape"},
	{ErrInvalidQuantityForComposed, "error.plate.invalid_quantity_for_composed"},
	{ErrInvalidQuantityForLeaf, "error.plate.invalid_quantity_for_leaf"},
	{ErrUnitRequiresPortion, "error.plate.unit_requires_portion"},
}

// MessageKey returns the i18n message key of the quantity sentinel err wraps.
// Check it before domain.MessageKey: these sentinels travel with
// domain.ErrInvalidInput.
func MessageKey(err error) (string, bool) {
	for _, q := range quantityKeys {
		if errors.Is(err, q.err) {
			return q.key, true
		}
	}
	return "", false
}

// Limits.
const (
	// MaxEaters caps Plate.Eaters (it matches profile.MaxHouseholdPortions).
	MaxEaters float64 = 20
	// MaxSessionNoteLen caps a session note, in characters.
	MaxSessionNoteLen = 200
	// MaxLeftoverCells caps LeftoverCells' count and AddLeftovers' cells.
	MaxLeftoverCells = 14
	// maxRangeDays caps Range and ClearRange.
	maxRangeDays = 366
)

// Slots is the time-slot surface the service needs: existence for plate
// writes and the sort order for cell order (D12).
type Slots interface {
	Exists(ctx context.Context, slotID int64) (bool, error)
	List(ctx context.Context, activeOnly bool) ([]slot.TimeSlot, error)
}

// Cell modes returned by CellChecker.
const (
	CellPlan    = "plan"
	CellRoutine = "routine"
	CellNone    = "none"
)

// CellChecker resolves a cell's availability mode: CellPlan, CellRoutine or
// CellNone. The availability service satisfies it.
type CellChecker interface {
	CellMode(ctx context.Context, date time.Time, slotID int64) (string, error)
}

// FoodLookup is the food-side surface the plate service depends on: kind,
// keep days and freezable, plus portion overrides for grams resolution.
type FoodLookup interface {
	Get(ctx context.Context, foodID int64) (*food.Food, error)
	ListPortions(ctx context.Context, foodID int64) ([]food.Portion, error)
}

// ProfileReader supplies the default eaters of a new plate (D10).
type ProfileReader interface {
	HouseholdPortions(ctx context.Context) (float64, error)
}

// SettingsReader resolves app settings; the batch rules read the plan.*
// keep-days and freezer keys.
type SettingsReader interface {
	Get(ctx context.Context, key string) (settings.Value, error)
}

// Deps are the service's dependencies. Cells is optional: without it every
// cell counts as "plan". Now defaults to time.Now. Outcomes records the AI
// runs' outcomes on accept, discard, unaccept and restore (D25).
type Deps struct {
	Plates   Repository
	Sessions SessionRepository
	Slots    Slots
	Foods    FoodLookup
	Cells    CellChecker
	Profile  ProfileReader
	Settings SettingsReader
	Events   EventLog
	Outcomes RunOutcomes
	Tx       domain.Transactor
	Now      func() time.Time
}

// Service is the single write path for plates, components and cook sessions
// (Phase 2 §2.1). Every mutation runs in one unit of work.
type Service struct {
	plates   Repository
	sessions SessionRepository
	slots    Slots
	foods    FoodLookup
	cells    CellChecker
	profile  ProfileReader
	settings SettingsReader
	events   EventLog
	outcomes RunOutcomes
	tx       domain.Transactor
	now      func() time.Time
}

// NewService creates a plate service.
func NewService(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		plates: d.Plates, sessions: d.Sessions, slots: d.Slots, foods: d.Foods,
		cells: d.Cells, profile: d.Profile, settings: d.Settings, events: d.Events, outcomes: d.Outcomes,
		tx: d.Tx, now: now,
	}
}

// ── Plates ──────────────────────────────────────────────────────────────

// PlateInput creates a plate. Eaters nil means the household's portions.
// A composed component without CookSessionID gets a fresh session at the
// plate's cell (D8); with one, it is validated against that session (D13).
type PlateInput struct {
	Date       time.Time
	SlotID     int64
	Eaters     *float64
	Note       *string
	Components []PlateComponent
}

// Create validates and persists a new plate with its components.
func (s *Service) Create(ctx context.Context, in PlateInput) (*Plate, error) {
	comps := make([]PlateComponent, len(in.Components))
	copy(comps, in.Components)
	for i := range comps {
		comps[i].SortOrder = i
	}
	in.Components = comps
	var out *Plate
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		j := newJournal()
		t := newTracker(j)
		id, err := s.createPlate(ctx, in, e, t, true, newOrigin(ctx), true)
		if err != nil {
			return err
		}
		j.create(id)
		if err := s.settle(ctx, t); err != nil {
			return err
		}
		if err := s.commit(ctx, j); err != nil {
			return err
		}
		out, err = s.plates.Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// createPlate writes one plate with components whose SortOrder is set. strict
// enforces the D13 cell rules on linked sessions (Apply restores state as it
// was and only checks the food). The plate gets o's state, run and reason;
// with adopt it takes the state of the existing sessions it links to instead.
// Where states meet (linked sessions of both states, or a restored plate's
// own state against its sessions'), confirmed wins (D3) and the merged
// closure is unified at settle. Its fresh sessions take its state.
func (s *Service) createPlate(ctx context.Context, in PlateInput, e env, t *tracker, strict bool, o origin, adopt bool) (int64, error) {
	if in.Date.IsZero() {
		return 0, fmt.Errorf("%w: date required", domain.ErrInvalidInput)
	}
	if err := s.validateSlot(ctx, in.SlotID); err != nil {
		return 0, err
	}
	eaters, err := s.resolveEaters(ctx, in.Eaters)
	if err != nil {
		return 0, err
	}
	p := &Plate{Date: dateOnly(in.Date), SlotID: in.SlotID, Eaters: eaters, Note: in.Note}
	if err := s.checkCell(ctx, p.Cell(), 0); err != nil {
		return 0, err
	}
	p.Components = make([]PlateComponent, len(in.Components))
	copy(p.Components, in.Components)
	foods := make([]*food.Food, len(p.Components))
	linkedStates := map[State]bool{}
	for i := range p.Components {
		pc := &p.Components[i]
		f, err := s.normaliseComponent(ctx, pc)
		if err != nil {
			return 0, fmt.Errorf("component[%d]: %w", i, err)
		}
		foods[i] = f
		if pc.CookSessionID == nil {
			continue
		}
		sess, err := s.attach(ctx, t, pc, f, p.Cell(), eaters, e, strict, origin{})
		if err != nil {
			return 0, fmt.Errorf("component[%d]: %w", i, err)
		}
		if sess != nil {
			linkedStates[sess.State] = true
		}
	}
	// The states that meet in the plate's closure: its linked sessions', and
	// the plate's own when it keeps a given one (restore).
	met := maps.Clone(linkedStates)
	if !adopt && len(met) > 0 {
		met[o.state] = true
	}
	if adopt && len(met) > 0 || len(met) > 1 {
		o.state = StateDraft
		if met[StateConfirmed] {
			o.state = StateConfirmed
		}
	}
	fresh := freshOrigin(ctx, o.state)
	for i := range p.Components {
		pc := &p.Components[i]
		if pc.CookSessionID != nil {
			continue
		}
		if _, err := s.attach(ctx, t, pc, foods[i], p.Cell(), eaters, e, strict, fresh); err != nil {
			return 0, fmt.Errorf("component[%d]: %w", i, err)
		}
	}
	p.State, p.AIRunID, p.Reason = o.state, o.runID, o.reason
	if err := s.plates.Create(ctx, p); err != nil {
		return 0, err
	}
	if len(met) > 1 {
		t.unify = append(t.unify, p.ID)
	}
	return p.ID, nil
}

// Get returns a plate with its components and feedback.
func (s *Service) Get(ctx context.Context, id int64) (*Plate, error) {
	return s.plates.Get(ctx, id)
}

// PlatePatch changes a plate. Nil fields are kept; SetNote writes Note (nil
// clears it).
type PlatePatch struct {
	Date    *time.Time
	SlotID  *int64
	Eaters  *float64
	Note    *string
	SetNote bool
}

// Update changes date, slot, eaters and note. A move must land in an empty
// cell that is not "none"; it never fails on batch rules and moves the cook
// cell of every session whose fresh component is on the plate (D13).
// Changing eaters runs D11 on every session linked to the plate.
func (s *Service) Update(ctx context.Context, id int64, patch PlatePatch) (*Plate, error) {
	var out *Plate
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		cur, err := s.plates.Get(ctx, id)
		if err != nil {
			return err
		}
		next := *cur
		if patch.Date != nil {
			if patch.Date.IsZero() {
				return fmt.Errorf("%w: date required", domain.ErrInvalidInput)
			}
			next.Date = dateOnly(*patch.Date)
		}
		if patch.SlotID != nil {
			if err := s.validateSlot(ctx, *patch.SlotID); err != nil {
				return err
			}
			next.SlotID = *patch.SlotID
		}
		if patch.Eaters != nil {
			if err := validateEaters(*patch.Eaters); err != nil {
				return err
			}
			next.Eaters = *patch.Eaters
		}
		if patch.SetNote {
			next.Note = patch.Note
		}
		moved := !next.Cell().Equal(cur.Cell())
		if moved {
			if err := s.checkCell(ctx, next.Cell(), id); err != nil {
				return err
			}
		}
		j := newJournal()
		j.touch(*cur)
		t := newTracker(j)
		sessionIDs := linkedSessions(cur.Components)
		for _, sid := range sessionIDs {
			if err := s.track(ctx, t, sid); err != nil {
				return err
			}
		}
		if err := s.plates.Update(ctx, &next); err != nil {
			return err
		}
		if moved {
			for _, sid := range sessionIDs {
				sess, err := s.sessions.GetSession(ctx, sid)
				if err != nil {
					return err
				}
				if !sess.CookCell().Equal(cur.Cell()) {
					continue
				}
				j.touchSession(*sess)
				sess.CookDate, sess.CookSlotID = next.Date, next.SlotID
				if err := s.sessions.UpdateSession(ctx, sess); err != nil {
					return err
				}
			}
		}
		if err := s.settle(ctx, t); err != nil {
			return err
		}
		if err := s.commit(ctx, j); err != nil {
			return err
		}
		out, err = s.plates.Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes a plate and returns what restores it. Sessions left without
// links are deleted; the others follow D11.
func (s *Service) Delete(ctx context.Context, id int64) (PlanSnapshot, error) {
	var snap PlanSnapshot
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		p, err := s.plates.Get(ctx, id)
		if err != nil {
			return err
		}
		j := newJournal()
		snap, err = s.remove(ctx, []Plate{*p}, allComponents, false, &refAlloc{}, nil, nil, j)
		if err != nil {
			return err
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return PlanSnapshot{}, err
	}
	return snap, nil
}

// ClearRange removes every plate in [from, to] (only slotID's plates when
// slotID is not 0) and the sessions left without links, and returns what
// restores them.
func (s *Service) ClearRange(ctx context.Context, from, to time.Time, slotID int64) (PlanSnapshot, error) {
	if err := validateRange(from, to); err != nil {
		return PlanSnapshot{}, err
	}
	var snap PlanSnapshot
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		plates, err := s.plates.ListByDateRange(ctx, dateOnly(from), dateOnly(to))
		if err != nil {
			return err
		}
		if slotID != 0 {
			plates = slices.DeleteFunc(plates, func(p Plate) bool { return p.SlotID != slotID })
		}
		j := newJournal()
		snap, err = s.remove(ctx, plates, allComponents, false, &refAlloc{}, nil, nil, j)
		if err != nil {
			return err
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return PlanSnapshot{}, err
	}
	return snap, nil
}

// Removal is the mid-week re-plan's delete (spec §3.5): the plates of
// PlateIDs go whole; every component eating from a session of Drop goes too,
// wherever its plate is, and a plate left without components goes with it.
// Each session in Keep ends with exactly its portions and frozen portions
// and survives even without links; each session in Move cooks at its cell.
type Removal struct {
	PlateIDs []int64
	Drop     []int64
	Keep     map[int64]SessionPortions
	Move     map[int64]CellKey
}

// RemovePlates applies r in one unit of work and returns what restores it.
// Sessions outside Keep follow D11, so a dropped session (all its links gone)
// is deleted and a cooked batch, kept, never is. A missing plate or dropped
// session is ErrNotFound and nothing is removed.
func (s *Service) RemovePlates(ctx context.Context, r Removal) (PlanSnapshot, error) {
	ids := uniqueIDs(r.PlateIDs)
	var snap PlanSnapshot
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		plates, err := s.plates.ListByIDs(ctx, ids)
		if err != nil {
			return err
		}
		if len(plates) != len(ids) {
			return fmt.Errorf("%w: a plate is gone", domain.ErrNotFound)
		}
		whole := make(map[int64]bool, len(ids))
		for _, id := range ids {
			whole[id] = true
		}
		dropped := map[int64]bool{}
		var more []int64
		for _, sid := range uniqueIDs(r.Drop) {
			if _, err := s.sessions.GetSession(ctx, sid); err != nil {
				return err
			}
			links, err := s.sessions.ListLinks(ctx, sid)
			if err != nil {
				return err
			}
			for _, l := range links {
				dropped[l.ComponentID] = true
				if !whole[l.PlateID] && !slices.Contains(more, l.PlateID) {
					more = append(more, l.PlateID)
				}
			}
		}
		if len(more) > 0 {
			extra, err := s.plates.ListByIDs(ctx, more)
			if err != nil {
				return err
			}
			plates = append(plates, extra...)
		}
		order, err := s.slotOrder(ctx)
		if err != nil {
			return err
		}
		sortPlates(plates, order)
		pinned := make(map[int64]bool, len(r.Keep))
		for id := range r.Keep {
			pinned[id] = true
		}
		pick := func(c PlateComponent) bool { return whole[c.PlateID] || dropped[c.ID] }
		j := newJournal()
		t := newTracker(j)
		if snap, err = s.remove(ctx, plates, pick, true, &refAlloc{}, t, pinned, j); err != nil {
			return err
		}
		t.ids = slices.DeleteFunc(t.ids, func(id int64) bool { return pinned[id] })
		if err := s.settle(ctx, t); err != nil {
			return err
		}
		for id, p := range r.Keep {
			sess, err := s.sessions.GetSession(ctx, id)
			if err != nil {
				return err
			}
			if sess.Portions == p.Portions && sess.FrozenPortions == p.FrozenPortions {
				continue
			}
			j.touchSession(*sess)
			sess.Portions, sess.FrozenPortions = p.Portions, p.FrozenPortions
			if err := s.sessions.UpdateSession(ctx, sess); err != nil {
				return err
			}
		}
		for _, id := range slices.Sorted(maps.Keys(r.Move)) {
			sess, err := s.sessions.GetSession(ctx, id)
			if err != nil {
				return err
			}
			c := r.Move[id]
			if sess.CookCell().Equal(c) {
				continue
			}
			j.touchSession(*sess)
			sess.CookDate, sess.CookSlotID = c.Date, c.SlotID
			if err := s.sessions.UpdateSession(ctx, sess); err != nil {
				return err
			}
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return PlanSnapshot{}, err
	}
	return snap, nil
}

// Range returns all plates in [from, to] inclusive. from must be ≤ to; span
// must be ≤ 366 days.
func (s *Service) Range(ctx context.Context, from, to time.Time) ([]Plate, error) {
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	return s.plates.ListByDateRange(ctx, from, to)
}

// ── Components ──────────────────────────────────────────────────────────

// AddComponent appends a food to a plate at the next sort_order and returns
// the plate. Its quantity (portions xor amount+unit) is validated against the
// food's kind; a composed component gets a fresh session unless it names one
// to link to.
func (s *Service) AddComponent(ctx context.Context, plateID int64, pc PlateComponent) (*Plate, error) {
	var out *Plate
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		p, err := s.plates.Get(ctx, plateID)
		if err != nil {
			return err
		}
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		t := newTracker(newJournal())
		f, err := s.normaliseComponent(ctx, &pc)
		if err != nil {
			return err
		}
		linked, err := s.attach(ctx, t, &pc, f, p.Cell(), p.Eaters, e, true, freshOrigin(ctx, p.State))
		if err != nil {
			return err
		}
		if _, err := s.joinLink(ctx, t, p.ID, p.State, linked); err != nil {
			return err
		}
		pc.PlateID = plateID
		pc.SortOrder = nextSortOrder(p.Components)
		if err := s.plates.CreateComponent(ctx, &pc); err != nil {
			return err
		}
		out, err = s.finishComponentChange(ctx, t, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetComponent returns the stored plate_component row by id.
func (s *Service) GetComponent(ctx context.Context, componentID int64) (*PlateComponent, error) {
	return s.plates.GetComponent(ctx, componentID)
}

// UpdateComponentQuantity changes the quantity (portions or amount+unit) of a
// component and returns its plate. The session link is kept and follows D11.
func (s *Service) UpdateComponentQuantity(ctx context.Context, componentID int64, q PlateComponent) (*Plate, error) {
	var out *Plate
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		pc, err := s.plates.GetComponent(ctx, componentID)
		if err != nil {
			return err
		}
		t := newTracker(newJournal())
		if pc.CookSessionID != nil {
			if err := s.track(ctx, t, *pc.CookSessionID); err != nil {
				return err
			}
		}
		pc.Portions, pc.Amount, pc.Unit, pc.Grams, pc.GramsSource = q.Portions, q.Amount, q.Unit, q.Grams, nil
		if _, err := s.normaliseComponent(ctx, pc); err != nil {
			return err
		}
		p, err := s.plates.Get(ctx, pc.PlateID)
		if err != nil {
			return err
		}
		if err := s.plates.UpdateComponent(ctx, pc); err != nil {
			return err
		}
		out, err = s.finishComponentChange(ctx, t, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SwapComponent replaces the food of a component, keeping its sort_order, and
// returns its plate. quantity nil reuses the current quantity, which must fit
// the new food's kind. A composed new food gets a fresh session; the old
// session is deleted when orphaned and otherwise follows D11.
func (s *Service) SwapComponent(ctx context.Context, componentID, newFoodID int64, quantity *PlateComponent) (*Plate, error) {
	var out *Plate
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		pc, err := s.plates.GetComponent(ctx, componentID)
		if err != nil {
			return err
		}
		p, err := s.plates.Get(ctx, pc.PlateID)
		if err != nil {
			return err
		}
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		t := newTracker(newJournal())
		if pc.CookSessionID != nil {
			if err := s.track(ctx, t, *pc.CookSessionID); err != nil {
				return err
			}
		}
		pc.FoodID = newFoodID
		pc.CookSessionID = nil
		if quantity != nil {
			pc.Portions, pc.Amount, pc.Unit, pc.Grams = quantity.Portions, quantity.Amount, quantity.Unit, quantity.Grams
		}
		pc.GramsSource = nil
		f, err := s.normaliseComponent(ctx, pc)
		if err != nil {
			return err
		}
		if _, err := s.attach(ctx, t, pc, f, p.Cell(), p.Eaters, e, true, freshOrigin(ctx, p.State)); err != nil {
			return err
		}
		if err := s.plates.UpdateComponent(ctx, pc); err != nil {
			return err
		}
		out, err = s.finishComponentChange(ctx, t, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RemoveComponent removes a component and returns its plate. Its session is
// deleted when orphaned and otherwise follows D11.
func (s *Service) RemoveComponent(ctx context.Context, componentID int64) (*Plate, error) {
	var out *Plate
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		pc, err := s.plates.GetComponent(ctx, componentID)
		if err != nil {
			return err
		}
		p, err := s.plates.Get(ctx, pc.PlateID)
		if err != nil {
			return err
		}
		t := newTracker(newJournal())
		if pc.CookSessionID != nil {
			if err := s.track(ctx, t, *pc.CookSessionID); err != nil {
				return err
			}
		}
		if err := s.plates.DeleteComponent(ctx, componentID); err != nil {
			return err
		}
		out, err = s.finishComponentChange(ctx, t, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// finishComponentChange settles D11, bumps the plate and logs its event;
// before is the plate as it was before the change.
func (s *Service) finishComponentChange(ctx context.Context, t *tracker, before *Plate) (*Plate, error) {
	if err := s.settle(ctx, t); err != nil {
		return nil, err
	}
	if err := s.plates.Touch(ctx, before.ID); err != nil {
		return nil, err
	}
	t.j.touch(*before)
	if err := s.commit(ctx, t.j); err != nil {
		return nil, err
	}
	return s.plates.Get(ctx, before.ID)
}

// ── Sessions ────────────────────────────────────────────────────────────

// CookSession returns the plain session, without the view's links,
// consumption and warnings.
func (s *Service) CookSession(ctx context.Context, id int64) (*CookSession, error) {
	return s.sessions.GetSession(ctx, id)
}

// Session returns a session with its links, consumption and warnings.
func (s *Service) Session(ctx context.Context, id int64) (*SessionView, error) {
	sess, err := s.sessions.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	e, err := s.loadEnv(ctx)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, sess, e)
}

func (s *Service) view(ctx context.Context, sess *CookSession, e env) (*SessionView, error) {
	f, err := s.foods.Get(ctx, sess.FoodID)
	if err != nil {
		return nil, err
	}
	links, err := s.sessions.ListLinks(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	v := BuildView(*sess, f.Name, e.set.KeepDays(f.KeepDays), links, e.set, e.order)
	return &v, nil
}

// SessionPortions is an explicit session edit. It always wins over D11.
type SessionPortions struct {
	Portions       float64
	FrozenPortions float64
	Note           *string
}

// SetSessionPortions sets portions, frozen portions and note. Frozen portions
// need plan.freezing_enabled and a recipe not marked unfreezable (D14).
func (s *Service) SetSessionPortions(ctx context.Context, id int64, in SessionPortions) (*SessionView, error) {
	if !(in.Portions > 0) {
		return nil, fmt.Errorf("%w: portions must be positive", domain.ErrInvalidInput)
	}
	if in.FrozenPortions < 0 || in.FrozenPortions > in.Portions {
		return nil, fmt.Errorf("%w: frozen_portions must be between 0 and portions", domain.ErrInvalidInput)
	}
	note, err := normaliseNote(in.Note, MaxSessionNoteLen)
	if err != nil {
		return nil, err
	}
	var out *SessionView
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.GetSession(ctx, id)
		if err != nil {
			return err
		}
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		if in.FrozenPortions > 0 {
			if !e.set.FreezingEnabled {
				return fmt.Errorf("%w: session %d", domain.ErrSessionFreezingDisabled, id)
			}
			f, err := s.foods.Get(ctx, sess.FoodID)
			if err != nil {
				return err
			}
			if f.Freezable != nil && !*f.Freezable {
				return fmt.Errorf("%w: food %d", domain.ErrSessionNotFreezable, f.ID)
			}
		}
		j, err := s.sessionJournal(ctx, *sess)
		if err != nil {
			return err
		}
		sess.Portions, sess.FrozenPortions, sess.Note = in.Portions, in.FrozenPortions, note
		if err := s.sessions.UpdateSession(ctx, sess); err != nil {
			return err
		}
		if err := s.commit(ctx, j); err != nil {
			return err
		}
		out, err = s.view(ctx, sess, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetCooked marks a session cooked now, or not cooked.
func (s *Service) SetCooked(ctx context.Context, id int64, cooked bool) (*SessionView, error) {
	var out *SessionView
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.GetSession(ctx, id)
		if err != nil {
			return err
		}
		j, err := s.sessionJournal(ctx, *sess)
		if err != nil {
			return err
		}
		sess.CookedAt = nil
		if cooked {
			at := s.now().UTC().Truncate(time.Second)
			sess.CookedAt = &at
		}
		if err := s.sessions.UpdateSession(ctx, sess); err != nil {
			return err
		}
		if err := s.commit(ctx, j); err != nil {
			return err
		}
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		out, err = s.view(ctx, sess, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LeftoverCells returns the next count cells, in cell order, a leftover of the
// session may go to (D29): after the cook cell, on or before use-by, mode
// "plan" and without a plate.
func (s *Service) LeftoverCells(ctx context.Context, id int64, count int) ([]CellKey, error) {
	if count < 1 || count > MaxLeftoverCells {
		return nil, fmt.Errorf("%w: count must be between 1 and %d", domain.ErrInvalidInput, MaxLeftoverCells)
	}
	sess, err := s.sessions.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	f, err := s.foods.Get(ctx, sess.FoodID)
	if err != nil {
		return nil, err
	}
	e, err := s.loadEnv(ctx)
	if err != nil {
		return nil, err
	}
	useBy := UseBy(*sess, e.set.KeepDays(f.KeepDays))
	active, err := s.slots.List(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list slots: %w", err)
	}
	plates, err := s.plates.ListByDateRange(ctx, sess.CookDate, useBy)
	if err != nil {
		return nil, err
	}
	occupied := make(map[CellKey]bool, len(plates))
	for _, p := range plates {
		occupied[p.Cell()] = true
	}
	out := []CellKey{}
	for d := sess.CookDate; !d.After(useBy); d = d.AddDate(0, 0, 1) {
		for _, sl := range active {
			c := CellKey{Date: d, SlotID: sl.ID}
			if !Before(sess.CookCell(), c, e.order) || occupied[c] {
				continue
			}
			mode, err := s.cellMode(ctx, c)
			if err != nil {
				return nil, err
			}
			if mode != CellPlan {
				continue
			}
			out = append(out, c)
			if len(out) == count {
				return out, nil
			}
		}
	}
	return out, nil
}

// AddLeftovers creates one plate per cell, each with the household's eaters
// and one component linked to the session (same food, same per-eater portions
// as its first link). The session grows by D11 (D29).
func (s *Service) AddLeftovers(ctx context.Context, id int64, cells []CellKey) (*SessionView, error) {
	if len(cells) == 0 || len(cells) > MaxLeftoverCells {
		return nil, fmt.Errorf("%w: cells must hold 1 to %d cells", domain.ErrInvalidInput, MaxLeftoverCells)
	}
	var out *SessionView
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.GetSession(ctx, id)
		if err != nil {
			return err
		}
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		links, err := s.sessions.ListLinks(ctx, id)
		if err != nil {
			return err
		}
		perEater := 1.0
		if ordered := OrderLinks(*sess, links, e.set.DefaultKeepDays, e.order); len(ordered) > 0 {
			perEater = ordered[0].PerEater
		}
		j := newJournal()
		j.editSession(*sess)
		t := newTracker(j)
		for _, c := range cells {
			portions := perEater
			sid := id
			in := PlateInput{
				Date: c.Date, SlotID: c.SlotID,
				Components: []PlateComponent{{FoodID: sess.FoodID, Portions: &portions, CookSessionID: &sid}},
			}
			pid, err := s.createPlate(ctx, in, e, t, true, newOrigin(ctx), true)
			if err != nil {
				return fmt.Errorf("leftover %s slot %d: %w", c.Date.Format(dateLayout), c.SlotID, err)
			}
			j.create(pid)
		}
		if err := s.settle(ctx, t); err != nil {
			return err
		}
		if err := s.commit(ctx, j); err != nil {
			return err
		}
		sess, err = s.sessions.GetSession(ctx, id)
		if err != nil {
			return err
		}
		out, err = s.view(ctx, sess, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteSession removes every component linked to the session, deletes the
// plates left without components and the session, and returns what restores
// them ("Remove batch").
func (s *Service) DeleteSession(ctx context.Context, id int64) (PlanSnapshot, error) {
	var snap PlanSnapshot
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.GetSession(ctx, id)
		if err != nil {
			return err
		}
		j := newJournal()
		j.editSession(*sess)
		links, err := s.sessions.ListLinks(ctx, id)
		if err != nil {
			return err
		}
		if len(links) == 0 {
			snap = emptySnapshot()
			if err := s.sessions.DeleteSession(ctx, id); err != nil {
				return err
			}
			return s.commit(ctx, j)
		}
		var plates []Plate
		seen := map[int64]bool{}
		for _, l := range links {
			if seen[l.PlateID] {
				continue
			}
			seen[l.PlateID] = true
			p, err := s.plates.Get(ctx, l.PlateID)
			if err != nil {
				return err
			}
			plates = append(plates, *p)
		}
		order, err := s.slotOrder(ctx)
		if err != nil {
			return err
		}
		sortPlates(plates, order)
		pick := func(pc PlateComponent) bool { return pc.CookSessionID != nil && *pc.CookSessionID == id }
		snap, err = s.remove(ctx, plates, pick, true, &refAlloc{}, nil, nil, j)
		if err != nil {
			return err
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return PlanSnapshot{}, err
	}
	return snap, nil
}

// sessionJournal starts the journal of an edit of sess: the session and
// every plate linked to it.
func (s *Service) sessionJournal(ctx context.Context, sess CookSession) (*journal, error) {
	j := newJournal()
	j.editSession(sess)
	links, err := s.sessions.ListLinks(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if _, seen := j.plates[l.PlateID]; seen {
			continue
		}
		p, err := s.plates.Get(ctx, l.PlateID)
		if err != nil {
			return nil, err
		}
		j.touch(*p)
	}
	return j, nil
}

// ── Snapshot apply ──────────────────────────────────────────────────────

// Apply writes a snapshot (D22): the snapshot's sessions by ref, then plates
// and components. A composed component links to {ref}, to {id} (a session
// that must still exist, else ErrSessionNotFound and nothing is written) or,
// without a link, to a new fresh session. Every re-linked existing session
// follows D11. A "none" cell or an occupied one is handled per OnConflict.
//
// Apply decides every plate's placement before it writes anything. The plates
// ConflictOverwrite replaces are then removed together, so a session they
// share is recorded once; only the {id} sessions of plates Apply writes count
// as surviving that removal (they are linked again before the final settle).
func (s *Service) Apply(ctx context.Context, snap PlanSnapshot, opts ApplyOptions) (*ApplyResult, error) {
	switch opts.OnConflict {
	case ConflictFail, ConflictSkip, ConflictOverwrite:
	default:
		return nil, fmt.Errorf("%w: on_conflict must be fail, skip or overwrite", domain.ErrInvalidInput)
	}
	sessByRef, cells, err := validateSnapshot(snap)
	if err != nil {
		return nil, err
	}
	res := &ApplyResult{PlateIDs: []int64{}, SkippedOccupied: []CellKey{}, SkippedUnavailable: []CellKey{}, Removed: emptySnapshot()}
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		steps, replace, err := s.placeSnapshot(ctx, snap, cells, opts.OnConflict, res)
		if err != nil {
			return err
		}
		j := newJournal()
		if opts.Restore {
			j.created = ActionRestore
		}
		t := newTracker(j)
		pinned := map[int64]bool{}
		for _, st := range steps {
			for _, c := range st.sp.Components {
				if c.Session == nil || c.Session.ID == 0 || pinned[c.Session.ID] {
					continue
				}
				pinned[c.Session.ID] = true
				if _, err := s.sessions.GetSession(ctx, c.Session.ID); err != nil {
					if errors.Is(err, domain.ErrNotFound) {
						return fmt.Errorf("%w: session %d", domain.ErrSessionNotFound, c.Session.ID)
					}
					return err
				}
				if err := s.track(ctx, t, c.Session.ID); err != nil {
					return err
				}
			}
		}
		if len(replace) > 0 {
			// The overwrite shares Apply's tracker and keeps the pinned
			// sessions: Apply settles D11 and GC once, after relinking.
			res.Removed, err = s.remove(ctx, replace, allComponents, false, &refAlloc{}, t, pinned, j)
			if err != nil {
				return err
			}
		}
		created := map[string]int64{}
		for _, st := range steps {
			if st.merge != nil {
				j.touch(*st.merge)
				if err := s.mergeInto(ctx, st.merge, st.sp, sessByRef, created, e, t, opts.Restore); err != nil {
					return err
				}
				res.PlateIDs = append(res.PlateIDs, st.merge.ID)
				continue
			}
			comps, err := s.snapshotComponents(ctx, j, st.sp.Components, sessByRef, created, opts.Restore)
			if err != nil {
				return err
			}
			var eaters *float64
			if st.sp.Eaters > 0 {
				eaters = &st.sp.Eaters
			}
			in := PlateInput{Date: st.cell.Date, SlotID: st.cell.SlotID, Eaters: eaters, Note: st.sp.Note, Components: comps}
			o := snapshotOrigin(ctx, st.sp.State, st.sp.AIRunID, st.sp.Reason, opts.Restore)
			id, err := s.createPlate(ctx, in, e, t, false, o, !opts.Restore)
			if err != nil {
				return err
			}
			if fb := st.sp.Feedback; fb != nil {
				if err := s.plates.RestoreFeedback(ctx, id, Feedback{Status: fb.Status, Note: fb.Note, RatedAt: fb.RatedAt}); err != nil {
					return err
				}
			}
			j.create(id)
			res.PlateIDs = append(res.PlateIDs, id)
		}
		if err := s.settle(ctx, t); err != nil {
			return err
		}
		if opts.Restore {
			if err := s.recordRestored(ctx, snap); err != nil {
				return err
			}
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// recordRestored marks the runs of the drafts a restore brings back (D25):
// the Undo of a discard reopens its run.
func (s *Service) recordRestored(ctx context.Context, snap PlanSnapshot) error {
	seen := map[int64]bool{}
	note := func(st State, run *int64) {
		if st == StateDraft && run != nil {
			seen[*run] = true
		}
	}
	for _, sp := range snap.Plates {
		note(sp.State, sp.AIRunID)
	}
	for _, ss := range snap.Sessions {
		note(ss.State, ss.AIRunID)
	}
	for _, run := range sortedKeys(seen) {
		if err := s.outcomes.Record(ctx, run, OutcomeRestored); err != nil {
			return err
		}
	}
	return nil
}

// placement is where Apply writes one snapshot plate: into the surviving
// plate merge, or as a new plate on cell.
type placement struct {
	sp    SnapshotPlate
	cell  CellKey
	merge *Plate
}

// placeSnapshot decides, without writing, where each snapshot plate goes. It
// records skipped cells in res and returns the plates to write, in snapshot
// order, and the plates ConflictOverwrite replaces. cells are the snapshot
// plates' cells as validateSnapshot parsed them.
func (s *Service) placeSnapshot(ctx context.Context, snap PlanSnapshot, cells []CellKey, mode ConflictMode, res *ApplyResult) ([]placement, []Plate, error) {
	var steps []placement
	var replace []Plate
	for i, sp := range snap.Plates {
		cell := cells[i]

		if sp.PlateID != nil {
			p, err := s.plates.Get(ctx, *sp.PlateID)
			switch {
			case errors.Is(err, domain.ErrNotFound):
			case err != nil:
				return nil, nil, err
			case p.Cell().Equal(cell):
				steps = append(steps, placement{sp: sp, cell: cell, merge: p})
				continue
			}
		}

		cellMode, err := s.cellMode(ctx, cell)
		if err != nil {
			return nil, nil, err
		}
		if cellMode == CellNone {
			if mode == ConflictFail {
				return nil, nil, fmt.Errorf("%w: %s slot %d", domain.ErrCellUnavailable, sp.Date, sp.SlotID)
			}
			res.SkippedUnavailable = append(res.SkippedUnavailable, cell)
			continue
		}
		existingID, err := s.plates.GetIDByCell(ctx, cell.Date, cell.SlotID)
		switch {
		case errors.Is(err, domain.ErrNotFound):
		case err != nil:
			return nil, nil, err
		case mode == ConflictFail:
			return nil, nil, fmt.Errorf("%w: %s slot %d", domain.ErrCellOccupied, sp.Date, sp.SlotID)
		case mode == ConflictSkip:
			res.SkippedOccupied = append(res.SkippedOccupied, cell)
			continue
		default:
			old, err := s.plates.Get(ctx, existingID)
			if err != nil {
				return nil, nil, err
			}
			replace = append(replace, *old)
		}
		steps = append(steps, placement{sp: sp, cell: cell})
	}
	return steps, replace, nil
}

// mergeInto adds a snapshot plate's components back to its surviving plate p.
// A link that joins p to a session of the other state follows D3 (joinLink).
func (s *Service) mergeInto(ctx context.Context, p *Plate, sp SnapshotPlate, sessByRef map[string]validSession, created map[string]int64, e env, t *tracker, restore bool) error {
	comps, err := s.snapshotComponents(ctx, t.j, sp.Components, sessByRef, created, restore)
	if err != nil {
		return err
	}
	st := p.State
	for i := range comps {
		pc := &comps[i]
		f, err := s.normaliseComponent(ctx, pc)
		if err != nil {
			return err
		}
		linked, err := s.attach(ctx, t, pc, f, p.Cell(), p.Eaters, e, false, freshOrigin(ctx, st))
		if err != nil {
			return err
		}
		if st, err = s.joinLink(ctx, t, p.ID, st, linked); err != nil {
			return err
		}
		pc.PlateID = p.ID
		if err := s.plates.CreateComponent(ctx, pc); err != nil {
			return err
		}
	}
	return s.plates.Touch(ctx, p.ID)
}

// snapshotComponents turns snapshot components into plate components,
// creating each referenced snapshot session on first use (state, run and
// reason per snapshotOrigin).
func (s *Service) snapshotComponents(ctx context.Context, j *journal, in []SnapshotComponent, sessByRef map[string]validSession, created map[string]int64, restore bool) ([]PlateComponent, error) {
	out := make([]PlateComponent, len(in))
	for i, c := range in {
		pc := PlateComponent{
			FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, Grams: c.Grams,
			SortOrder: c.SortOrder,
		}
		if c.Session != nil {
			switch {
			case c.Session.ID != 0:
				id := c.Session.ID
				pc.CookSessionID = &id
			case c.Session.Ref != "":
				id, ok := created[c.Session.Ref]
				if !ok {
					ss := sessByRef[c.Session.Ref]
					o := snapshotOrigin(ctx, ss.State, ss.AIRunID, ss.Reason, restore)
					sess := &CookSession{
						FoodID: ss.FoodID, CookDate: ss.cookDate, CookSlotID: ss.CookSlotID,
						Portions: ss.Portions, FrozenPortions: ss.FrozenPortions,
						CookedAt: ss.CookedAt, Note: ss.Note,
						State: o.state, AIRunID: o.runID, Reason: o.reason,
					}
					if err := s.sessions.CreateSession(ctx, sess); err != nil {
						return nil, err
					}
					j.createSession(sess.ID)
					id = sess.ID
					created[c.Session.Ref] = id
				}
				pc.CookSessionID = &id
			}
		}
		out[i] = pc
	}
	return out, nil
}

// validSession is a snapshot session with its parsed cook date.
type validSession struct {
	SnapshotSession
	cookDate time.Time
}

// validateSnapshot checks a snapshot's shape before anything is written. It
// returns its sessions by ref and the plates' cells, index-aligned with
// snap.Plates, with the dates it parsed.
func validateSnapshot(snap PlanSnapshot) (map[string]validSession, []CellKey, error) {
	byRef := make(map[string]validSession, len(snap.Sessions))
	for i, ss := range snap.Sessions {
		if ss.Ref == "" {
			return nil, nil, fmt.Errorf("%w: sessions[%d] ref required", domain.ErrInvalidInput, i)
		}
		if _, dup := byRef[ss.Ref]; dup {
			return nil, nil, fmt.Errorf("%w: duplicate session ref %q", domain.ErrInvalidInput, ss.Ref)
		}
		cookDate, err := time.Parse(dateLayout, ss.CookDate)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: sessions[%d] cook_date", domain.ErrInvalidInput, i)
		}
		if !(ss.Portions > 0) || ss.FrozenPortions < 0 || ss.FrozenPortions > ss.Portions {
			return nil, nil, fmt.Errorf("%w: sessions[%d] portions", domain.ErrInvalidInput, i)
		}
		if ss.State != "" && !ss.State.Valid() {
			return nil, nil, fmt.Errorf("%w: sessions[%d] state", domain.ErrInvalidInput, i)
		}
		byRef[ss.Ref] = validSession{SnapshotSession: ss, cookDate: cookDate}
	}
	cells := make([]CellKey, len(snap.Plates))
	seen := make(map[CellKey]bool, len(snap.Plates))
	for i, sp := range snap.Plates {
		date, err := time.Parse(dateLayout, sp.Date)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: plates[%d] date", domain.ErrInvalidInput, i)
		}
		c := CellKey{Date: date, SlotID: sp.SlotID}
		if seen[c] {
			return nil, nil, fmt.Errorf("%w: plates[%d] duplicate cell %s slot %d", domain.ErrInvalidInput, i, sp.Date, sp.SlotID)
		}
		seen[c] = true
		cells[i] = c
		if sp.Eaters < 0 {
			return nil, nil, fmt.Errorf("%w: plates[%d] eaters", domain.ErrInvalidInput, i)
		}
		if sp.State != "" && !sp.State.Valid() {
			return nil, nil, fmt.Errorf("%w: plates[%d] state", domain.ErrInvalidInput, i)
		}
		for j, c := range sp.Components {
			if c.Session == nil {
				continue
			}
			if (c.Session.ID != 0) == (c.Session.Ref != "") {
				return nil, nil, fmt.Errorf("%w: plates[%d].components[%d] session needs exactly one of id or ref", domain.ErrInvalidInput, i, j)
			}
			if c.Session.Ref != "" {
				if _, ok := byRef[c.Session.Ref]; !ok {
					return nil, nil, fmt.Errorf("%w: plates[%d].components[%d] unknown session ref %q", domain.ErrInvalidInput, i, j, c.Session.Ref)
				}
			}
		}
	}
	return byRef, cells, nil
}

// ── Removal ─────────────────────────────────────────────────────────────

func allComponents(PlateComponent) bool { return true }

// refAlloc hands out snapshot session refs ("s1", "s2", …).
type refAlloc struct{ n int }

func (r *refAlloc) next() string {
	r.n++
	return "s" + strconv.Itoa(r.n)
}

// remove deletes the components of plates that pick selects, then the
// sessions left without links (others follow D11), then the plates: every
// plate, or with keepPartial only those left without components. It returns
// the snapshot that restores what it removed: a component whose session
// survives links {id}, one whose session goes links {ref}.
//
// With a nil t, remove settles its own tracker. With the caller's t it only
// records into it and leaves the settle to the caller, and the pinned
// sessions count as surviving: the caller links them again before it settles.
// Every plate is touched in j; the caller commits j.
func (s *Service) remove(ctx context.Context, plates []Plate, pick func(PlateComponent) bool, keepPartial bool, refs *refAlloc, t *tracker, pinned map[int64]bool, j *journal) (PlanSnapshot, error) {
	for _, p := range plates {
		j.touch(p)
	}
	snap := emptySnapshot()
	removed := map[int64]bool{}
	var sessionIDs []int64
	seenSession := map[int64]bool{}
	for _, p := range plates {
		for _, c := range p.Components {
			if !pick(c) {
				continue
			}
			removed[c.ID] = true
			if c.CookSessionID != nil && !seenSession[*c.CookSessionID] {
				seenSession[*c.CookSessionID] = true
				sessionIDs = append(sessionIDs, *c.CookSessionID)
			}
		}
	}

	settleHere := t == nil
	if settleHere {
		t = newTracker(j)
	}
	refOf := map[int64]string{}
	for _, sid := range sessionIDs {
		links, err := s.sessions.ListLinks(ctx, sid)
		if err != nil {
			return PlanSnapshot{}, err
		}
		t.record(sid, totalPortions(links))
		survives := pinned[sid]
		for _, l := range links {
			if !removed[l.ComponentID] {
				survives = true
				break
			}
		}
		if survives {
			continue
		}
		sess, err := s.sessions.GetSession(ctx, sid)
		if err != nil {
			return PlanSnapshot{}, err
		}
		ref := refs.next()
		refOf[sid] = ref
		snap.Sessions = append(snap.Sessions, SnapshotSession{
			Ref: ref, FoodID: sess.FoodID, CookDate: sess.CookDate.Format(dateLayout), CookSlotID: sess.CookSlotID,
			Portions: sess.Portions, FrozenPortions: sess.FrozenPortions, CookedAt: sess.CookedAt, Note: sess.Note,
			State: sess.State, AIRunID: sess.AIRunID, Reason: sess.Reason,
		})
	}

	deletePlate := make([]bool, len(plates))
	for i, p := range plates {
		var comps []SnapshotComponent
		for _, c := range p.Components {
			if !removed[c.ID] {
				continue
			}
			sc := SnapshotComponent{FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, Grams: c.Grams, SortOrder: c.SortOrder}
			if c.CookSessionID != nil {
				if ref, ok := refOf[*c.CookSessionID]; ok {
					sc.Session = &SessionLink{Ref: ref}
				} else {
					sc.Session = &SessionLink{ID: *c.CookSessionID}
				}
			}
			comps = append(comps, sc)
		}
		deletePlate[i] = !keepPartial || len(comps) == len(p.Components)
		if len(comps) == 0 && !deletePlate[i] {
			continue
		}
		if comps == nil {
			comps = []SnapshotComponent{}
		}
		entry := SnapshotPlate{
			Date: p.DateString(), SlotID: p.SlotID, Eaters: p.Eaters, Note: p.Note,
			State: p.State, AIRunID: p.AIRunID, Reason: p.Reason, Components: comps,
		}
		if p.Feedback != nil {
			entry.Feedback = &SnapshotFeedback{Status: p.Feedback.Status, Note: p.Feedback.Note, RatedAt: p.Feedback.RatedAt}
		}
		if !deletePlate[i] {
			id := p.ID
			entry.PlateID = &id
		}
		snap.Plates = append(snap.Plates, entry)
	}

	for _, p := range plates {
		for _, c := range p.Components {
			if removed[c.ID] {
				if err := s.plates.DeleteComponent(ctx, c.ID); err != nil {
					return PlanSnapshot{}, err
				}
			}
		}
	}
	if settleHere {
		if err := s.settle(ctx, t); err != nil {
			return PlanSnapshot{}, err
		}
	}
	for i, p := range plates {
		if deletePlate[i] {
			if err := s.plates.Delete(ctx, p.ID); err != nil {
				return PlanSnapshot{}, err
			}
			continue
		}
		if err := s.plates.Touch(ctx, p.ID); err != nil {
			return PlanSnapshot{}, err
		}
	}
	return snap, nil
}

func emptySnapshot() PlanSnapshot {
	return PlanSnapshot{Plates: []SnapshotPlate{}, Sessions: []SnapshotSession{}}
}

// ── D11 tracking ────────────────────────────────────────────────────────

// tracker records the consumption of existing sessions before a mutation so
// settle can apply D11 after it. j is the mutation's journal: the session
// writes of settle, attach and Apply record into it. unify lists the plates
// whose batch closure settle makes take the plate's state (D3).
type tracker struct {
	before map[int64]float64
	ids    []int64
	unify  []int64
	j      *journal
}

func newTracker(j *journal) *tracker { return &tracker{before: map[int64]float64{}, j: j} }

func (t *tracker) record(id int64, total float64) {
	if _, ok := t.before[id]; ok {
		return
	}
	t.before[id] = total
	t.ids = append(t.ids, id)
}

// track records session id's current consumption unless already tracked.
func (s *Service) track(ctx context.Context, t *tracker, id int64) error {
	if _, ok := t.before[id]; ok {
		return nil
	}
	links, err := s.sessions.ListLinks(ctx, id)
	if err != nil {
		return err
	}
	t.record(id, totalPortions(links))
	return nil
}

// settle applies D11 to every tracked session: one without links is deleted;
// the others get Sync'd portions. Then it unifies the closures of t.unify.
func (s *Service) settle(ctx context.Context, t *tracker) error {
	for _, id := range t.ids {
		links, err := s.sessions.ListLinks(ctx, id)
		if err != nil {
			return err
		}
		sess, err := s.sessions.GetSession(ctx, id)
		if errors.Is(err, domain.ErrNotFound) && len(links) == 0 {
			continue
		}
		if err != nil {
			return err
		}
		t.j.touchSession(*sess)
		if len(links) == 0 {
			if err := s.sessions.DeleteSession(ctx, id); err != nil {
				return err
			}
			continue
		}
		portions, frozen := Sync(Usage{Total: t.before[id]}, Usage{Total: totalPortions(links)}, sess.Portions, sess.FrozenPortions)
		if portions == sess.Portions && frozen == sess.FrozenPortions {
			continue
		}
		sess.Portions, sess.FrozenPortions = portions, frozen
		if err := s.sessions.UpdateSession(ctx, sess); err != nil {
			return err
		}
	}
	return s.unify(ctx, t)
}

func totalPortions(links []Link) float64 {
	var total float64
	for _, l := range links {
		total += l.PerEater * l.Eaters
	}
	return total
}

// ── Helpers ─────────────────────────────────────────────────────────────

// env is what batch rules need: the settings and the slot order.
type env struct {
	set   BatchSettings
	order SlotOrder
}

func (s *Service) loadEnv(ctx context.Context) (env, error) {
	var e env
	set, err := ReadBatchSettings(ctx, s.settings)
	if err != nil {
		return e, err
	}
	e.set = set
	e.order, err = s.slotOrder(ctx)
	return e, err
}

// ReadBatchSettings reads the plan.* keep-days and freezer settings the batch
// rules use.
func ReadBatchSettings(ctx context.Context, r SettingsReader) (BatchSettings, error) {
	keep, err := IntSetting(ctx, r, settings.KeyPlanDefaultKeepDays)
	if err != nil {
		return BatchSettings{}, err
	}
	freezerDays, err := IntSetting(ctx, r, settings.KeyPlanFreezerKeepDays)
	if err != nil {
		return BatchSettings{}, err
	}
	freezing, err := boolSetting(ctx, r, settings.KeyPlanFreezingEnabled)
	if err != nil {
		return BatchSettings{}, err
	}
	return BatchSettings{DefaultKeepDays: keep, FreezingEnabled: freezing, FreezerKeepDays: freezerDays}, nil
}

// IntSetting reads and parses an int setting.
func IntSetting(ctx context.Context, r SettingsReader, key string) (int, error) {
	v, err := r.Get(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", key, err)
	}
	n, err := strconv.Atoi(v.Raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return n, nil
}

// boolSetting parses a bool setting the way the settings validator accepts
// it (strconv.ParseBool).
func boolSetting(ctx context.Context, r SettingsReader, key string) (bool, error) {
	v, err := r.Get(ctx, key)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", key, err)
	}
	b, err := strconv.ParseBool(v.Raw)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return b, nil
}

func (s *Service) slotOrder(ctx context.Context) (SlotOrder, error) {
	all, err := s.slots.List(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list slots: %w", err)
	}
	order := make(SlotOrder, len(all))
	for _, sl := range all {
		order[sl.ID] = sl.SortOrder
	}
	return order, nil
}

func sortPlates(plates []Plate, order SlotOrder) {
	sort.SliceStable(plates, func(i, j int) bool { return Before(plates[i].Cell(), plates[j].Cell(), order) })
}

// normaliseNote trims a note; empty becomes nil, longer than maxLen runes is
// invalid.
func normaliseNote(n *string, maxLen int) (*string, error) {
	if n == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*n)
	if utf8.RuneCountInString(v) > maxLen {
		return nil, fmt.Errorf("%w: note longer than %d characters", domain.ErrInvalidInput, maxLen)
	}
	if v == "" {
		return nil, nil
	}
	return &v, nil
}

func (s *Service) cellMode(ctx context.Context, c CellKey) (string, error) {
	if s.cells == nil {
		return CellPlan, nil
	}
	mode, err := s.cells.CellMode(ctx, c.Date, c.SlotID)
	if err != nil {
		return "", fmt.Errorf("check cell: %w", err)
	}
	return mode, nil
}

// checkCell rejects a cell that resolves to "none" (ErrCellUnavailable) or
// holds a plate other than selfID (ErrCellOccupied). selfID is 0 on create.
func (s *Service) checkCell(ctx context.Context, c CellKey, selfID int64) error {
	mode, err := s.cellMode(ctx, c)
	if err != nil {
		return err
	}
	if mode == CellNone {
		return fmt.Errorf("%w: %s slot %d", domain.ErrCellUnavailable, c.Date.Format(dateLayout), c.SlotID)
	}
	existingID, err := s.plates.GetIDByCell(ctx, c.Date, c.SlotID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("check cell: %w", err)
	case existingID != selfID:
		return fmt.Errorf("%w: plate %d", domain.ErrCellOccupied, existingID)
	}
	return nil
}

func (s *Service) validateSlot(ctx context.Context, slotID int64) error {
	if slotID <= 0 {
		return fmt.Errorf("%w: slot_id required", domain.ErrSlotUnknown)
	}
	ok, err := s.slots.Exists(ctx, slotID)
	if err != nil {
		return fmt.Errorf("check slot: %w", err)
	}
	if !ok {
		return fmt.Errorf("%w: slot %d does not exist", domain.ErrSlotUnknown, slotID)
	}
	return nil
}

func (s *Service) resolveEaters(ctx context.Context, eaters *float64) (float64, error) {
	if eaters != nil {
		return *eaters, validateEaters(*eaters)
	}
	v, err := s.profile.HouseholdPortions(ctx)
	if err != nil {
		return 0, fmt.Errorf("read household portions: %w", err)
	}
	return v, nil
}

func validateEaters(v float64) error {
	if !(v > 0) || v > MaxEaters {
		return fmt.Errorf("%w: eaters must be above 0 and at most %g", domain.ErrInvalidInput, MaxEaters)
	}
	return nil
}

func validateRange(from, to time.Time) error {
	if from.After(to) {
		return fmt.Errorf("%w: from must not be after to", domain.ErrInvalidInput)
	}
	if to.Sub(from) > maxRangeDays*24*time.Hour {
		return fmt.Errorf("%w: range exceeds %d days", domain.ErrInvalidInput, maxRangeDays)
	}
	return nil
}

// attach gives a composed component its session: a fresh one at cell when it
// names none (portions = per-eater portions × eaters, state and run from
// fresh), else the named one after checking it (food always; the D13 cell
// rules when strict), which it returns. The named session is tracked for D11.
func (s *Service) attach(ctx context.Context, t *tracker, pc *PlateComponent, f *food.Food, cell CellKey, eaters float64, e env, strict bool, fresh origin) (*CookSession, error) {
	if f.Kind != food.KindComposed {
		return nil, nil
	}
	if pc.CookSessionID == nil {
		sess := &CookSession{
			FoodID: f.ID, CookDate: cell.Date, CookSlotID: cell.SlotID, Portions: *pc.Portions * eaters,
			State: fresh.state, AIRunID: fresh.runID,
		}
		if err := s.sessions.CreateSession(ctx, sess); err != nil {
			return nil, err
		}
		t.j.createSession(sess.ID)
		pc.CookSessionID = &sess.ID
		return nil, nil
	}
	sess, err := s.sessions.GetSession(ctx, *pc.CookSessionID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%w: session %d", domain.ErrSessionNotFound, *pc.CookSessionID)
	}
	if err != nil {
		return nil, err
	}
	if strict {
		if err := CheckLink(cell, f.ID, *sess, e.set.KeepDays(f.KeepDays), e.set, e.order); err != nil {
			return nil, err
		}
	} else if f.ID != sess.FoodID {
		return nil, fmt.Errorf("%w: food %d, session %d cooks food %d", domain.ErrSessionFoodMismatch, f.ID, sess.ID, sess.FoodID)
	}
	return sess, s.track(ctx, t, sess.ID)
}

// normaliseComponent validates the quantity shape against the food's kind and
// resolves leaf grams in place. A composed component without portions gets 1.
func (s *Service) normaliseComponent(ctx context.Context, pc *PlateComponent) (*food.Food, error) {
	if pc.FoodID <= 0 {
		return nil, fmt.Errorf("%w: food_id required", domain.ErrInvalidInput)
	}
	hasPortions := pc.Portions != nil
	hasLeaf := pc.Amount != nil || pc.Unit != nil
	if hasPortions && hasLeaf {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrInvalidQuantityShape)
	}

	f, err := s.foods.Get(ctx, pc.FoodID)
	if err != nil {
		// food.Repository wraps not-found as domain.ErrNotFound already.
		return nil, err
	}

	switch f.Kind {
	case food.KindComposed:
		if hasLeaf {
			return nil, fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrInvalidQuantityForComposed)
		}
		if pc.Portions == nil {
			one := 1.0
			pc.Portions = &one
		}
		if !(*pc.Portions > 0) {
			return nil, fmt.Errorf("%w: portions must be positive", domain.ErrInvalidInput)
		}
		pc.Amount, pc.Unit, pc.Grams, pc.GramsSource = nil, nil, nil, nil
		return f, nil

	case food.KindLeaf:
		if hasPortions {
			return nil, fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrInvalidQuantityForLeaf)
		}
		if !hasLeaf {
			return nil, fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrInvalidQuantityShape)
		}
		if pc.CookSessionID != nil {
			return nil, fmt.Errorf("%w: a leaf component has no cook session", domain.ErrInvalidInput)
		}
		if pc.Amount == nil || !(*pc.Amount > 0) {
			return nil, fmt.Errorf("%w: amount must be positive", domain.ErrInvalidInput)
		}
		if pc.Unit == nil || *pc.Unit == "" {
			return nil, fmt.Errorf("%w: unit required", domain.ErrInvalidInput)
		}
		manualGrams := float64(0)
		if pc.Grams != nil {
			manualGrams = *pc.Grams
		}
		grams, source, err := food.ResolveGrams(ctx, s.foods, pc.FoodID, *pc.Amount, *pc.Unit, manualGrams)
		if err != nil {
			// Surface a specific key when the failure is the count-unit /
			// missing-portion case; otherwise let the error flow through.
			if errors.Is(err, domain.ErrInvalidInput) {
				return nil, fmt.Errorf("%w: %w", err, ErrUnitRequiresPortion)
			}
			return nil, err
		}
		canonical := *pc.Unit
		if normalized := units.Normalize(canonical); normalized != "" {
			canonical = normalized
		}
		pc.Unit = &canonical
		pc.Grams = &grams
		src := source
		pc.GramsSource = &src
		return f, nil

	default:
		return nil, fmt.Errorf("%w: food %d has unknown kind %q", domain.ErrInvalidInput, pc.FoodID, f.Kind)
	}
}

func linkedSessions(comps []PlateComponent) []int64 {
	var ids []int64
	seen := map[int64]bool{}
	for _, c := range comps {
		if c.CookSessionID != nil && !seen[*c.CookSessionID] {
			seen[*c.CookSessionID] = true
			ids = append(ids, *c.CookSessionID)
		}
	}
	return ids
}

func nextSortOrder(comps []PlateComponent) int {
	next := 0
	for _, c := range comps {
		if c.SortOrder >= next {
			next = c.SortOrder + 1
		}
	}
	return next
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
