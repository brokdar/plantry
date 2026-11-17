package plate

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Meal operations (meal editing slice 2b): move, swap, replace by position,
// move cook and copy. Each runs in one unit of work, writes plate events and
// returns a MealResult with its Undo. A source or target cell before today is
// domain.ErrPastCell. Hand edits on confirmed rows are allowed; the agent is
// guarded by the draft guard.
//
// The local rule: when a meal moves, each batch it eats from first
// re-anchors its cook to the batch's earliest meal; when that would break the
// keeping time (R2) or the batch is cooked, the named meal leaves the batch
// and its dish cooks fresh at its own cell. A meal the caller did not name
// never loses its dish; at most it becomes its batch's cooking meal.

// OccupantKind says what MoveMeal does with a plate on the target cell.
type OccupantKind string

// Occupant kinds. OccupantNone refuses a taken cell (OccupiedError).
const (
	OccupantNone OccupantKind = ""
	OccupantSwap OccupantKind = "swap"
	OccupantTo   OccupantKind = "to"
	OccupantDrop OccupantKind = "drop"
)

// Occupant is MoveMeal's handling of the target's plate: swap it to the
// source cell (eaters stay with the cells), move it on To (an empty cell),
// or drop it (RemoveMeals).
type Occupant struct {
	Kind OccupantKind
	To   CellKey
}

// OccupiedError is domain.ErrCellOccupied naming the plate on the cell.
type OccupiedError struct{ PlateID int64 }

func (e *OccupiedError) Error() string {
	return fmt.Sprintf("%v: plate %d", domain.ErrCellOccupied, e.PlateID)
}

func (e *OccupiedError) Unwrap() error { return domain.ErrCellOccupied }

// parkDate is where a swap parks the occupant for a moment inside its unit of
// work, so the (date, slot) unique index never sees two plates on one cell.
var parkDate = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)

// relocation is one plate of a move: as it was and as it will be.
type relocation struct {
	before, after Plate
}

// MoveMeal moves a plate with its components to cell to. occ applies when
// to holds a plate. The target must not be "none"; neither cell may be past.
// Each batch the moved plates eat from then follows the local rule.
func (s *Service) MoveMeal(ctx context.Context, plateID int64, to CellKey, occ Occupant) (MealResult, error) {
	var res MealResult
	undo, err := s.Recorded(ctx, func(ctx context.Context) error {
		cur, err := s.plates.Get(ctx, plateID)
		if err != nil {
			return err
		}
		if err := s.validateSlot(ctx, to.SlotID); err != nil {
			return err
		}
		next := *cur
		next.Date, next.SlotID = dateOnly(to.Date), to.SlotID
		if next.Cell().Equal(cur.Cell()) {
			return fmt.Errorf("%w: the plate is on that cell", domain.ErrInvalidInput)
		}
		res, err = s.moveMeal(ctx, *cur, next, occ)
		return err
	})
	if err != nil {
		return MealResult{}, err
	}
	res.Undo = undo
	return res, nil
}

