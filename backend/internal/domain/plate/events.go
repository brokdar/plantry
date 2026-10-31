package plate

import (
	"bytes"
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
// when the mutation is done: the state before it of every plate and every
// cook session it changes (nil for one it creates), each in first-touch
// order. Ids are never reused (00030 made both tables AUTOINCREMENT), so an
// id names one plate or session for the life of the database.
type journal struct {
	created Action // the action of a created plate: create or restore

	plateIDs []int64
	plates   map[int64]*Plate

	sessionIDs []int64
	sessions   map[int64]*CookSession
	// editID is the session of an explicit session edit: it is set as
	// cook_session_id on the plate events, and its event is written even
	// when nothing about the session changed.
	editID int64
}

func newJournal() *journal {
	return &journal{created: ActionCreate, plates: map[int64]*Plate{}, sessions: map[int64]*CookSession{}}
}

// touch records p's state before the mutation changes it. Later touches of
// the same plate keep the first state.
func (j *journal) touch(p Plate) {
	if _, ok := j.plates[p.ID]; ok {
		return
	}
	j.plates[p.ID] = &p
	j.plateIDs = append(j.plateIDs, p.ID)
}

// create records a plate the mutation created.
func (j *journal) create(id int64) {
	if _, ok := j.plates[id]; ok {
		return
	}
	j.plates[id] = nil
	j.plateIDs = append(j.plateIDs, id)
}

// touchSession records sess's state before the mutation changes it. Later
// touches of the same session keep the first state.
func (j *journal) touchSession(sess CookSession) {
	if _, ok := j.sessions[sess.ID]; ok {
		return
	}
	j.sessions[sess.ID] = &sess
	j.sessionIDs = append(j.sessionIDs, sess.ID)
}

// createSession records a session the mutation created.
func (j *journal) createSession(id int64) {
	if _, ok := j.sessions[id]; ok {
		return
	}
	j.sessions[id] = nil
	j.sessionIDs = append(j.sessionIDs, id)
}

// editSession marks the mutation an explicit edit of sess (its state before).
func (j *journal) editSession(sess CookSession) {
	j.touchSession(sess)
	j.editID = sess.ID
}

// commit writes one event per touched plate (create/restore, update or
// delete by its state before and after), then one with plate_id NULL per
// touched session that changed (create, update or delete) and for the
// session of an explicit edit. It runs last in the mutation's unit of work.
func (s *Service) commit(ctx context.Context, j *journal) error {
	who := actor.From(ctx)
	var runID *int64
	if who.RunID != 0 {
		id := who.RunID
		runID = &id
	}
	at := s.now().UTC().Truncate(time.Second)
	var editID *int64
	if j.editID != 0 {
		id := j.editID
		editID = &id
	}
	for _, id := range j.plateIDs {
		before := j.plates[id]
		after, err := s.plates.Get(ctx, id)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			after = nil
		case err != nil:
			return err
		}
		e := Event{PlateID: &id, CookSessionID: editID, Actor: who.Kind, AIRunID: runID, At: at}
		e.Action = actionOf(before == nil, after == nil, j.created)
		if e.Action == "" {
			continue
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
	for _, id := range j.sessionIDs {
		before := j.sessions[id]
		after, err := s.sessions.GetSession(ctx, id)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			after = nil
		case err != nil:
			return err
		}
		e := Event{CookSessionID: &id, Actor: who.Kind, AIRunID: runID, At: at}
		e.Action = actionOf(before == nil, after == nil, ActionCreate)
		if e.Action == "" {
			continue
		}
		cellOf := after
		if cellOf == nil {
			cellOf = before
		}
		e.Date, e.SlotID = cellOf.CookDate, cellOf.CookSlotID
		if e.Before, err = sessionJSON(before); err != nil {
			return err
		}
		if e.After, err = sessionJSON(after); err != nil {
			return err
		}
		if id != j.editID && bytes.Equal(e.Before, e.After) {
			continue // tracked, but D11 left it as it was
		}
		if err := s.events.Append(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// actionOf is the action for a subject that did (not) exist before and
// after a mutation; "" when it existed at neither point.
func actionOf(noBefore, noAfter bool, created Action) Action {
	switch {
	case noBefore && noAfter:
		return ""
	case noBefore:
		return created
	case noAfter:
		return ActionDelete
	default:
		return ActionUpdate
	}
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
