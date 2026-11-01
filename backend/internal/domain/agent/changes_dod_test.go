package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// Phase 4 DoD fixes for changes: recording survives a client disconnect, a
// failed save is not silent, undo refuses what it cannot revert.

// toolSetWith builds the env's tool set over another planner or change store.
func (e *planToolEnv) toolSetWith(planner agent.Planner, changes agent.ChangeStore) *agent.ToolSet {
	e.t.Helper()
	foods := sqlite.NewFoodRepo(e.db)
	ts, err := agent.NewToolSet(agent.Services{
		Foods: food.NewService(foods), NutritionResolver: food.NewNutritionResolver(foods),
		Plates: e.plates, Profile: profile.NewService(sqlite.NewProfileRepo(e.db)),
		Slots: slot.NewService(sqlite.NewSlotRepo(e.db)), Settings: testsvc.Settings(e.db),
		PlanView: testsvc.PlanView(e.db), Planner: planner, Cells: e.avail, Changes: changes,
	})
	require.NoError(e.t, err)
	return ts
}

// dropAwait is a planner whose first Await reports the caller's ctx ended
// (the client went away) while the run, which goes on, has finished.
type dropAwait struct {
	agent.Planner
	cancel context.CancelFunc
	calls  int
	detach []error // ctx.Err() of every later Await
}

func (p *dropAwait) Await(ctx context.Context, id int64) (*planning.RunView, error) {
	p.calls++
	if p.calls == 1 {
		p.cancel()
		return nil, ctx.Err()
	}
	p.detach = append(p.detach, ctx.Err())
	return p.Planner.Await(ctx, id)
}

// failCreate is a change store whose save fails.
type failCreate struct{ agent.ChangeStore }

func (failCreate) CreateChange(context.Context, *agent.Change) error { return errors.New("disk full") }

// DoD go S1 / sfh 2: a regenerate whose caller went away still records its
// change once the run finished, on a detached ctx, so it can be undone.
func TestChange_RegenerateRecordedAfterClientDisconnect(t *testing.T) {
	e := newPlanToolEnv(t)
	target, idx := e.sidePlate()
	old := target.Components[idx]
	ctx, cancel := context.WithCancel(e.ctx)
	conv := e.insert(`INSERT INTO ai_conversations (title) VALUES ('DoD')`)
	ctx, run := e.rec.Start(ctx, airun.Start{Kind: airun.KindChat, ConversationID: &conv})
	require.NotZero(t, run.ID())
	planner := &dropAwait{Planner: e.planner, cancel: cancel}
	ts := e.toolSetWith(planner, sqlite.NewAIChangeRepo(e.db))

	_, _, ch, err := ts.Execute(ctx, "regenerate", json.RawMessage(fmt.Sprintf(`{"kind":"side","component_id":%d}`, old.ID)))
	require.ErrorIs(t, err, context.Canceled, "the tool still reports the disconnect")
	require.NotNil(t, ch, "the run rewrote the plan, so the change is recorded")
	assert.True(t, ch.Undoable())
	assert.Equal(t, "regenerate_side", ch.TextKey)
	require.Len(t, ch.RunIDs, 2)
	assert.Equal(t, []error{nil}, planner.detach, "the run is waited for on a ctx that is not cancelled")
	var rows int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM ai_changes WHERE id = ?`, ch.ID).Scan(&rows))
	assert.Equal(t, 1, rows)

	_, err = e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	back, err := e.plates.Get(context.Background(), target.ID)
	require.NoError(t, err)
	assert.Equal(t, components(target), components(*back))
}

// DoD sfh 1: a change that could not be saved is returned unrecorded, the
// tool result says so, and the call's write stands.
func TestChange_SaveFailureIsSurfaced(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.mainSession()
	ts := e.toolSetWith(e.planner, failCreate{sqlite.NewAIChangeRepo(e.db)})

	out, _, ch, err := ts.Execute(ctx, "set_plate_eaters", json.RawMessage(fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, p.ID)))
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.Zero(t, ch.ID)
	assert.False(t, ch.Undoable())
	pl := ch.Payload()
	assert.False(t, pl.Recorded)
	assert.False(t, pl.Undoable)
	assert.Equal(t, "set_plate_eaters", pl.Tool)
	assert.Equal(t, "set_plate_eaters", pl.TextKey)
	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, "change_not_recorded", res["warning"])
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, got.Eaters, 1e-9, "the write stands")

	ok := e.change(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":2}`, p.ID))
	assert.True(t, ok.Payload().Recorded)
}

