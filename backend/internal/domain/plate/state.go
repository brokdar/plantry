package plate

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
)

// origin is the state, AI run and reason a new row is written with.
type origin struct {
	state  State
	runID  *int64
	reason *string
}

// newOrigin is D2: a new row's state follows the actor in ctx, the user's
// rows are confirmed, the agent's and the generator's are drafts, and every
// row carries the actor's run.
func newOrigin(ctx context.Context) origin {
	who := actor.From(ctx)
	o := origin{state: StateConfirmed}
	if who.Kind != actor.User {
		o.state = StateDraft
	}
	if who.RunID != 0 {
		id := who.RunID
		o.runID = &id
	}
	return o
}

// freshOrigin is the origin of a fresh session at a plate in state st: it
// takes the plate's state (D3) and the actor's run.
func freshOrigin(ctx context.Context, st State) origin {
	o := newOrigin(ctx)
	o.state = st
	return o
}

// snapshotOrigin is the origin of a row Apply writes from a snapshot (D5):
// with restore the snapshot's state and run (an empty state follows D2);
// otherwise D2. The reason is the snapshot's either way.
func snapshotOrigin(ctx context.Context, st State, runID *int64, reason *string, restore bool) origin {
	o := newOrigin(ctx)
	if restore && st != "" {
		o.state, o.runID = st, runID
	}
	o.reason = reason
	return o
}

// Confirm confirms the batch closure of seed (D3) and returns the closure.
// Each row it changes gets one confirm event; Confirm leaves run outcomes
// alone (D25).
func (s *Service) Confirm(ctx context.Context, seed RowSet) (RowSet, error) {
	return s.setClosureState(ctx, seed, StateConfirmed, ActionConfirm)
}

// Unlock returns the batch closure of seed to draft and returns the closure.
// Each row it changes gets one unlock event.
func (s *Service) Unlock(ctx context.Context, seed RowSet) (RowSet, error) {
	return s.setClosureState(ctx, seed, StateDraft, ActionUnlock)
}

func (s *Service) setClosureState(ctx context.Context, seed RowSet, to State, action Action) (RowSet, error) {
	seed = uniqueRows(seed)
	if seed.Empty() {
		return RowSet{}, fmt.Errorf("%w: plate or session required", domain.ErrInvalidInput)
	}
	var out RowSet
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		plates, sessions, err := s.loadRows(ctx, seed)
		if err != nil {
			return err
		}
		if len(plates) != len(seed.PlateIDs) || len(sessions) != len(seed.SessionIDs) {
			return fmt.Errorf("%w: plate or session", domain.ErrNotFound)
		}
		lo, hi := rowsSpan(plates, sessions)
		c, rows, err := s.loadClosure(ctx, lo, hi, func([]Plate, []CookSession) RowSet { return seed })
		if err != nil {
			return err
		}
		j := newJournal()
		j.changed = action
		if _, err := s.setState(ctx, j, c, rows, to); err != nil {
			return err
		}
		out = c
		return s.commit(ctx, j)
	})
	if err != nil {
		return RowSet{}, err
	}
	return out, nil
}

// Accept confirms every draft row of the window closure of [from, to] in one
// unit of work (D4): the draft plates dated in it and the draft sessions
// cooking in it, closed under links (so a draft batch cooked before from
// that feeds the window is included). It returns the rows it confirmed,
// which Unaccept takes back, writes one accept event per row and moves the
// outcome of every run of those rows (D25). Without drafts it fails with
// ErrNothingToAccept.
func (s *Service) Accept(ctx context.Context, from, to time.Time) (RowSet, error) {
	if err := validateRange(from, to); err != nil {
		return RowSet{}, err
	}
	var out RowSet
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		c, rows, err := s.loadDrafts(ctx, dateOnly(from), dateOnly(to))
		if err != nil {
			return err
		}
		j := newJournal()
		j.changed = ActionAccept
		if out, err = s.setState(ctx, j, c, rows, StateConfirmed); err != nil {
			return err
		}
		if err := s.recordOutcomes(ctx, OutcomeAccepted, out, rows); err != nil {
			return err
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return RowSet{}, err
	}
	return out, nil
}

