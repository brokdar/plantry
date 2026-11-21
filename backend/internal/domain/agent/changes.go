package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// UndoBlockedConfirmed is the undo_blocked reason of a change whose rows are
// confirmed now.
const UndoBlockedConfirmed = "confirmed"

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
	// Cells are the cells the call's availability ops changed, each as it
	// was before the call and as the call left it.
	Cells []CellBefore
	// Light is the light requests of the dates the call saved, before and
	// after; nil when it saved none.
	Light     *LightBefore
	CreatedAt time.Time
	UndoneAt  *time.Time
	// Unrecorded marks a change that was made but could not be saved: it has
	// no ID, is not undoable and is never stored.
	Unrecorded bool
	// UndoBlocked is set by Changes.List: UndoBlockedConfirmed when a row of
	// the change is confirmed now, else nil.
	UndoBlocked *string
}

// Undoable reports whether the change wrote anything undo can put back.
func (c Change) Undoable() bool {
	return !c.Unrecorded && (c.FirstEventID != nil || len(c.Cells) > 0 || c.Light != nil)
}

// Payload is the change as the chat stream and the conversation carry it.
func (c Change) Payload() llm.ChangePayload {
	return llm.ChangePayload{
		ID: c.ID, RunID: c.RunID, Tool: c.Tool, TextKey: c.TextKey, Params: c.Params,
		Dates: c.Dates, PlateIDs: c.PlateIDs, SessionIDs: c.SessionIDs, Undoable: c.Undoable(), UndoBlocked: c.UndoBlocked,
		Recorded: !c.Unrecorded,
	}
}

// CellBefore is a cell's availability override before an availability op;
// Override nil means the cell had none. After is the override the call left
// (nil: none), which undo compares with the current one to see a newer
// manual edit.
type CellBefore struct {
	Date     string        `json:"date"`
	SlotID   int64         `json:"slot_id"`
	Override *CellOverride `json:"override"`
	After    *CellOverride `json:"after"`
}

// LightBefore is the light requests of Dates as a call found them (Before)
// and left them (After; undo compares it with the current ones to see a
// newer edit).
type LightBefore struct {
	Dates  []string               `json:"dates"`
	Before []planning.LightIntent `json:"before"`
	After  []planning.LightIntent `json:"after"`
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
	// CreateChange inserts c and sets its ID and CreatedAt.
	CreateChange(ctx context.Context, c *Change) error
	// GetChange returns the change or an error wrapping domain.ErrNotFound.
	GetChange(ctx context.Context, id int64) (*Change, error)
	// ConversationChanges lists the changes of a conversation's runs by id.
	ConversationChanges(ctx context.Context, conversationID int64) ([]Change, error)
	// UndoneAfter lists the undone changes whose first event id is above
	// eventID.
	UndoneAfter(ctx context.Context, eventID int64) ([]Change, error)
	// MarkUndone sets undone_at; false when it was set already.
	MarkUndone(ctx context.Context, id int64, at time.Time) (bool, error)
}