// DoD sfh 4: a change with nothing to revert is not "undone".
func TestChange_UndoNothingToRevert(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	empty := &agent.Change{
		RunID: airun.FromContext(ctx).ID(), StepSeq: 1, Tool: "regenerate", TextKey: "regenerate_day",
		Params: map[string]any{}, RunIDs: []int64{airun.FromContext(ctx).ID()},
		Dates: []string{}, PlateIDs: []int64{}, SessionIDs: []int64{},
	}
	require.NoError(t, sqlite.NewAIChangeRepo(e.db).CreateChange(context.Background(), empty))
	require.False(t, empty.Undoable())

	_, err := e.changes.Undo(context.Background(), empty.ID)
	require.ErrorIs(t, err, domain.ErrAIChangeNotUndoable)
	var undone *string
	require.NoError(t, e.db.QueryRow(`SELECT undone_at FROM ai_changes WHERE id = ?`, empty.ID).Scan(&undone))
	assert.Nil(t, undone, "not marked undone")
}

// DoD sfh 3: a newer manual edit of the cell makes the availability undo
// stale; nothing is written.
func TestChange_UndoAvailabilityStaleAfterManualEdit(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	d := planStart.AddDate(0, 0, 6)
	var slotID int64
	require.NoError(t, e.db.QueryRow(`SELECT id FROM time_slots WHERE name_key = 'slot.breakfast'`).Scan(&slotID))
	for _, p := range e.week() {
		if p.Date.Equal(d) && p.SlotID == slotID {
			_, err := e.plates.Delete(context.Background(), p.ID)
			require.NoError(t, err)
		}
	}
	ch := e.change(ctx, "set_availability", fmt.Sprintf(`{"date":%q,"slot_id":%d,"mode":"none"}`, d.Format(time.DateOnly), slotID))
	require.NotNil(t, ch.CellBefore)

	note := "eating out after all"
	_, _, err := e.avail.SetOverride(context.Background(), availability.OverrideInput{Date: d, SlotID: slotID, Mode: availability.ModePlan, Note: &note})
	require.NoError(t, err)

	_, err = e.changes.Undo(context.Background(), ch.ID)
	require.ErrorIs(t, err, domain.ErrAIChangeStale)
	o, err := e.avail.Override(context.Background(), d, slotID)
	require.NoError(t, err)
	require.NotNil(t, o, "the user's override stands")
	assert.Equal(t, availability.ModePlan, o.Mode)
	var undone *string
	require.NoError(t, e.db.QueryRow(`SELECT undone_at FROM ai_changes WHERE id = ?`, ch.ID).Scan(&undone))
	assert.Nil(t, undone)
}

// DoD go nit 5: an undo is not a user correction.
func TestChange_UndoIsNotACorrection(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	target, idx := e.sidePlate()
	old := target.Components[idx]
	ch := e.change(ctx, "regenerate", fmt.Sprintf(`{"kind":"side","component_id":%d}`, old.ID))
	n := e.eventCount()

	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)

	evs, err := sqlite.NewPlateEventRepo(e.db).List(context.Background())
	require.NoError(t, err)
	require.Greater(t, len(evs), n)
	for _, ev := range evs[n:] {
		assert.Equal(t, plate.ActionUndo, ev.Action)
	}
	repl, err := sqlite.NewPlateEventRepo(e.db).UserReplacements(context.Background(), planStart, planStart.AddDate(0, 0, 7))
	require.NoError(t, err)
	assert.Empty(t, repl, "the swap back is not a user recipe swap")
}