// Unaccept returns exactly the rows of an Accept to draft (its undo) and
// resets their runs' outcome (D25). Every row must exist and be confirmed,
// else ErrStateMismatch and nothing changes. Each row gets one unlock event.
func (s *Service) Unaccept(ctx context.Context, rows RowSet) (RowSet, error) {
	rows = uniqueRows(rows)
	if rows.Empty() {
		return RowSet{}, fmt.Errorf("%w: plate_ids or session_ids required", domain.ErrInvalidInput)
	}
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		plates, sessions, err := s.loadRows(ctx, rows)
		if err != nil {
			return err
		}
		if len(plates) != len(rows.PlateIDs) || len(sessions) != len(rows.SessionIDs) {
			return fmt.Errorf("%w: a row no longer exists", domain.ErrStateMismatch)
		}
		loaded := closureRows{plates: map[int64]Plate{}, sessions: map[int64]CookSession{}}
		for _, p := range plates {
			if p.State != StateConfirmed {
				return fmt.Errorf("%w: plate %d", domain.ErrStateMismatch, p.ID)
			}
			loaded.plates[p.ID] = p
		}
		for _, sess := range sessions {
			if sess.State != StateConfirmed {
				return fmt.Errorf("%w: session %d", domain.ErrStateMismatch, sess.ID)
			}
			loaded.sessions[sess.ID] = sess
		}
		j := newJournal()
		j.changed = ActionUnlock
		if _, err := s.setState(ctx, j, rows, loaded, StateDraft); err != nil {
			return err
		}
		if err := s.recordOutcomes(ctx, OutcomeReset, rows, loaded); err != nil {
			return err
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return RowSet{}, err
	}
	return rows, nil
}

// Discard deletes every draft row of the window closure of [from, to] (as
// Accept picks them) in one unit of work and returns the snapshot that
// restores them as drafts with their runs and reasons (D4, D5). Each row
// gets one discard event and every run of those rows is marked discarded
// (D25). Without drafts it fails with ErrNothingToAccept.
func (s *Service) Discard(ctx context.Context, from, to time.Time) (PlanSnapshot, error) {
	if err := validateRange(from, to); err != nil {
		return PlanSnapshot{}, err
	}
	var snap PlanSnapshot
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		c, rows, err := s.loadDrafts(ctx, dateOnly(from), dateOnly(to))
		if err != nil {
			return err
		}
		drafts := RowSet{PlateIDs: []int64{}, SessionIDs: []int64{}}
		var plates []Plate
		for _, id := range c.PlateIDs {
			if p := rows.plates[id]; p.State == StateDraft {
				plates = append(plates, p)
				drafts.PlateIDs = append(drafts.PlateIDs, id)
			}
		}
		for _, id := range c.SessionIDs {
			if rows.sessions[id].State == StateDraft {
				drafts.SessionIDs = append(drafts.SessionIDs, id)
			}
		}
		order, err := s.slotOrder(ctx)
		if err != nil {
			return err
		}
		sortPlates(plates, order)
		j := newJournal()
		j.deleted = ActionDiscard
		if snap, err = s.remove(ctx, plates, allComponents, false, &refAlloc{}, nil, nil, j); err != nil {
			return err
		}
		if err := s.recordOutcomes(ctx, OutcomeDiscarded, drafts, rows); err != nil {
			return err
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return PlanSnapshot{}, err
	}
	return snap, nil
}

// DraftClosure returns the window closure of [from, to] as Accept picks it
// (D4) and the plates it holds; an empty set when there are no drafts. It
// writes nothing. The generator replaces it (D6).
func (s *Service) DraftClosure(ctx context.Context, from, to time.Time) (RowSet, []Plate, error) {
	var out RowSet
	var plates []Plate
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		c, rows, err := s.loadDrafts(ctx, dateOnly(from), dateOnly(to))
		if errors.Is(err, domain.ErrNothingToAccept) {
			return nil
		}
		if err != nil {
			return err
		}
		out, plates = c, closurePlates(c, rows)
		return nil
	})
	return out, plates, err
}

