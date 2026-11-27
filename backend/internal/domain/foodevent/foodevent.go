// Package foodevent is the change log of food rows other than plates (VAR-5):
// an event per recipe, attribute, taste item or substitution change, and a
// generic Undo that mirrors the plan change rule (plate.Service.RevertChecked).
// The kinds' own writes live elsewhere: a producer calls Record inside its
// unit of work, and registers a Restorer so Revert can put a row back.
package foodevent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
)

// Kind is the kind of row a change is about.
type Kind string

// Kinds.
const (
	KindRecipe       Kind = "recipe"
	KindAttribute    Kind = "attribute"
	KindTasteItem    Kind = "taste_item"
	KindSubstitution Kind = "substitution"
	// KindRecipeContent is an assistant's recipe change: a filled variant
	// (create) or an in-place edit (update) (VAR-1, VAR-2).
	KindRecipeContent Kind = "recipe_content"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	switch k {
	case KindRecipe, KindAttribute, KindTasteItem, KindSubstitution, KindRecipeContent:
		return true
	}
	return false
}

// Action is what a change did to its subject. Undo is every write of Revert:
// it is not a user change, and it makes the reverted range stale.
type Action string

// Actions.
const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionUndo   Action = "undo"
)

// Event is one change-log row. Before and After are JSON snapshots, nil when
// the subject did not exist (create) or no longer does (delete).
type Event struct {
	ID        int64
	Kind      Kind
	SubjectID int64
	Action    Action
	Actor     actor.Kind
	Before    json.RawMessage
	After     json.RawMessage
	At        time.Time
}

// Log is the port the change log is written to. Append joins the unit of
// work carried by ctx.
type Log interface {
	Append(ctx context.Context, e Event) error
	// LastID is the highest event id, 0 when there is none.
	LastID(ctx context.Context) (int64, error)
	// Range returns the events with first <= id <= last, ascending.
	Range(ctx context.Context, first, last int64) ([]Event, error)
}

// Restorer puts one kind's row back. before is the row's snapshot before the
// change being undone, nil when the change created it (the row is removed).
// It runs in the Revert's unit of work.
type Restorer interface {
	Restore(ctx context.Context, subjectID int64, before json.RawMessage) error
}

// Verifier is an optional Restorer extension: Verify runs before Restore and
// returns domain.ErrPlanChangeStale when the row no longer matches after, the
// snapshot the change being undone wrote (the row changed outside the log).
type Verifier interface {
	Verify(ctx context.Context, subjectID int64, after json.RawMessage) error
}

// Undo is the revert token of a change: the range of events it wrote.
// FirstEventID is 0 when it wrote none.
type Undo struct {
	FirstEventID int64 `json:"first_event_id"`
	LastEventID  int64 `json:"last_event_id"`
}

// Subject names a reverted row.
type Subject struct {
	Kind      Kind  `json:"kind"`
	SubjectID int64 `json:"subject_id"`
}

// Service records and reverts food events.
type Service struct {
	tx        domain.Transactor
	log       Log
	restorers map[Kind]Restorer
	now       func() time.Time
}

// NewService creates a Service. now nil means time.Now.
func NewService(tx domain.Transactor, log Log, restorers map[Kind]Restorer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{tx: tx, log: log, restorers: restorers, now: now}
}

// Record appends e (its actor from ctx, its time from the clock), joining the
// unit of work in ctx.
func (s *Service) Record(ctx context.Context, e Event) error {
	if !e.Kind.Valid() {
		return fmt.Errorf("%w: food event kind %q", domain.ErrInvalidInput, e.Kind)
	}
	e.Actor = actor.From(ctx).Kind
	e.At = s.now()
	return s.log.Append(ctx, e)
}

// Recorded runs fn in one unit of work and returns the range of events it
// wrote. Events of one unit of work are contiguous: SQLite serialises writers.
func (s *Service) Recorded(ctx context.Context, fn func(ctx context.Context) error) (Undo, error) {
	var u Undo
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		mark, err := s.log.LastID(ctx)
		if err != nil {
			return err
		}
		if err := fn(ctx); err != nil {
			return err
		}
		last, err := s.log.LastID(ctx)
		if err != nil {
			return err
		}
		if last > mark {
			u = Undo{FirstEventID: mark + 1, LastEventID: last}
		}
		return nil
	})
	if err != nil {
		return Undo{}, err
	}
	return u, nil
}

// Revert reverts the events of u, newest first, in one unit of work and
// records an undo event for each. It fails with domain.ErrPlanChangeStale,
// writing nothing, when a row of the range has a later event that is not in
// the range (including an earlier Revert of it), and with
// domain.ErrInvalidInput when the range holds no event or a kind has no
// Restorer.
func (s *Service) Revert(ctx context.Context, u Undo) ([]Subject, error) {
	if u.FirstEventID <= 0 || u.LastEventID < u.FirstEventID {
		return nil, fmt.Errorf("%w: no event range", domain.ErrInvalidInput)
	}
	subjects := []Subject{}
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		events, err := s.log.Range(ctx, u.FirstEventID, u.LastEventID)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return fmt.Errorf("%w: no events %d..%d", domain.ErrInvalidInput, u.FirstEventID, u.LastEventID)
		}
		for _, e := range events {
			if s.restorers[e.Kind] == nil {
				return fmt.Errorf("%w: no undo for food event kind %q", domain.ErrInvalidInput, e.Kind)
			}
		}
		last, err := s.log.LastID(ctx)
		if err != nil {
			return err
		}
		later, err := s.log.Range(ctx, u.LastEventID+1, last)
		if err != nil {
			return err
		}
		touched := make(map[Subject]bool, len(events))
		for _, e := range events {
			touched[Subject{e.Kind, e.SubjectID}] = true
		}
		for _, e := range later {
			if touched[Subject{e.Kind, e.SubjectID}] {
				return fmt.Errorf("%w: a row was written after the change", domain.ErrPlanChangeStale)
			}
		}
		for _, e := range slices.Backward(events) {
			if v, ok := s.restorers[e.Kind].(Verifier); ok {
				if err := v.Verify(ctx, e.SubjectID, e.After); err != nil {
					return err
				}
			}
			if err := s.restorers[e.Kind].Restore(ctx, e.SubjectID, e.Before); err != nil {
				return err
			}
			if err := s.Record(ctx, Event{Kind: e.Kind, SubjectID: e.SubjectID, Action: ActionUndo, Before: e.After, After: e.Before}); err != nil {
				return err
			}
			if sub := (Subject{e.Kind, e.SubjectID}); !slices.Contains(subjects, sub) {
				subjects = append(subjects, sub)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return subjects, nil
}