// moveMeal moves cur to next's cell (next may also carry new eaters and note)
// inside the caller's unit of work.
func (s *Service) moveMeal(ctx context.Context, cur, next Plate, occ Occupant) (MealResult, error) {
	today := dateOnly(s.now())
	if err := notPast(today, cur.Cell(), next.Cell()); err != nil {
		return MealResult{}, err
	}
	if err := s.guardPlates(ctx, cur); err != nil {
		return MealResult{}, err
	}
	mode, err := s.cellMode(ctx, next.Cell())
	if err != nil {
		return MealResult{}, err
	}
	if mode == CellNone {
		return MealResult{}, fmt.Errorf("%w: %s slot %d", domain.ErrCellUnavailable, next.DateString(), next.SlotID)
	}
	moves := []relocation{{before: cur, after: next}}
	var dropped []Plate
	occID, err := s.plates.GetIDByCell(ctx, next.Date, next.SlotID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
	case err != nil:
		return MealResult{}, err
	default:
		q, err := s.plates.Get(ctx, occID)
		if err != nil {
			return MealResult{}, err
		}
		if err := s.guardPlates(ctx, *q); err != nil {
			return MealResult{}, err
		}
		qNext := *q
		switch occ.Kind {
		case OccupantSwap:
			qNext.Date, qNext.SlotID = cur.Date, cur.SlotID
			moves[0].after.Eaters, qNext.Eaters = q.Eaters, cur.Eaters
		case OccupantTo:
			c := CellKey{Date: dateOnly(occ.To.Date), SlotID: occ.To.SlotID}
			if err := s.validateSlot(ctx, c.SlotID); err != nil {
				return MealResult{}, err
			}
			if err := notPast(today, c); err != nil {
				return MealResult{}, err
			}
			if c.Equal(next.Cell()) {
				return MealResult{}, fmt.Errorf("%w: the occupant must move to another cell", domain.ErrInvalidInput)
			}
			if err := s.checkCell(ctx, c, cur.ID); err != nil { // the source cell is free by then
				return MealResult{}, err
			}
			qNext.Date, qNext.SlotID = c.Date, c.SlotID
		case OccupantDrop:
			dropped = append(dropped, *q)
		default:
			return MealResult{}, &OccupiedError{PlateID: occID}
		}
		if occ.Kind != OccupantDrop {
			moves = append(moves, relocation{before: *q, after: qNext})
		}
	}

	e, err := s.loadEnv(ctx)
	if err != nil {
		return MealResult{}, err
	}
	j := newJournal()
	t := newTracker(j)
	named := map[int64]relocation{}
	var sids []int64
	for _, m := range moves {
		j.touch(m.before)
		named[m.before.ID] = m
		sids = append(sids, linkedSessions(m.before.Components)...)
	}
	sids = uniqueIDs(sids)
	portions, err := s.trackPortions(ctx, t, sids)
	if err != nil {
		return MealResult{}, err
	}
	if len(dropped) > 0 {
		if _, err := s.remove(ctx, dropped, allComponents, false, &refAlloc{}, t, nil, j); err != nil {
			return MealResult{}, err
		}
	}
	if len(moves) == 2 {
		parked := moves[1].before
		parked.Date = parkDate
		if err := s.plates.Update(ctx, &parked); err != nil {
			return MealResult{}, err
		}
	}
	for i := range moves {
		if err := s.plates.Update(ctx, &moves[i].after); err != nil {
			return MealResult{}, err
		}
	}
	for _, sid := range sids {
		if err := s.localRule(ctx, t, sid, named, e, today); err != nil {
			return MealResult{}, err
		}
	}
	if err := s.settle(ctx, t); err != nil {
		return MealResult{}, err
	}
	if err := s.reportResized(ctx, t, sids, portions); err != nil {
		return MealResult{}, err
	}
	if err := s.commit(ctx, j); err != nil {
		return MealResult{}, err
	}
	ids := make([]int64, len(moves))
	for i, m := range moves {
		ids[i] = m.before.ID
	}
	return s.mealResult(ctx, t, ids)
}

// trackPortions tracks sids for D11 and returns each one's portions before
// the mutation.
func (s *Service) trackPortions(ctx context.Context, t *tracker, sids []int64) (map[int64]float64, error) {
	portions := make(map[int64]float64, len(sids))
	for _, sid := range sids {
		if err := s.track(ctx, t, sid); err != nil {
			return nil, err
		}
		sess, err := s.sessions.GetSession(ctx, sid)
		if err != nil {
			return nil, err
		}
		portions[sid] = sess.Portions
	}
	return portions, nil
}