// PlateClosure returns the batch closure of a plate (D3) and the plates it
// holds. It writes nothing.
func (s *Service) PlateClosure(ctx context.Context, plateID int64) (RowSet, []Plate, error) {
	var out RowSet
	var plates []Plate
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		p, err := s.plates.Get(ctx, plateID)
		if err != nil {
			return err
		}
		seed := RowSet{PlateIDs: []int64{p.ID}}
		lo, hi := rowsSpan([]Plate{*p}, nil)
		c, rows, err := s.loadClosure(ctx, lo, hi, func([]Plate, []CookSession) RowSet { return seed })
		if err != nil {
			return err
		}
		out, plates = c, closurePlates(c, rows)
		return nil
	})
	return out, plates, err
}

func closurePlates(c RowSet, rows closureRows) []Plate {
	out := make([]Plate, 0, len(c.PlateIDs))
	for _, id := range c.PlateIDs {
		out = append(out, rows.plates[id])
	}
	return out
}

// RemoveDrafts deletes draft rows in one unit of work, the delete half of
// the generator's replace (D10): every plate of rows.PlateIDs, and every
// component eating from a session of rows.SessionIDs, so the session goes
// too while a plate keeping other components stays. A row that is gone or
// not a draft fails with ErrStateMismatch and nothing is removed. It returns
// the snapshot that restores what it removed.
func (s *Service) RemoveDrafts(ctx context.Context, rows RowSet) (PlanSnapshot, error) {
	rows = uniqueRows(rows)
	var snap PlanSnapshot
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		sessions, err := s.sessions.ListSessionsByIDs(ctx, rows.SessionIDs)
		if err != nil {
			return err
		}
		if len(sessions) != len(rows.SessionIDs) {
			return fmt.Errorf("%w: a session is gone", domain.ErrStateMismatch)
		}
		dropSession := make(map[int64]bool, len(sessions))
		for _, sess := range sessions {
			if sess.State != StateDraft {
				return fmt.Errorf("%w: session %d is confirmed", domain.ErrStateMismatch, sess.ID)
			}
			dropSession[sess.ID] = true
		}
		whole := make(map[int64]bool, len(rows.PlateIDs))
		for _, id := range rows.PlateIDs {
			whole[id] = true
		}
		linked, err := s.sessions.PlateIDsOfSessions(ctx, rows.SessionIDs)
		if err != nil {
			return err
		}
		ids := append(slices.Clone(rows.PlateIDs), linked...)
		plates, err := s.plates.ListByIDs(ctx, uniqueIDs(ids))
		if err != nil {
			return err
		}
		found := 0
		for _, p := range plates {
			if !whole[p.ID] {
				continue
			}
			found++
			if p.State != StateDraft {
				return fmt.Errorf("%w: plate %d is confirmed", domain.ErrStateMismatch, p.ID)
			}
		}
		if found != len(rows.PlateIDs) {
			return fmt.Errorf("%w: a plate is gone", domain.ErrStateMismatch)
		}
		order, err := s.slotOrder(ctx)
		if err != nil {
			return err
		}
		sortPlates(plates, order)
		pick := func(pc PlateComponent) bool {
			return whole[pc.PlateID] || pc.CookSessionID != nil && dropSession[*pc.CookSessionID]
		}
		j := newJournal()
		if snap, err = s.remove(ctx, plates, pick, true, &refAlloc{}, nil, nil, j); err != nil {
			return err
		}
		return s.commit(ctx, j)
	})
	if err != nil {
		return PlanSnapshot{}, err
	}
	return snap, nil
}

