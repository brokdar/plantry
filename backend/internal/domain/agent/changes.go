package agent

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Change is one plan-writing tool call (Phase 4 D13): the line the assistant
// shows and the unit of undo. Its plate events are those of RunIDs (the chat
// run, plus a regenerate run the call waited for) with ids in
// [FirstEventID, LastEventID].
type Change struct {
	ID      int64
	RunID   int64
	StepSeq int
	Tool    string
	// TextKey and Params render the line (assistant.change.<text_key>);
	// params carry display names and dates (§3.1).
	TextKey    string
	Params     map[string]any
	RunIDs     []int64
	Dates      []string
	PlateIDs   []int64
	SessionIDs []int64
	// FirstEventID and LastEventID are nil when the call wrote no plate
	// event.
	FirstEventID *int64
	LastEventID  *int64
	// CellBefore is the cell set_availability changed, as it was before.
	CellBefore *CellBefore
	CreatedAt  time.Time
	UndoneAt   *time.Time
}

// Undoable reports whether the change wrote anything undo can put back.
func (c Change) Undoable() bool { return c.FirstEventID != nil || c.CellBefore != nil }

// Payload is the change as the chat stream and the conversation carry it.
func (c Change) Payload() llm.ChangePayload {
	return llm.ChangePayload{
		ID: c.ID, RunID: c.RunID, Tool: c.Tool, TextKey: c.TextKey, Params: c.Params,
		Dates: c.Dates, PlateIDs: c.PlateIDs, SessionIDs: c.SessionIDs, Undoable: c.Undoable(),
	}
}

// CellBefore is a cell's availability override before set_availability;
// Override nil means the cell had none.
type CellBefore struct {
	Date     string        `json:"date"`
	SlotID   int64         `json:"slot_id"`
	Override *CellOverride `json:"override"`
}

// CellOverride is a stored availability override.
type CellOverride struct {
	CellMode string  `json:"mode"`
	PresetID *int64  `json:"preset_id,omitempty"`
	Note     *string `json:"note,omitempty"`
}

// ChangeStore is the port for ai_changes and the plate events they span.
// Every method joins the unit of work carried by ctx.
type ChangeStore interface {
	// LastEventID is the highest plate event id, 0 when there is none.
	LastEventID(ctx context.Context) (int64, error)
	// RunEvents returns the plate events of runIDs with afterID < id <=
	// throughID, ascending.
	RunEvents(ctx context.Context, afterID, throughID int64, runIDs []int64) ([]plate.Event, error)
	// LaterEvents counts the events after afterID of the plates or sessions.
	LaterEvents(ctx context.Context, afterID int64, plateIDs, sessionIDs []int64) (int64, error)
	// CreateChange inserts c and sets its ID and CreatedAt.
	CreateChange(ctx context.Context, c *Change) error
	// GetChange returns the change or an error wrapping domain.ErrNotFound.
	GetChange(ctx context.Context, id int64) (*Change, error)
	// ConversationChanges lists the changes of a conversation's runs by id.
	ConversationChanges(ctx context.Context, conversationID int64) ([]Change, error)
	// MarkUndone sets undone_at; false when it was set already.
	MarkUndone(ctx context.Context, id int64, at time.Time) (bool, error)
}

// changeNote is what a plan-writing tool tells Execute about its call: the
// line's text key and params, a regenerate run it waited for and the cell
// set_availability changed.
type changeNote struct {
	set     bool
	textKey string
	params  map[string]any
	runIDs  []int64
	cell    *CellBefore
}

type changeNoteKey struct{}

func withChangeNote(ctx context.Context) (context.Context, *changeNote) {
	n := &changeNote{}
	return context.WithValue(ctx, changeNoteKey{}, n), n
}

func noteFrom(ctx context.Context) *changeNote {
	n, _ := ctx.Value(changeNoteKey{}).(*changeNote)
	if n == nil {
		return &changeNote{} // a handler called outside Execute: noted nowhere
	}
	return n
}

// noteChange marks the tool call a change with its line's text key and
// params.
func noteChange(ctx context.Context, textKey string, params map[string]any) {
	n := noteFrom(ctx)
	n.set, n.textKey, n.params = true, textKey, params
}

// noteRun adds a run whose events belong to the call (regenerate's run).
func noteRun(ctx context.Context, runID int64) {
	n := noteFrom(ctx)
	n.runIDs = append(n.runIDs, runID)
}

// noteCell records the cell's override before set_availability writes it.
func noteCell(ctx context.Context, date time.Time, slotID int64, o *availability.Override) {
	cb := &CellBefore{Date: date.Format(time.DateOnly), SlotID: slotID}
	if o != nil {
		cb.Override = &CellOverride{CellMode: string(o.Mode), PresetID: o.PresetID, Note: o.Note}
	}
	noteFrom(ctx).cell = cb
}

// watermark is the last plate event id before a call, or -1 when changes
// are not recorded (no store, or no run to attribute them to).
func (s *ToolSet) watermark(ctx context.Context, runID int64) int64 {
	if s.changes == nil || runID == 0 {
		return -1
	}
	id, err := s.changes.LastEventID(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ai.change.watermark failed", "run_id", runID, "error", err)
		return -1
	}
	return id
}