// localRule decides batch sid after the named plates moved (their rows are
// written): the cook re-anchors to the earliest meal when a cooking meal
// moved or a named meal now comes before the cook (the latter only when R2
// holds for every meal); then every named meal the batch cannot feed (before
// the cook, past the keeping time, or the batch is cooked) cooks fresh. The
// session is kept out of settle's re-anchor.
func (s *Service) localRule(ctx context.Context, t *tracker, sid int64, named map[int64]relocation, e env, today time.Time) error {
	sess, err := s.sessions.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	links, err := s.sessions.ListLinks(ctx, sid)
	if err != nil {
		return err
	}
	var mine []Link
	for _, l := range links {
		if _, ok := named[l.PlateID]; ok {
			mine = append(mine, l)
		}
	}
	if len(mine) == 0 {
		return nil
	}
	if t.noMove == nil {
		t.noMove = map[int64]bool{}
	}
	t.noMove[sid] = true
	f, err := s.foods.Get(ctx, sess.FoodID)
	if err != nil {
		return err
	}
	keep := e.set.KeepDays(f.KeepDays)
	if sess.CookedAt == nil {
		cook := sess.CookCell()
		cookMoved := false
		for _, l := range mine {
			if named[l.PlateID].before.Cell().Equal(t.prior[sid].cook) {
				cookMoved = true
			}
		}
		target := cook
		if first, ok := earliestMeal(links, today, e.order); ok {
			switch {
			case cookMoved:
				target = first
			case Before(first, cook, e.order):
				cand := *sess
				cand.CookDate, cand.CookSlotID = first.Date, first.SlotID
				if feedsAll(cand, links, keep, e) {
					target = first
				}
			}
		}
		if !target.Equal(cook) {
			t.j.touchSession(*sess)
			others := len(links) > len(mine)
			sess.CookDate, sess.CookSlotID = target.Date, target.SlotID
			if err := s.sessions.UpdateSession(ctx, sess); err != nil {
				return err
			}
			ac := AlsoChanged{
				SessionID: sid, Date: target.Date.Format(dateLayout), SlotID: target.SlotID, What: AlsoCookMoved,
				FromDate: cook.Date.Format(dateLayout), FromSlotID: cook.SlotID,
			}
			for _, l := range links {
				if l.Cell.Equal(target) {
					ac.PlateID = l.PlateID
					if _, ok := named[l.PlateID]; !ok {
						ac.What = AlsoBecameCooking
					}
				}
			}
			if others { // a batch of named meals only is no other meal's concern
				t.moves = append(t.moves, ac)
			}
		}
	}
	for _, l := range mine {
		err := CheckLink(l.Cell, sess.FoodID, *sess, keep, e.set, e.order)
		if err == nil {
			continue
		}
		p := named[l.PlateID].after
		if err := s.cookFresh(ctx, t, p, l, f, e); err != nil {
			return err
		}
		t.moves = append(t.moves, AlsoChanged{
			SessionID: sid, PlateID: l.PlateID, Date: l.Cell.Date.Format(dateLayout), SlotID: l.Cell.SlotID,
			What: AlsoCooksFresh, Why: splitReason(err, *sess),
		})
	}
	return nil
}

// cookFresh gives the component of link l on plate p a fresh session at its
// own cell.
func (s *Service) cookFresh(ctx context.Context, t *tracker, p Plate, l Link, f *food.Food, e env) error {
	i := slices.IndexFunc(p.Components, func(c PlateComponent) bool { return c.ID == l.ComponentID })
	if i < 0 {
		return fmt.Errorf("%w: component %d", domain.ErrNotFound, l.ComponentID)
	}
	pc := p.Components[i]
	pc.CookSessionID = nil
	if _, err := s.attach(ctx, t, &pc, f, l.Cell, l.Eaters, e, true, freshOrigin(ctx, p.State)); err != nil {
		return err
	}
	return s.plates.UpdateComponent(ctx, &pc)
}

// splitReason is AlsoChanged.Why of a meal that left its batch.
func splitReason(err error, sess CookSession) string {
	switch {
	case sess.CookedAt != nil:
		return "cooked"
	case errors.Is(err, domain.ErrSessionBeforeCook):
		return "before_cook"
	default:
		return "past_keep_days"
	}
}

// earliestMeal is the cell of the first link, in cell order, not before
// today.
func earliestMeal(links []Link, today time.Time, order SlotOrder) (CellKey, bool) {
	var first *CellKey
	for _, l := range links {
		if dateOnly(l.Cell.Date).Before(today) {
			continue
		}
		if first == nil || Before(l.Cell, *first, order) {
			c := l.Cell
			first = &c
		}
	}
	if first == nil {
		return CellKey{}, false
	}
	return *first, true
}

// feedsAll reports whether s can feed every link (CheckLink: on or after the
// cook, within the keeping or freezer time).
func feedsAll(s CookSession, links []Link, keep int, e env) bool {
	for _, l := range links {
		if CheckLink(l.Cell, s.FoodID, s, keep, e.set, e.order) != nil {
			return false
		}
	}
	return true
}

