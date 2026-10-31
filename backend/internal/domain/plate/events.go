package plate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
)

// Action is what a mutation did to a plate or session (D20).
type Action string

// Actions. Phase 3 adds its own.
const (
	ActionCreate  Action = "create"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionRestore Action = "restore"
)

// Event is one audit-trail row (D20). A plate event has PlateID and the
// plate's cell; a session event has PlateID nil, CookSessionID and the cook
// cell. CookSessionID is also set on the plate events of a session edit.
// Before and After are JSON snapshots, nil when the subject did not exist
// (create, restore) or no longer does (delete).
type Event struct {
	ID            int64
	PlateID       *int64
	CookSessionID *int64
	Date          time.Time
	SlotID        int64
	Action        Action
	Actor         actor.Kind
	AIRunID       *int64
	Before        json.RawMessage
	After         json.RawMessage
	At            time.Time
}

// EventLog is the port the audit trail is written to. Append joins the unit
// of work carried by ctx (D21).
type EventLog interface {
	Append(ctx context.Context, e Event) error
}

// eventSession is a session event's snapshot: the snapshot session shape with
// the session's id.
type eventSession struct {
	ID int64 `json:"id"`
	SnapshotSession
}

// journal records what one mutation touches so its events can be written
// when the mutation is done: each plate's state before it (nil for a plate
// the mutation creates) in first-touch order and, for a session edit, the
// session's state before it.
type journal struct {
	created Action // the action of a created plate: create or restore
	plates  []*touched
	// open indexes the plates by id until they are deleted: SQLite reuses
	// the highest plate id, so a plate created after a delete (Apply's
	// overwrite) may get the deleted plate's id and is a new entry.
	open map[int64]*touched

	sessionID     int64 // set for session edits
	sessionBefore *CookSession
}

// touched is one plate of a journal: its state before the mutation (nil when
// the mutation created it) and whether the mutation deleted it.
type touched struct {
	id      int64
	before  *Plate
	deleted bool
}

func newJournal() *journal {
	return &journal{created: ActionCreate, open: map[int64]*touched{}}
}

// touch records p's state before the mutation changes it. Later touches of
// the same plate keep the first state.
func (j *journal) touch(p Plate) {
	if _, ok := j.open[p.ID]; ok {
		return
	}
	j.add(&touched{id: p.ID, before: &p})
}

// create records a plate the mutation created.
func (j *journal) create(id int64) {
	if _, ok := j.open[id]; ok {
		return
	}
	j.add(&touched{id: id})
}

// remove records that the mutation deleted plate id.
func (j *journal) remove(id int64) {
	if t, ok := j.open[id]; ok {
		t.deleted = true
		delete(j.open, id)
	}
}

func (j *journal) add(t *touched) {
	j.plates = append(j.plates, t)
	j.open[t.id] = t
}

// editSession marks the mutation a session edit of sess (its state before).
func (j *journal) editSession(sess CookSession) {
	j.sessionID = sess.ID
	j.sessionBefore = &sess
}

// commit writes one event per touched plate (create/restore, update or
// delete by its state before and after), plus the session event of a session
// edit. It runs last in the mutation's unit of work.
func (s *Service) commit(ctx context.Context, j *journal) error {
	who := actor.From(ctx)
	var runID *int64
	if who.RunID != 0 {
		id := who.RunID
		runID = &id
	}
	at := s.now().UTC().Truncate(time.Second)
	var sessionID *int64
	if j.sessionID != 0 {
		id := j.sessionID
		sessionID = &id
	}
	for _, tp := range j.plates {
		id, before := tp.id, tp.before
		var after *Plate
		if !tp.deleted {
			p, err := s.plates.Get(ctx, id)
			switch {
			case errors.Is(err, domain.ErrNotFound):
			case err != nil:
				return err
			default:
				after = p
			}
		}
		var err error
		e := Event{PlateID: &id, CookSessionID: sessionID, Actor: who.Kind, AIRunID: runID, At: at}
		switch {
		case before == nil && after == nil:
			continue
		case before == nil:
			e.Action = j.created
		case after == nil:
			e.Action = ActionDelete
		default:
			e.Action = ActionUpdate
		}
		cellOf := after
		if cellOf == nil {
			cellOf = before
		}
		e.Date, e.SlotID = cellOf.Date, cellOf.SlotID
		if e.Before, err = plateJSON(before); err != nil {
			return err
		}
		if e.After, err = plateJSON(after); err != nil {
			return err
		}
		if err := s.events.Append(ctx, e); err != nil {
			return err
		}
	}
	if j.sessionBefore == nil {
		return nil
	}
	after, err := s.sessions.GetSession(ctx, j.sessionID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		after = nil
	case err != nil:
		return err
	}
	e := Event{CookSessionID: sessionID, Action: ActionUpdate, Actor: who.Kind, AIRunID: runID, At: at}
	cellOf := after
	if after == nil {
		e.Action = ActionDelete
		cellOf = j.sessionBefore
	}
	e.Date, e.SlotID = cellOf.CookDate, cellOf.CookSlotID
	if e.Before, err = sessionJSON(j.sessionBefore); err != nil {
		return err
	}
	if e.After, err = sessionJSON(after); err != nil {
		return err
	}
	return s.events.Append(ctx, e)
}

// plateJSON is p in the snapshot plate shape with its own id and every
// component's session by id; nil for nil.
func plateJSON(p *Plate) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	id := p.ID
	sp := SnapshotPlate{
		PlateID: &id, Date: p.DateString(), SlotID: p.SlotID, Eaters: p.Eaters, Note: p.Note,
		Components: make([]SnapshotComponent, len(p.Components)),
	}
	if p.Feedback != nil {
		sp.Feedback = &SnapshotFeedback{Status: p.Feedback.Status, Note: p.Feedback.Note, RatedAt: p.Feedback.RatedAt}
	}
	for i, c := range p.Components {
		sc := SnapshotComponent{FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, Grams: c.Grams, SortOrder: c.SortOrder}
		if c.CookSessionID != nil {
			sc.Session = &SessionLink{ID: *c.CookSessionID}
		}
		sp.Components[i] = sc
	}
	return marshalEvent(sp)
}

// sessionJSON is sess in the snapshot session shape with its id; nil for nil.
func sessionJSON(sess *CookSession) (json.RawMessage, error) {
	if sess == nil {
		return nil, nil
	}
	return marshalEvent(eventSession{ID: sess.ID, SnapshotSession: SnapshotSession{
		FoodID: sess.FoodID, CookDate: sess.CookDate.Format(dateLayout), CookSlotID: sess.CookSlotID,
		Portions: sess.Portions, FrozenPortions: sess.FrozenPortions, CookedAt: sess.CookedAt, Note: sess.Note,
	}})
}

func marshalEvent(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal plate event: %w", err)
	}
	return b, nil
}