// recordChange stores the change a tool call noted: the plate events its
// runs wrote after mark, and what it shows. A failure is logged and yields
// no change: the call's writes stand either way.
func (s *ToolSet) recordChange(ctx context.Context, runID int64, seq int, tool string, n *changeNote, mark int64) *Change {
	runIDs := append([]int64{runID}, n.runIDs...)
	events, err := s.changes.RunEvents(ctx, mark, math.MaxInt64, runIDs)
	if err != nil {
		slog.WarnContext(ctx, "ai.change.events failed", "run_id", runID, "tool", tool, "error", err)
		return nil
	}
	c := &Change{
		RunID: runID, StepSeq: seq, Tool: tool, TextKey: n.textKey, Params: n.params,
		RunIDs: runIDs, CellBefore: n.cell,
		Dates: []string{}, PlateIDs: []int64{}, SessionIDs: []int64{},
	}
	if c.Params == nil {
		c.Params = map[string]any{}
	}
	if n.cell != nil {
		c.Dates = append(c.Dates, n.cell.Date)
	}
	plates, sessions := eventRows(events)
	c.PlateIDs, c.SessionIDs = plates, sessions
	for _, e := range events {
		c.Dates = append(c.Dates, e.Date.Format(time.DateOnly))
	}
	slices.Sort(c.Dates)
	c.Dates = slices.Compact(c.Dates)
	if len(events) > 0 {
		first, last := events[0].ID, events[len(events)-1].ID
		c.FirstEventID, c.LastEventID = &first, &last
	}
	if err := s.changes.CreateChange(ctx, c); err != nil {
		slog.WarnContext(ctx, "ai.change.create failed", "run_id", runID, "tool", tool, "error", err)
		return nil
	}
	return c
}

// eventRows returns the plates and the sessions events are about, each
// ascending without duplicates. A plate event of a session edit names the
// plate only.
func eventRows(events []plate.Event) (plateIDs, sessionIDs []int64) {
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

// CellRestorer puts a cell's availability override back
// (availability.Service).
type CellRestorer interface {
	RestoreOverride(ctx context.Context, date time.Time, slotID int64, o *availability.Override) error
}

// Changes lists a conversation's changes and undoes one (D13).
type Changes struct {
	store  ChangeStore
	plates *plate.Service
	cells  CellRestorer
	tx     domain.Transactor
	now    func() time.Time
}

// NewChanges wires the undo service.
func NewChanges(store ChangeStore, plates *plate.Service, cells CellRestorer, tx domain.Transactor, now func() time.Time) *Changes {
	return &Changes{store: store, plates: plates, cells: cells, tx: tx, now: now}
}

// List returns the changes of a conversation, ascending by id.
func (c *Changes) List(ctx context.Context, conversationID int64) ([]Change, error) {
	return c.store.ConversationChanges(ctx, conversationID)
}

// Undo reverts one change as the actor in ctx (the user), in one unit of
// work: its plates and sessions go back to their state before it
// (plate.Service.Revert) and a set_availability cell gets its override back.
// It fails with domain.ErrAIChangeUndone when the change was undone and
// domain.ErrAIChangeStale, writing nothing, when one of its rows has an event
// the change did not make. It returns the rows it wrote.
func (c *Changes) Undo(ctx context.Context, id int64) (plate.RowSet, error) {
	var rows plate.RowSet
	err := c.tx.InTx(ctx, func(ctx context.Context) error {
		ch, err := c.store.GetChange(ctx, id)
		if err != nil {
			return err
		}
		if ch.UndoneAt != nil {
			return fmt.Errorf("%w: change %d", domain.ErrAIChangeUndone, id)
		}
		if ch.FirstEventID != nil {
			events, err := c.store.RunEvents(ctx, *ch.FirstEventID-1, *ch.LastEventID, ch.RunIDs)
			if err != nil {
				return err
			}
			plateIDs, sessionIDs := eventRows(events)
			// Every event on these rows since the change began must be one of
			// the change's own: a user edit between its first and last event
			// has no run id and would otherwise be overwritten.
			all, err := c.store.LaterEvents(ctx, *ch.FirstEventID-1, plateIDs, sessionIDs)
			if err != nil {
				return err
			}
			if all != int64(len(events)) {
				return fmt.Errorf("%w: change %d", domain.ErrAIChangeStale, id)
			}
			if rows, err = c.plates.Revert(ctx, events); err != nil {
				return err
			}
		}
		if cb := ch.CellBefore; cb != nil {
			if err := c.restoreCell(ctx, *cb); err != nil {
				return err
			}
		}
		ok, err := c.store.MarkUndone(ctx, id, c.now())
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: change %d", domain.ErrAIChangeUndone, id)
		}
		return nil
	})
	if err != nil {
		return plate.RowSet{}, err
	}
	if rows.PlateIDs == nil {
		rows.PlateIDs = []int64{}
	}
	if rows.SessionIDs == nil {
		rows.SessionIDs = []int64{}
	}
	return rows, nil
}

func (c *Changes) restoreCell(ctx context.Context, cb CellBefore) error {
	d, err := time.Parse(time.DateOnly, cb.Date)
	if err != nil {
		return fmt.Errorf("change cell date: %w", err)
	}
	var o *availability.Override
	if cb.Override != nil {
		o = &availability.Override{Mode: availability.Mode(cb.Override.CellMode), PresetID: cb.Override.PresetID, Note: cb.Override.Note}
	}
	return c.cells.RestoreOverride(ctx, d, cb.SlotID, o)
}

// plateNames is a plate's food names for a change line, comma-separated.
func plateNames(p *plate.Plate) string {
	names := make([]string, len(p.Components))
	for i, c := range p.Components {
		names[i] = c.FoodName
	}
	return strings.Join(names, ", ")
}