// reportResized adds batch_resized for every session of sids that still
// exists with other portions than before.
func (s *Service) reportResized(ctx context.Context, t *tracker, sids []int64, before map[int64]float64) error {
	sessions, err := s.sessions.ListSessionsByIDs(ctx, sids)
	if err != nil {
		return err
	}
	slices.SortFunc(sessions, func(a, b CookSession) int { return int(a.ID - b.ID) })
	for _, sess := range sessions {
		if almostEqual(sess.Portions, before[sess.ID]) {
			continue
		}
		t.moves = append(t.moves, AlsoChanged{
			SessionID: sess.ID, Date: sess.CookDate.Format(dateLayout), SlotID: sess.CookSlotID, What: AlsoBatchResized,
		})
	}
	return nil
}

// mealResult is the result of a meal operation: the plates of ids (in that
// order) as they are now and what else changed.
func (s *Service) mealResult(ctx context.Context, t *tracker, ids []int64) (MealResult, error) {
	plates, err := s.plates.ListByIDs(ctx, ids)
	if err != nil {
		return MealResult{}, err
	}
	slices.SortFunc(plates, func(a, b Plate) int { return slices.Index(ids, a.ID) - slices.Index(ids, b.ID) })
	also := t.moves
	if also == nil {
		also = []AlsoChanged{}
	}
	return MealResult{Removed: emptySnapshot(), Plates: plates, AlsoChanged: also}, nil
}

func notPast(today time.Time, cells ...CellKey) error {
	for _, c := range cells {
		if dateOnly(c.Date).Before(today) {
			return fmt.Errorf("%w: %s", domain.ErrPastCell, c.Date.Format(dateLayout))
		}
	}
	return nil
}

// ── Replace ─────────────────────────────────────────────────────────────

// Selector addresses a plate's component: by ComponentID, or by Position
// (main: the plate's main; side: its only side).
type Selector struct {
	Position    Position
	ComponentID int64
}

// Source is where a replacing composed food comes from: fresh at the plate's
// cell (zero value), eaten from batch SessionID (R2 checked), or a new batch
// cooked at CookAt (on or before the plate's cell).
type Source struct {
	SessionID int64
	CookAt    *CellKey
}

// ReplaceComponent replaces the food of the selected component, keeping its
// position and per-eater quantity unless qty is given. Its old batch shrinks
// (D11) and re-anchors (R1). The same food again is domain.ErrInvalidInput.
func (s *Service) ReplaceComponent(ctx context.Context, plateID int64, sel Selector, foodID int64, src Source, qty *PlateComponent) (MealResult, error) {
	var res MealResult
	undo, err := s.Recorded(ctx, func(ctx context.Context) error {
		p, err := s.plates.Get(ctx, plateID)
		if err != nil {
			return err
		}
		if err := notPast(dateOnly(s.now()), p.Cell()); err != nil {
			return err
		}
		if err := s.guardPlates(ctx, *p); err != nil {
			return err
		}
		pc, err := selectComponent(p, sel)
		if err != nil {
			return err
		}
		if pc.FoodID == foodID {
			return fmt.Errorf("%w: the component already is that food", domain.ErrInvalidInput)
		}
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		t := newTracker(newJournal())
		if err := s.replace(ctx, t, p, &pc, foodID, qty, src, e); err != nil {
			return err
		}
		if _, err := s.finishComponentChange(ctx, t, p); err != nil {
			return err
		}
		res, err = s.mealResult(ctx, t, []int64{p.ID})
		return err
	})
	if err != nil {
		return MealResult{}, err
	}
	res.Undo = undo
	return res, nil
}

