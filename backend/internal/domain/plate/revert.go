package plate

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Revert puts every plate and cook session that events touched back as it
// was before the first of them, in one unit of work (Phase 4 D13, the undo
// of an agent change). A row the events created is deleted; any other row is
// written back from its before snapshot with its own id, state, run and
// reason. Nothing is re-derived: D11 does not run, because the snapshots
// already hold the sizes that belonged together. Revert writes its own
// events (update, delete, or restore for a row it brings back) as the actor
// in ctx and returns the plates and sessions it wrote.
//
// The caller checks that no row was written after events (an undo that is
// not stale). A session the events created that still has links outside
// them fails with domain.ErrAIChangeStale.
func (s *Service) Revert(ctx context.Context, events []Event) (RowSet, error) {
	plateIDs, plateBefore, sessionIDs, sessionBefore := firstBefore(events)
	var out RowSet
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		j := newJournal()
		j.created = ActionRestore
		current, err := s.plates.ListByIDs(ctx, plateIDs)
		if err != nil {
			return err
		}
		curPlates := make(map[int64]Plate, len(current))
		for _, p := range current {
			curPlates[p.ID] = p
		}
		sessions, err := s.sessions.ListSessionsByIDs(ctx, sessionIDs)
		if err != nil {
			return err
		}
		curSessions := make(map[int64]CookSession, len(sessions))
		for _, sess := range sessions {
			curSessions[sess.ID] = sess
		}

		// Sessions first, so the plates written back can link them.
		for _, id := range sessionIDs {
			if sessionBefore[id] == nil {
				continue
			}
			sess, err := decodeSession(sessionBefore[id])
			if err != nil {
				return err
			}
			if cur, ok := curSessions[id]; ok {
				j.touchSession(cur)
			} else {
				j.createSession(id)
			}
			if err := s.sessions.UpsertSession(ctx, sess); err != nil {
				return err
			}
		}
		// Every touched plate goes, then the ones that existed come back, so
		// a cell the change filled is free for the plate it replaced.
		for _, id := range plateIDs {
			cur, ok := curPlates[id]
			if !ok {
				continue
			}
			j.touch(cur)
			if err := s.plates.Delete(ctx, id); err != nil {
				return err
			}
		}
		for _, id := range plateIDs {
			if plateBefore[id] == nil {
				continue
			}
			p, err := decodePlate(plateBefore[id])
			if err != nil {
				return err
			}
			if _, ok := curPlates[id]; !ok {
				j.create(id)
			}
			if err := s.plates.Reinsert(ctx, &p); err != nil {
				return err
			}
			if p.Feedback != nil {
				if err := s.plates.RestoreFeedback(ctx, id, *p.Feedback); err != nil {
					return err
				}
			}
		}
		// Sessions the change created go once nothing eats from them.
		for _, id := range sessionIDs {
			cur, ok := curSessions[id]
			if sessionBefore[id] != nil || !ok {
				continue
			}
			links, err := s.sessions.ListLinks(ctx, id)
			if err != nil {
				return err
			}
			if len(links) > 0 {
				return fmt.Errorf("%w: cook session %d is eaten from by other plates", domain.ErrAIChangeStale, id)
			}
			j.touchSession(cur)
			if err := s.sessions.DeleteSession(ctx, id); err != nil {
				return err
			}
		}
		out = RowSet{PlateIDs: slices.Clone(plateIDs), SessionIDs: slices.Clone(sessionIDs)}
		return s.commit(ctx, j)
	})
	if err != nil {
		return RowSet{}, err
	}
	slices.Sort(out.PlateIDs)
	slices.Sort(out.SessionIDs)
	return out, nil
}

// firstBefore returns the plates and sessions of events in first-touch order
// with the before snapshot of each one's first event (nil: the events
// created it).
func firstBefore(events []Event) (plateIDs []int64, plates map[int64]json.RawMessage, sessionIDs []int64, sessions map[int64]json.RawMessage) {
	sorted := slices.Clone(events)
	slices.SortFunc(sorted, func(a, b Event) int { return cmp.Compare(a.ID, b.ID) })
	plates, sessions = map[int64]json.RawMessage{}, map[int64]json.RawMessage{}
	for _, e := range sorted {
		switch {
		case e.PlateID != nil:
			if _, ok := plates[*e.PlateID]; !ok {
				plates[*e.PlateID] = e.Before
				plateIDs = append(plateIDs, *e.PlateID)
			}
		case e.CookSessionID != nil:
			if _, ok := sessions[*e.CookSessionID]; !ok {
				sessions[*e.CookSessionID] = e.Before
				sessionIDs = append(sessionIDs, *e.CookSessionID)
			}
		}
	}
	return plateIDs, plates, sessionIDs, sessions
}

// decodePlate reads a plate event snapshot (plateJSON) back into a plate.
func decodePlate(raw json.RawMessage) (Plate, error) {
	var sp SnapshotPlate
	if err := json.Unmarshal(raw, &sp); err != nil {
		return Plate{}, fmt.Errorf("decode plate snapshot: %w", err)
	}
	if sp.PlateID == nil {
		return Plate{}, fmt.Errorf("decode plate snapshot: no plate_id")
	}
	d, err := time.Parse(dateLayout, sp.Date)
	if err != nil {
		return Plate{}, fmt.Errorf("decode plate snapshot date: %w", err)
	}
	p := Plate{
		ID: *sp.PlateID, Date: d, SlotID: sp.SlotID, Eaters: sp.Eaters, Note: sp.Note,
		State: sp.State, AIRunID: sp.AIRunID, Reason: sp.Reason,
		Components: make([]PlateComponent, len(sp.Components)),
	}
	if sp.Feedback != nil {
		p.Feedback = &Feedback{Status: sp.Feedback.Status, Note: sp.Feedback.Note, RatedAt: sp.Feedback.RatedAt}
	}
	for i, c := range sp.Components {
		pc := PlateComponent{FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, Grams: c.Grams, SortOrder: c.SortOrder}
		if c.Session != nil {
			id := c.Session.ID
			pc.CookSessionID = &id
		}
		p.Components[i] = pc
	}
	return p, nil
}

// decodeSession reads a session event snapshot (sessionJSON) back into a
// session.
func decodeSession(raw json.RawMessage) (CookSession, error) {
	var es eventSession
	if err := json.Unmarshal(raw, &es); err != nil {
		return CookSession{}, fmt.Errorf("decode session snapshot: %w", err)
	}
	d, err := time.Parse(dateLayout, es.CookDate)
	if err != nil {
		return CookSession{}, fmt.Errorf("decode session snapshot date: %w", err)
	}
	return CookSession{
		ID: es.ID, FoodID: es.FoodID, CookDate: d, CookSlotID: es.CookSlotID,
		Portions: es.Portions, FrozenPortions: es.FrozenPortions, CookedAt: es.CookedAt, Note: es.Note,
		State: es.State, AIRunID: es.AIRunID, Reason: es.Reason,
	}, nil
}