// changeNote is what a plan-writing tool tells Execute about its call: the
// line's text key and params, a regenerate run it waited for and the cells
// its availability ops changed.
type changeNote struct {
	set     bool
	textKey string
	params  map[string]any
	runIDs  []int64
	cells   []CellBefore
	light   *LightBefore
	// dates are the cells' dates the call touched besides its events' (a
	// move's source and target).
	dates []string
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

// noteDates adds dates the change line's cells cover.
func noteDates(ctx context.Context, dates []string) {
	n := noteFrom(ctx)
	n.dates = append(n.dates, dates...)
}

// noteRun adds a run whose events belong to the call (regenerate's run).
func noteRun(ctx context.Context, runID int64) {
	n := noteFrom(ctx)
	n.runIDs = append(n.runIDs, runID)
}

// noteCell records a cell's override before an availability op writes it and
// the one it left. A cell noted again keeps its first before and takes the
// new after, so undo restores the state before the whole call.
func noteCell(ctx context.Context, date time.Time, slotID int64, before, after *availability.Override) {
	n := noteFrom(ctx)
	d := date.Format(time.DateOnly)
	for i := range n.cells {
		if n.cells[i].Date == d && n.cells[i].SlotID == slotID {
			n.cells[i].After = cellOverride(after)
			return
		}
	}
	n.cells = append(n.cells, CellBefore{Date: d, SlotID: slotID, Override: cellOverride(before), After: cellOverride(after)})
}

// noteLight records the light requests of dates before and after a call
// saved them; nothing when they are the same.
func noteLight(ctx context.Context, dates []string, before, after []planning.LightIntent) {
	if sameLight(before, after) {
		return
	}
	noteFrom(ctx).light = &LightBefore{Dates: dates, Before: before, After: after}
}

// sameLight reports whether two sets of intents are equal, order aside.
func sameLight(a, b []planning.LightIntent) bool {
	return len(a) == len(b) && !slices.ContainsFunc(a, func(l planning.LightIntent) bool { return !slices.Contains(b, l) })
}

// lightOn keeps the intents dated in dates.
func lightOn(in []planning.LightIntent, dates []string) []planning.LightIntent {
	var out []planning.LightIntent
	for _, l := range in {
		if slices.Contains(dates, l.Date) {
			out = append(out, l)
		}
	}
	return out
}

func cellOverride(o *availability.Override) *CellOverride {
	if o == nil {
		return nil
	}
	return &CellOverride{CellMode: string(o.Mode), PresetID: o.PresetID, Note: o.Note}
}

// sameOverride reports whether two overrides say the same (nil: none).
func sameOverride(a, b *CellOverride) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.CellMode == b.CellMode && equalPtr(a.PresetID, b.PresetID) && equalPtr(a.Note, b.Note)
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
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

// unrecordedChange is the change line of a call whose change could not be
// saved: no id, nothing to undo. The call's writes stand.
func unrecordedChange(runID int64, seq int, tool string, n *changeNote) *Change {
	c := &Change{
		RunID: runID, StepSeq: seq, Tool: tool, TextKey: n.textKey, Params: n.params, Unrecorded: true,
		RunIDs: append([]int64{runID}, n.runIDs...), Dates: []string{}, PlateIDs: []int64{}, SessionIDs: []int64{},
	}
	if c.Params == nil {
		c.Params = map[string]any{}
	}
	return c
}

// recordChange stores the change a tool call noted: the plate events its
// runs wrote after mark, and what it shows. A failure is logged at error
// level and yields an unrecorded change: the call's writes stand either way.
func (s *ToolSet) recordChange(ctx context.Context, runID int64, seq int, tool string, n *changeNote, mark int64) *Change {
	runIDs := append([]int64{runID}, n.runIDs...)
	events, err := s.changes.RunEvents(ctx, mark, math.MaxInt64, runIDs)
	if err != nil {
		slog.ErrorContext(ctx, "ai.change.events failed", "run_id", runID, "tool", tool, "error", err)
		return unrecordedChange(runID, seq, tool, n)
	}
	c := &Change{
		RunID: runID, StepSeq: seq, Tool: tool, TextKey: n.textKey, Params: n.params,
		RunIDs: runIDs, Cells: n.cells, Light: n.light,
		Dates: []string{}, PlateIDs: []int64{}, SessionIDs: []int64{},
	}
	if c.Params == nil {
		c.Params = map[string]any{}
	}
	for _, cb := range n.cells {
		c.Dates = append(c.Dates, cb.Date)
	}
	if n.light != nil {
		c.Dates = append(c.Dates, n.light.Dates...)
	}
	for _, d := range n.dates {
		if d != "" {
			c.Dates = append(c.Dates, d)
		}
	}
	plates, sessions := plate.EventRows(events)
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
		slog.ErrorContext(ctx, "ai.change.create failed", "run_id", runID, "tool", tool, "error", err)
		return unrecordedChange(runID, seq, tool, n)
	}
	return c
}

// CellRestorer puts a cell's availability override back
// (availability.Service).
type CellRestorer interface {
	// Override returns the cell's stored override, nil when none.
	Override(ctx context.Context, date time.Time, slotID int64) (*availability.Override, error)
	RestoreOverride(ctx context.Context, date time.Time, slotID int64, o *availability.Override) error
}