// selectComponent returns the component sel names on p.
func selectComponent(p *Plate, sel Selector) (PlateComponent, error) {
	if sel.ComponentID != 0 {
		for _, c := range p.Components {
			if c.ID == sel.ComponentID {
				return c, nil
			}
		}
		return PlateComponent{}, fmt.Errorf("%w: component %d is not on plate %d", domain.ErrNotFound, sel.ComponentID, p.ID)
	}
	if sel.Position != PositionMain && sel.Position != PositionSide {
		return PlateComponent{}, fmt.Errorf("%w: position must be main or side", domain.ErrInvalidInput)
	}
	var found []PlateComponent
	for _, c := range p.Components {
		if c.Position == sel.Position {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 0:
		return PlateComponent{}, fmt.Errorf("%w: the plate has no %s", domain.ErrInvalidInput, sel.Position)
	case 1:
		return found[0], nil
	}
	return PlateComponent{}, fmt.Errorf("%w: the plate has several sides; name the component", domain.ErrInvalidInput)
}

// replace swaps pc's food to foodID inside a component change: the old batch
// is tracked (D11, R1 at settle), the new food eats from src.
func (s *Service) replace(ctx context.Context, t *tracker, p *Plate, pc *PlateComponent, foodID int64, qty *PlateComponent, src Source, e env) error {
	if src.SessionID != 0 && src.CookAt != nil {
		return fmt.Errorf("%w: session_id and cook_at exclude each other", domain.ErrInvalidInput)
	}
	if pc.CookSessionID != nil {
		if err := s.track(ctx, t, *pc.CookSessionID); err != nil {
			return err
		}
	}
	pc.FoodID = foodID
	pc.CookSessionID = nil
	if qty != nil {
		pc.Portions, pc.Amount, pc.Unit, pc.Grams = qty.Portions, qty.Amount, qty.Unit, qty.Grams
	}
	pc.GramsSource = nil
	f, err := s.normaliseComponent(ctx, pc)
	if err != nil {
		return err
	}
	if (src.SessionID != 0 || src.CookAt != nil) && f.Kind != food.KindComposed {
		return fmt.Errorf("%w: only a composed food eats from a batch", domain.ErrInvalidInput)
	}
	fresh := freshOrigin(ctx, p.State)
	switch {
	case src.SessionID != 0:
		id := src.SessionID
		pc.CookSessionID = &id
	case src.CookAt != nil:
		c := CellKey{Date: dateOnly(src.CookAt.Date), SlotID: src.CookAt.SlotID}
		if err := s.validateSlot(ctx, c.SlotID); err != nil {
			return err
		}
		if err := notPast(dateOnly(s.now()), c); err != nil {
			return err
		}
		mode, err := s.cellMode(ctx, c)
		if err != nil {
			return err
		}
		if mode == CellNone {
			return fmt.Errorf("%w: %s slot %d", domain.ErrCellUnavailable, c.Date.Format(dateLayout), c.SlotID)
		}
		sess := &CookSession{
			FoodID: f.ID, CookDate: c.Date, CookSlotID: c.SlotID, Portions: *pc.Portions * p.Eaters,
			State: fresh.state, AIRunID: fresh.runID,
		}
		if err := s.sessions.CreateSession(ctx, sess); err != nil {
			return err
		}
		t.j.createSession(sess.ID)
		pc.CookSessionID = &sess.ID
	}
	linked, err := s.attach(ctx, t, pc, f, p.Cell(), p.Eaters, e, true, fresh)
	if err != nil {
		return err
	}
	if _, err := s.joinLink(ctx, t, p.ID, p.State, linked); err != nil {
		return err
	}
	return s.plates.UpdateComponent(ctx, pc)
}

// ── Move cook ───────────────────────────────────────────────────────────

// MoveCook moves batch sessionID's cooking to cell to without moving its
// meals (cook ahead). The batch must be uncooked, to must not be "none" or
// past, and every meal must come on or after to and within the keeping time
// from it; otherwise domain.ErrInvalidInput names the blocking meals.
func (s *Service) MoveCook(ctx context.Context, sessionID int64, to CellKey) (MealResult, error) {
	var res MealResult
	undo, err := s.Recorded(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.GetSession(ctx, sessionID)
		if err != nil {
			return err
		}
		if err := s.guardSession(ctx, *sess); err != nil {
			return err
		}
		to = CellKey{Date: dateOnly(to.Date), SlotID: to.SlotID}
		if err := s.validateSlot(ctx, to.SlotID); err != nil {
			return err
		}
		if err := notPast(dateOnly(s.now()), to); err != nil {
			return err
		}
		mode, err := s.cellMode(ctx, to)
		if err != nil {
			return err
		}
		if mode == CellNone {
			return fmt.Errorf("%w: %s slot %d", domain.ErrCellUnavailable, to.Date.Format(dateLayout), to.SlotID)
		}
		if sess.CookedAt != nil {
			return fmt.Errorf("%w: session %d is cooked", domain.ErrInvalidInput, sess.ID)
		}
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		f, err := s.foods.Get(ctx, sess.FoodID)
		if err != nil {
			return err
		}
		links, err := s.sessions.ListLinks(ctx, sess.ID)
		if err != nil {
			return err
		}
		cand := *sess
		cand.CookDate, cand.CookSlotID = to.Date, to.SlotID
		keep := e.set.KeepDays(f.KeepDays)
		var blocking []string
		for _, l := range OrderLinks(cand, links, keep, e.order) {
			where := fmt.Sprintf("plate %d (%s slot %d)", l.PlateID, l.Cell.Date.Format(dateLayout), l.Cell.SlotID)
			if Before(l.Cell, to, e.order) {
				blocking = append(blocking, where+" comes before the cook")
			} else if CheckLink(l.Cell, cand.FoodID, cand, keep, e.set, e.order) != nil {
				blocking = append(blocking, where+" is past the keeping time")
			}
		}
		if len(blocking) > 0 {
			return fmt.Errorf("%w: %s", domain.ErrInvalidInput, strings.Join(blocking, "; "))
		}
		if !to.Equal(sess.CookCell()) {
			j := newJournal()
			j.touchSession(*sess)
			if err := s.sessions.UpdateSession(ctx, &cand); err != nil {
				return err
			}
			if err := s.commit(ctx, j); err != nil {
				return err
			}
		}
		res = MealResult{Removed: emptySnapshot(), Plates: []Plate{}, Sessions: []CookSession{cand}, AlsoChanged: []AlsoChanged{}}
		return nil
	})
	if err != nil {
		return MealResult{}, err
	}
	res.Undo = undo
	return res, nil
}

