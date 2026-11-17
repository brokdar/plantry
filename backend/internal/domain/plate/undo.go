package plate

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Undo is the revert token of a hand edit: the range of plate events it
// wrote and the availability cells it changed. FirstEventID is 0 when it
// wrote no event. POST /api/plan/revert takes it back.
type Undo struct {
	FirstEventID int64      `json:"first_event_id"`
	LastEventID  int64      `json:"last_event_id"`
	Cells        []CellUndo `json:"cells"`
}

// CellUndo is a cell's availability override before and after an edit (nil:
// the cell had none).
type CellUndo struct {
	Date   string        `json:"date"`
	SlotID int64         `json:"slot_id"`
	Before *CellOverride `json:"before"`
	After  *CellOverride `json:"after"`
}

// CellOverride is a stored availability override as an undo token carries
// it.
type CellOverride struct {
	Mode     string  `json:"mode"`
	PresetID *int64  `json:"preset_id,omitempty"`
	Note     *string `json:"note,omitempty"`
}

// Recorded runs fn in one unit of work and returns the range of plate events
// it wrote, so any mutation (a component edit from the plate sheet) yields
// an Undo. Events of one unit of work are contiguous: SQLite serialises
// writers.
func (s *Service) Recorded(ctx context.Context, fn func(ctx context.Context) error) (Undo, error) {
	u := Undo{Cells: []CellUndo{}}
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		mark, err := s.events.LastID(ctx)
		if err != nil {
			return err
		}
		if err := fn(ctx); err != nil {
			return err
		}
		last, err := s.events.LastID(ctx)
		if err != nil {
			return err
		}
		if last > mark {
			u.FirstEventID, u.LastEventID = mark+1, last
		}
		return nil
	})
	if err != nil {
		return Undo{}, err
	}
	return u, nil
}

// RevertRange reverts the plate events first..last of a hand edit (Revert),
// in one unit of work. It fails with domain.ErrPlanChangeStale, writing
// nothing, when a plate or session of the range has a later event, and with
// domain.ErrInvalidInput when the range holds no event.
func (s *Service) RevertRange(ctx context.Context, first, last int64) (RowSet, error) {
	if first <= 0 || last < first {
		return RowSet{}, fmt.Errorf("%w: no event range", domain.ErrInvalidInput)
	}
	var rows RowSet
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		events, err := s.events.Range(ctx, first, last)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return fmt.Errorf("%w: no events %d..%d", domain.ErrInvalidInput, first, last)
		}
		rows, err = s.RevertChecked(ctx, events)
		return err
	})
	if err != nil {
		return RowSet{}, err
	}
	return rows, nil
}

// RevertChecked is Revert of events (ascending) after checking that every
// event on their plates and sessions since the first of them is one of
// events: a later write would otherwise be lost (domain.ErrPlanChangeStale).
// The agent's change undo and the hand revert share it.
func (s *Service) RevertChecked(ctx context.Context, events []Event) (RowSet, error) {
	if len(events) == 0 {
		return RowSet{PlateIDs: []int64{}, SessionIDs: []int64{}}, nil
	}
	var rows RowSet
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		plateIDs, sessionIDs := EventRows(events)
		n, err := s.events.CountAfter(ctx, events[0].ID-1, plateIDs, sessionIDs)
		if err != nil {
			return err
		}
		if n != int64(len(events)) {
			return fmt.Errorf("%w: a row was written after the change", domain.ErrPlanChangeStale)
		}
		rows, err = s.Revert(ctx, events)
		if errors.Is(err, domain.ErrAIChangeStale) {
			return fmt.Errorf("%w: %v", domain.ErrPlanChangeStale, err)
		}
		return err
	})
	if err != nil {
		return RowSet{}, err
	}
	return rows, nil
}

// EventRows returns the plates and the sessions events are about, each
// ascending without duplicates. A plate event of a session edit names the
// plate only.
func EventRows(events []Event) (plateIDs, sessionIDs []int64) {
	plateIDs, sessionIDs = []int64{}, []int64{}
	for _, e := range events {
		switch {
		case e.PlateID != nil:
			plateIDs = append(plateIDs, *e.PlateID)
		case e.CookSessionID != nil:
			sessionIDs = append(sessionIDs, *e.CookSessionID)
		}
	}
	slices.Sort(plateIDs)
	slices.Sort(sessionIDs)
	return slices.Compact(plateIDs), slices.Compact(sessionIDs)
}