// Changes lists a conversation's changes and undoes one (D13).
type Changes struct {
	store  ChangeStore
	plates *plate.Service
	cells  CellRestorer
	light  planning.LightStore
	tx     domain.Transactor
	now    func() time.Time
}

// NewChanges wires the undo service.
func NewChanges(store ChangeStore, plates *plate.Service, cells CellRestorer, light planning.LightStore, tx domain.Transactor, now func() time.Time) *Changes {
	return &Changes{store: store, plates: plates, cells: cells, light: light, tx: tx, now: now}
}

// List returns the changes of a conversation, ascending by id.
// A change whose rows are confirmed now carries UndoBlocked.
func (c *Changes) List(ctx context.Context, conversationID int64) ([]Change, error) {
	list, err := c.store.ConversationChanges(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	var open []*Change
	for i := range list {
		if list[i].UndoneAt == nil && list[i].Undoable() && list[i].Tool != "confirm" {
			open = append(open, &list[i])
		}
	}
	rows, err := c.confirmedRows(ctx, open)
	if err != nil {
		return nil, err
	}
	for _, ch := range open {
		if rows.hit(ch) {
			reason := UndoBlockedConfirmed
			ch.UndoBlocked = &reason
		}
	}
	return list, nil
}

// confirmedSet holds the ids of the plates and sessions that are confirmed.
type confirmedSet struct {
	plates, sessions map[int64]bool
}

func (s confirmedSet) hit(ch *Change) bool {
	return slices.ContainsFunc(ch.PlateIDs, func(id int64) bool { return s.plates[id] }) ||
		slices.ContainsFunc(ch.SessionIDs, func(id int64) bool { return s.sessions[id] })
}

// confirmedRows loads the plates and sessions of changes in two queries and
// returns those confirmed now.
func (c *Changes) confirmedRows(ctx context.Context, changes []*Change) (confirmedSet, error) {
	set := confirmedSet{plates: map[int64]bool{}, sessions: map[int64]bool{}}
	var plateIDs, sessionIDs []int64
	for _, ch := range changes {
		plateIDs = append(plateIDs, ch.PlateIDs...)
		sessionIDs = append(sessionIDs, ch.SessionIDs...)
	}
	if len(plateIDs) > 0 {
		plates, err := c.plates.PlatesByIDs(ctx, slices.Compact(slices.Sorted(slices.Values(plateIDs))))
		if err != nil {
			return set, err
		}
		for _, p := range plates {
			set.plates[p.ID] = p.State == plate.StateConfirmed
		}
	}
	if len(sessionIDs) > 0 {
		sessions, err := c.plates.CookSessions(ctx, slices.Compact(slices.Sorted(slices.Values(sessionIDs))))
		if err != nil {
			return set, err
		}
		for _, sess := range sessions {
			set.sessions[sess.ID] = sess.State == plate.StateConfirmed
		}
	}
	return set, nil
}

// confirmed reports whether a plate or session of the change is confirmed
// now. A confirm change is exempt: undoing it is what takes the rows back to
// draft.
func (c *Changes) confirmed(ctx context.Context, ch *Change) (bool, error) {
	if ch.Tool == "confirm" {
		return false, nil
	}
	rows, err := c.confirmedRows(ctx, []*Change{ch})
	return rows.hit(ch), err
}

// Undo reverts one change as the actor in ctx (the user), in one unit of
// work: its plates and sessions go back to their state before it
// (plate.Service.Revert) and every cell of its availability ops gets its
// override back.
// It fails with domain.ErrAIChangeUndone when the change was undone and
// domain.ErrAIChangeConfirmed when one of its plates or sessions is
// confirmed now and domain.ErrAIChangeStale, writing nothing, when one of its
// rows has an event the change did not make or any of its cells' override
// is no longer the one it wrote. A change that wrote nothing (not Undoable) is
// domain.ErrAIChangeNotUndoable and stays as it is. It returns the rows it
// wrote.
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
		if !ch.Undoable() {
			return fmt.Errorf("%w: change %d wrote nothing to undo", domain.ErrAIChangeNotUndoable, id)
		}
		blocked, err := c.confirmed(ctx, ch)
		if err != nil {
			return err
		}
		if blocked {
			return fmt.Errorf("%w: change %d", domain.ErrAIChangeConfirmed, id)
		}
		for _, cb := range ch.Cells {
			if err := c.checkCell(ctx, id, cb); err != nil {
				return err
			}
		}
		if ch.Light != nil {
			if err := c.checkLight(ctx, id, ch.Light); err != nil {
				return err
			}
		}
		if ch.FirstEventID != nil {
			events, err := c.store.RunEvents(ctx, *ch.FirstEventID-1, *ch.LastEventID, ch.RunIDs)
			if err != nil {
				return err
			}
			// Every event on these rows since the change began must be one of
			// the change's own or of a newer change that is undone now (with
			// its undo): a user edit between its first and last event has no
			// run id and would otherwise be overwritten.
			tolerated, err := c.undoneEvents(ctx, *ch.FirstEventID)
			if err != nil {
				return err
			}
			rows, err = c.plates.RevertCheckedTolerating(ctx, events, tolerated)
			if errors.Is(err, domain.ErrPlanChangeStale) {
				return fmt.Errorf("%w: change %d", domain.ErrAIChangeStale, id)
			}
			if err != nil {
				return err
			}
		}
		for _, cb := range ch.Cells {
			if err := c.restoreCell(ctx, cb); err != nil {
				return err
			}
		}
		if ch.Light != nil {
			// Prune from the real today: the change's earliest date as
			// "today" would delete the untouched rows before it.
			y, m, d := c.now().Date()
			today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
			if err := c.light.Replace(ctx, today, ch.Light.Dates, ch.Light.Before); err != nil {
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

// undoneEvents returns the ids of the events of the changes undone after
// eventID: their rows were put back, so they do not make an older change
// stale (undoing newest first).
func (c *Changes) undoneEvents(ctx context.Context, eventID int64) (map[int64]bool, error) {
	list, err := c.store.UndoneAfter(ctx, eventID)
	if err != nil {
		return nil, err
	}
	ids := map[int64]bool{}
	for _, ch := range list {
		if ch.FirstEventID == nil {
			continue
		}
		events, err := c.store.RunEvents(ctx, *ch.FirstEventID-1, *ch.LastEventID, ch.RunIDs)
		if err != nil {
			return nil, err
		}
		for _, e := range events {
			ids[e.ID] = true
		}
	}
	return ids, nil
}

// checkCell fails with domain.ErrAIChangeStale when the cell's override is
// not the one the change left (a newer manual edit).
func (c *Changes) checkCell(ctx context.Context, id int64, cb CellBefore) error {
	d, err := time.Parse(time.DateOnly, cb.Date)
	if err != nil {
		return fmt.Errorf("change cell date: %w", err)
	}
	cur, err := c.cells.Override(ctx, d, cb.SlotID)
	if err != nil {
		return err
	}
	if !sameOverride(cellOverride(cur), cb.After) {
		return fmt.Errorf("%w: change %d: the cell %s slot %d was edited since", domain.ErrAIChangeStale, id, cb.Date, cb.SlotID)
	}
	return nil
}

// currentLight is the stored light requests of lb's dates.
func (c *Changes) currentLight(ctx context.Context, lb *LightBefore) ([]planning.LightIntent, error) {
	first, err := time.Parse(time.DateOnly, slices.Min(lb.Dates))
	if err != nil {
		return nil, fmt.Errorf("change light date: %w", err)
	}
	last, err := time.Parse(time.DateOnly, slices.Max(lb.Dates))
	if err != nil {
		return nil, fmt.Errorf("change light date: %w", err)
	}
	stored, err := c.light.List(ctx, first, last)
	return lightOn(stored, lb.Dates), err
}

// checkLight fails with domain.ErrAIChangeStale when the dates' light
// requests are not the ones the change left (a newer edit).
func (c *Changes) checkLight(ctx context.Context, id int64, lb *LightBefore) error {
	cur, err := c.currentLight(ctx, lb)
	if err != nil {
		return err
	}
	if !sameLight(cur, lb.After) {
		return fmt.Errorf("%w: change %d: the light requests of %s were edited since", domain.ErrAIChangeStale, id, strings.Join(lb.Dates, ", "))
	}
	return nil
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