// ── Copy ────────────────────────────────────────────────────────────────

// CopyAs is how CopyMeal's copies get their composed foods.
type CopyAs string

// Copy modes: leftover eats from the source plate's batches (R2 checked per
// cell), fresh cooks each copy at its own cell.
const (
	CopyLeftover CopyAs = "leftover"
	CopyFresh    CopyAs = "fresh"
)

// CopyMeal copies a plate (components and eaters) to each empty, non-past
// "plan" cell of to. A cell that fails (occupied, "none", past, or out of a
// batch's keeping time) fails the whole call, naming it.
func (s *Service) CopyMeal(ctx context.Context, plateID int64, to []CellKey, as CopyAs) (MealResult, error) {
	if as != CopyLeftover && as != CopyFresh {
		return MealResult{}, fmt.Errorf("%w: as must be leftover or fresh", domain.ErrInvalidInput)
	}
	if len(to) == 0 {
		return MealResult{}, fmt.Errorf("%w: no cells", domain.ErrInvalidInput)
	}
	var res MealResult
	undo, err := s.Recorded(ctx, func(ctx context.Context) error {
		p, err := s.plates.Get(ctx, plateID)
		if err != nil {
			return err
		}
		e, err := s.loadEnv(ctx)
		if err != nil {
			return err
		}
		today := dateOnly(s.now())
		j := newJournal()
		t := newTracker(j)
		ids := make([]int64, 0, len(to))
		var seen []CellKey
		for _, c := range to {
			c = CellKey{Date: dateOnly(c.Date), SlotID: c.SlotID}
			if slices.ContainsFunc(seen, c.Equal) {
				return fmt.Errorf("%w: cell %s slot %d twice", domain.ErrInvalidInput, c.Date.Format(dateLayout), c.SlotID)
			}
			seen = append(seen, c)
			if err := notPast(today, c); err != nil {
				return err
			}
			eaters := p.Eaters
			in := PlateInput{Date: c.Date, SlotID: c.SlotID, Eaters: &eaters, Components: copyComponents(p.Components, as)}
			id, err := s.createPlate(ctx, in, e, t, true, newOrigin(ctx), true)
			if err != nil {
				return fmt.Errorf("cell %s slot %d: %w", c.Date.Format(dateLayout), c.SlotID, err)
			}
			j.create(id)
			ids = append(ids, id)
		}
		if err := s.settle(ctx, t); err != nil {
			return err
		}
		if err := s.commit(ctx, j); err != nil {
			return err
		}
		res, err = s.mealResult(ctx, t, ids)
		return err
	})
	if err != nil {
		return MealResult{}, err
	}
	res.Undo = undo
	return res, nil
}

// copyComponents copies comps for a new plate; leftover keeps each composed
// component's batch.
func copyComponents(comps []PlateComponent, as CopyAs) []PlateComponent {
	out := make([]PlateComponent, len(comps))
	for i, c := range comps {
		out[i] = PlateComponent{FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, Position: c.Position, SortOrder: i}
		if as == CopyLeftover && c.CookSessionID != nil {
			id := *c.CookSessionID
			out[i].CookSessionID = &id
		}
	}
	return out
}