// loadDrafts loads the window closure of [from, to]: the draft plates dated
// in it and the draft sessions cooking in it, closed under links. Without
// drafts it fails with ErrNothingToAccept.
func (s *Service) loadDrafts(ctx context.Context, from, to time.Time) (RowSet, closureRows, error) {
	c, rows, err := s.loadClosure(ctx, from, to, func(plates []Plate, sessions []CookSession) RowSet {
		seed := RowSet{}
		for _, p := range plates {
			if p.State == StateDraft {
				seed.PlateIDs = append(seed.PlateIDs, p.ID)
			}
		}
		for _, sess := range sessions {
			if sess.State == StateDraft && !sess.CookDate.Before(from) && !sess.CookDate.After(to) {
				seed.SessionIDs = append(seed.SessionIDs, sess.ID)
			}
		}
		return seed
	})
	if err != nil {
		return RowSet{}, closureRows{}, err
	}
	if c.Empty() {
		return RowSet{}, closureRows{}, fmt.Errorf("%w: %s to %s", domain.ErrNothingToAccept, from.Format(dateLayout), to.Format(dateLayout))
	}
	return c, rows, nil
}

// closureRows are the plates and sessions loadClosure read, by id.
type closureRows struct {
	plates   map[int64]Plate
	sessions map[int64]CookSession
}

// loadClosure returns the batch closure of the seeds that seeds picks from
// the rows of [lo, hi], with the rows it read. It reads the plates and the
// sessions of a range (a bounded number of queries per pass) and widens the
// range until it holds every link and the cook cell of every session in the
// closure, so Closure sees every plate linked to one of them.
func (s *Service) loadClosure(ctx context.Context, lo, hi time.Time, seeds func([]Plate, []CookSession) RowSet) (RowSet, closureRows, error) {
	var seed RowSet
	for pass := 0; ; pass++ {
		plates, err := s.plates.ListByDateRange(ctx, lo, hi)
		if err != nil {
			return RowSet{}, closureRows{}, err
		}
		sessions, links, err := s.sessions.ListInRange(ctx, lo, hi)
		if err != nil {
			return RowSet{}, closureRows{}, err
		}
		if pass == 0 {
			seed = seeds(plates, sessions)
		}
		c := Closure(plates, seed.PlateIDs, seed.SessionIDs)
		rows := closureRows{plates: make(map[int64]Plate, len(plates)), sessions: make(map[int64]CookSession, len(sessions))}
		for _, p := range plates {
			rows.plates[p.ID] = p
		}
		for _, sess := range sessions {
			rows.sessions[sess.ID] = sess
		}
		nlo, nhi := lo, hi
		widen := func(d time.Time) {
			if d.Before(nlo) {
				nlo = d
			}
			if d.After(nhi) {
				nhi = d
			}
		}
		for _, sid := range c.SessionIDs {
			sess, ok := rows.sessions[sid]
			if !ok {
				return RowSet{}, closureRows{}, fmt.Errorf("closure: session %d is outside %s to %s", sid, lo.Format(dateLayout), hi.Format(dateLayout))
			}
			widen(sess.CookDate)
			for _, l := range links[sid] {
				widen(l.Cell.Date)
			}
		}
		if nlo.Equal(lo) && nhi.Equal(hi) {
			for _, pid := range c.PlateIDs {
				if _, ok := rows.plates[pid]; !ok {
					return RowSet{}, closureRows{}, fmt.Errorf("closure: plate %d is outside %s to %s", pid, lo.Format(dateLayout), hi.Format(dateLayout))
				}
			}
			return c, rows, nil
		}
		lo, hi = nlo, nhi
	}
}

// setState sets the rows of c not yet in state to to, recording each in j,
// and returns the rows it changed.
func (s *Service) setState(ctx context.Context, j *journal, c RowSet, rows closureRows, to State) (RowSet, error) {
	changed := RowSet{PlateIDs: []int64{}, SessionIDs: []int64{}}
	for _, id := range c.PlateIDs {
		if p := rows.plates[id]; p.State != to {
			j.touch(p)
			changed.PlateIDs = append(changed.PlateIDs, id)
		}
	}
	for _, id := range c.SessionIDs {
		if sess := rows.sessions[id]; sess.State != to {
			j.touchSession(sess)
			changed.SessionIDs = append(changed.SessionIDs, id)
		}
	}
	if err := s.plates.SetState(ctx, changed.PlateIDs, to); err != nil {
		return RowSet{}, err
	}
	if err := s.sessions.SetSessionsState(ctx, changed.SessionIDs, to); err != nil {
		return RowSet{}, err
	}
	return changed, nil
}

// joinLink applies D3 to plate id, in state st, that now eats from linked
// (nil: no existing session). When their states differ, confirmed wins: the
// plate is confirmed if it was a draft, and settle unifies the merged
// closure. It returns the plate's state.
func (s *Service) joinLink(ctx context.Context, t *tracker, id int64, st State, linked *CookSession) (State, error) {
	if linked == nil || linked.State == st {
		return st, nil
	}
	if st != StateConfirmed {
		if err := s.plates.SetState(ctx, []int64{id}, StateConfirmed); err != nil {
			return st, err
		}
	}
	t.unify = append(t.unify, id)
	return StateConfirmed, nil
}

// unify gives the batch closure of every plate in t.unify that plate's state
// (D3), recording the rows it changes in t.j. A plate already reached by an
// earlier closure is skipped.
func (s *Service) unify(ctx context.Context, t *tracker) error {
	done := map[int64]bool{}
	for _, id := range t.unify {
		if done[id] {
			continue
		}
		p, err := s.plates.Get(ctx, id)
		if err != nil {
			return err
		}
		c, rows, err := s.loadClosure(ctx, p.Date, p.Date, func([]Plate, []CookSession) RowSet {
			return RowSet{PlateIDs: []int64{id}}
		})
		if err != nil {
			return err
		}
		for _, pid := range c.PlateIDs {
			done[pid] = true
		}
		if _, err := s.setState(ctx, t.j, c, rows, p.State); err != nil {
			return err
		}
	}
	return nil
}

// recordOutcomes records ev once for every distinct AI run of the rows of c
// (D25), in run order: the runs that wrote them and the agent runs that
// edited them (D11), so a chat that only edited a generated draft is accepted
// or discarded with it.
func (s *Service) recordOutcomes(ctx context.Context, ev OutcomeEvent, c RowSet, rows closureRows) error {
	seen := map[int64]bool{}
	for _, id := range c.PlateIDs {
		if run := rows.plates[id].AIRunID; run != nil {
			seen[*run] = true
		}
	}
	for _, id := range c.SessionIDs {
		if run := rows.sessions[id].AIRunID; run != nil {
			seen[*run] = true
		}
	}
	edited, err := s.outcomes.AgentRuns(ctx, c)
	if err != nil {
		return err
	}
	for _, run := range edited {
		seen[run] = true
	}
	for _, run := range sortedKeys(seen) {
		if err := s.outcomes.Record(ctx, run, ev); err != nil {
			return err
		}
	}
	return nil
}

// loadRows reads the plates and sessions of r that exist.
func (s *Service) loadRows(ctx context.Context, r RowSet) ([]Plate, []CookSession, error) {
	plates, err := s.plates.ListByIDs(ctx, r.PlateIDs)
	if err != nil {
		return nil, nil, err
	}
	sessions, err := s.sessions.ListSessionsByIDs(ctx, r.SessionIDs)
	if err != nil {
		return nil, nil, err
	}
	return plates, sessions, nil
}

// rowsSpan is the date range of the plates' dates and the sessions' cook
// dates; at least one row is given.
func rowsSpan(plates []Plate, sessions []CookSession) (lo, hi time.Time) {
	dates := make([]time.Time, 0, len(plates)+len(sessions))
	for _, p := range plates {
		dates = append(dates, p.Date)
	}
	for _, sess := range sessions {
		dates = append(dates, sess.CookDate)
	}
	return slices.MinFunc(dates, time.Time.Compare), slices.MaxFunc(dates, time.Time.Compare)
}

// uniqueRows sorts r's ids and drops duplicates and non-positive ids.
func uniqueRows(r RowSet) RowSet {
	return RowSet{PlateIDs: uniqueIDs(r.PlateIDs), SessionIDs: uniqueIDs(r.SessionIDs)}
}

func uniqueIDs(ids []int64) []int64 {
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id > 0 {
			set[id] = true
		}
	}
	return sortedKeys(set)
}
