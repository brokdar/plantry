package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// Phase 4 B5: every plan-writing tool call of a chat run is one change with
// its own undo (D13). The calls run in a chat run of a conversation, as the
// agent service starts them.

// chat returns the plan tool ctx inside a new chat run of a new conversation.
func (e *planToolEnv) chat() (context.Context, int64) {
	e.t.Helper()
	conv := e.insert(`INSERT INTO ai_conversations (title) VALUES ('B5')`)
	ctx, run := e.rec.Start(e.ctx, airun.Start{Kind: airun.KindChat, ConversationID: &conv})
	require.NotZero(e.t, run.ID())
	return ctx, conv
}

// change runs a tool in ctx and returns the change it recorded.
func (e *planToolEnv) change(ctx context.Context, name, input string) *agent.Change {
	e.t.Helper()
	_, _, ch, err := e.ts.Execute(ctx, name, json.RawMessage(input))
	require.NoError(e.t, err)
	require.NotNil(e.t, ch, "a plan write is a change")
	return ch
}

func (e *planToolEnv) eventCount() int {
	e.t.Helper()
	evs, err := sqlite.NewPlateEventRepo(e.db).List(context.Background())
	require.NoError(e.t, err)
	return len(evs)
}

// sessionNamed returns the draft session of the food named name.
func (e *planToolEnv) sessionNamed(name string) int64 {
	e.t.Helper()
	for _, p := range e.week() {
		for _, c := range p.Components {
			if c.FoodName == name && c.CookSessionID != nil && p.State == plate.StateDraft {
				return *c.CookSessionID
			}
		}
	}
	e.t.Fatalf("no draft session of %s in the week", name)
	return 0
}

// B5-AC1
func TestChange_SetSessionPortions(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	sid := e.sessionNamed("Chili")
	before := e.session(sid)

	ch := e.change(ctx, "set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":6,"frozen_portions":2}`, sid))
	assert.Equal(t, "set_session_portions", ch.TextKey)
	assert.Equal(t, "Chili", ch.Params["name"])
	assert.Equal(t, 6.0, ch.Params["portions"])
	assert.Equal(t, 2.0, ch.Params["frozen"])
	assert.Equal(t, []int64{sid}, ch.SessionIDs)
	assert.True(t, ch.Undoable())
	var rows int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM ai_changes WHERE text_key = 'set_session_portions'`).Scan(&rows))
	assert.Equal(t, 1, rows, "one ai_changes row")

	n := e.eventCount()
	out, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	assert.Contains(t, out.SessionIDs, sid)
	v := e.session(sid)
	assert.InDelta(t, before.Portions, v.Portions, 1e-9)
	assert.InDelta(t, before.FrozenPortions, v.FrozenPortions, 1e-9)
	evs, err := sqlite.NewPlateEventRepo(e.db).List(context.Background())
	require.NoError(t, err)
	require.Greater(t, len(evs), n)
	for _, ev := range evs[n:] {
		assert.Equal(t, actor.User, ev.Actor, "undo writes user events")
	}
}

// B5-AC2
func TestChange_RegenerateSideUndo(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	target, idx := e.sidePlate()
	old := target.Components[idx]
	oldSession := e.session(*old.CookSessionID)

	ch := e.change(ctx, "regenerate", fmt.Sprintf(`{"kind":"side","component_id":%d}`, old.ID))
	assert.Equal(t, "regenerate_side", ch.TextKey)
	after, err := e.plates.Get(context.Background(), target.ID)
	require.NoError(t, err)
	newSide := ""
	for _, c := range after.Components {
		if !slices.ContainsFunc(target.Components, func(o plate.PlateComponent) bool { return o.FoodID == c.FoodID }) {
			newSide = c.FoodName
		}
	}
	require.NotEmpty(t, newSide, "the regenerate swaps in a different side")
	assert.Equal(t, newSide, ch.Params["name"], "AC-2.2: the line names the new side")
	assert.Equal(t, target.DateString(), ch.Params["date"])
	assert.NotEmpty(t, ch.Params["slot"])
	require.Len(t, ch.RunIDs, 2, "the chat run and the regenerate run")
	evs, err := sqlite.NewAIChangeRepo(e.db).RunEvents(context.Background(), *ch.FirstEventID-1, *ch.LastEventID, ch.RunIDs[1:])
	require.NoError(t, err)
	assert.NotEmpty(t, evs, "the change spans the regenerate run's events")
	assert.Contains(t, ch.PlateIDs, target.ID)

	_, err = e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	back, err := e.plates.Get(context.Background(), target.ID)
	require.NoError(t, err)
	assert.Equal(t, components(target), components(*back), "the old side is back with its session")
	v := e.session(*old.CookSessionID)
	assert.InDelta(t, oldSession.Portions, v.Portions, 1e-9)
	assert.Equal(t, old.FoodID, v.FoodID)
}

// B5-AC3
func TestChange_UndoStaleAndTwice(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.mainSession()

	ch := e.change(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, p.ID))
	assert.Equal(t, "set_plate_eaters", ch.TextKey)
	assert.Equal(t, 3.0, ch.Params["eaters"])
	four := 4.0
	_, err := e.plates.Update(context.Background(), p.ID, plate.PlatePatch{Eaters: &four})
	require.NoError(t, err)

	n := e.eventCount()
	_, err = e.changes.Undo(context.Background(), ch.ID)
	require.ErrorIs(t, err, domain.ErrAIChangeStale)
	key, _ := domain.MessageKey(err, "ai")
	assert.Equal(t, "error.ai.change_stale", key)
	assert.Equal(t, n, e.eventCount(), "nothing written")
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.InDelta(t, 4.0, got.Eaters, 1e-9)

	ch2 := e.change(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":2}`, p.ID))
	_, err = e.changes.Undo(context.Background(), ch2.ID)
	require.NoError(t, err)
	got, err = e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.InDelta(t, 4.0, got.Eaters, 1e-9)
	_, err = e.changes.Undo(context.Background(), ch2.ID)
	require.ErrorIs(t, err, domain.ErrAIChangeUndone)
	key, _ = domain.MessageKey(err, "ai")
	assert.Equal(t, "error.ai.change_undone", key)

	_, err = e.changes.Undo(context.Background(), 999999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// B5-AC4
func TestChange_UndoDeletePlate(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.mainSession()
	require.NotNil(t, p.AIRunID)

	ch := e.change(ctx, "delete_plate", fmt.Sprintf(`{"plate_id":%d}`, p.ID))
	assert.Equal(t, "delete_plate", ch.TextKey)
	assert.Equal(t, p.DateString(), ch.Params["date"])
	assert.NotEmpty(t, ch.Params["name"])

	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	back, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err, "the plate is back under its id")
	assert.Equal(t, plate.StateDraft, back.State)
	require.NotNil(t, back.AIRunID)
	assert.Equal(t, *p.AIRunID, *back.AIRunID)
	assert.Equal(t, components(p), components(*back))
}

// B5-AC4
func TestChange_UndoSetAvailabilityNone(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.mainSession()
	note := "Dinner at the cafe"
	_, _, err := e.avail.SetOverride(context.Background(), availability.OverrideInput{Date: p.Date, SlotID: p.SlotID, Mode: availability.ModePlan, Note: &note})
	require.NoError(t, err)
	beforeOverride, err := e.avail.Override(context.Background(), p.Date, p.SlotID)
	require.NoError(t, err)
	require.NotNil(t, beforeOverride)

	ch := e.change(ctx, "set_availability", fmt.Sprintf(`{"date":%q,"slot_id":%d,"mode":"none"}`, p.DateString(), p.SlotID))
	assert.Equal(t, "set_availability_none", ch.TextKey)
	require.NotNil(t, ch.CellBefore)
	assert.Contains(t, ch.PlateIDs, p.ID, "the removed draft plate is part of the change")

	_, err = e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	o, err := e.avail.Override(context.Background(), p.Date, p.SlotID)
	require.NoError(t, err)
	require.NotNil(t, o)
	assert.Equal(t, beforeOverride.Mode, o.Mode, "the override is back")
	back, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err, "the removed draft plate is back")
	assert.Equal(t, plate.StateDraft, back.State)
}

// B5-AC4: an availability change on an empty cell clears the override it
// wrote.
func TestChange_UndoSetAvailabilityWithoutOverride(t *testing.T) {
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

	ch := e.change(ctx, "set_availability", fmt.Sprintf(`{"date":%q,"slot_id":%d,"mode":"none"}`, d.Format("2006-01-02"), slotID))
	assert.Equal(t, "set_availability_none", ch.TextKey)
	assert.Equal(t, "slot.breakfast", ch.Params["slot"])
	assert.Nil(t, ch.FirstEventID, "no plate was touched")
	assert.True(t, ch.Undoable())

	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	o, err := e.avail.Override(context.Background(), d, slotID)
	require.NoError(t, err)
	assert.Nil(t, o)
}

// B5-AC5 (store side): the conversation lists its changes in order with
// undone_at.
func TestChange_ConversationList(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, conv := e.chat()
	p, sid := e.mainSession()
	first := e.change(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, p.ID))
	second := e.change(ctx, "set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":9}`, sid))
	_, err := e.changes.Undo(context.Background(), second.ID)
	require.NoError(t, err)
	other, _ := e.chat()
	e.change(other, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":5}`, p.ID))

	list, err := e.changes.List(context.Background(), conv)
	require.NoError(t, err)
	require.Len(t, list, 2, "only this conversation's changes")
	assert.Equal(t, first.ID, list[0].ID)
	assert.Nil(t, list[0].UndoneAt)
	assert.Equal(t, second.ID, list[1].ID)
	assert.NotNil(t, list[1].UndoneAt)
	assert.Equal(t, "set_session_portions", list[1].TextKey)
	assert.Equal(t, 9.0, list[1].Params["portions"])
}

// A tool call outside a chat run, or a read, records no change.
func TestChange_OnlyPlanWritesOfARun(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.mainSession()
	_, _, ch, err := e.ts.Execute(e.ctx, "set_plate_eaters", json.RawMessage(fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, p.ID)))
	require.NoError(t, err)
	assert.Nil(t, ch, "no run: no change")

	ctx, _ := e.chat()
	_, _, ch, err = e.ts.Execute(ctx, "get_plan", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Nil(t, ch, "a read is no change")
	_, _, ch, err = e.ts.Execute(ctx, "set_plate_eaters", json.RawMessage(`{"plate_id":999999,"eaters":3}`))
	require.Error(t, err)
	assert.Nil(t, ch, "a failed call is no change")
}

// D13: a confirm change is undone like any other: its rows go back to draft.
func TestChange_UndoConfirm(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, sid := e.mainSession()

	ch := e.change(ctx, "confirm", fmt.Sprintf(`{"plate_id":%d}`, p.ID))
	assert.Equal(t, "confirm", ch.TextKey)
	assert.Equal(t, len(ch.PlateIDs), ch.Params["count"])
	assert.Equal(t, plate.StateConfirmed, e.session(sid).State)

	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	assert.Equal(t, plate.StateDraft, e.session(sid).State)
	back, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, plate.StateDraft, back.State)
}

// D16: the chat stream carries one change event per plan-writing call, after
// its tool result.
func TestChange_ChatStreamsChangeEvent(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.mainSession()
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		{StopReason: "tool_use", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "tool_use", ToolUseID: "tu_0", ToolName: "get_plan", ToolInput: json.RawMessage(`{}`)},
			{Type: "tool_use", ToolUseID: "tu_1", ToolName: "set_plate_eaters", ToolInput: json.RawMessage(fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, p.ID))},
		}}},
		endTurn(fake.ScriptedUsage{}),
	}})
	svc := agent.NewService(sqlite.NewAIRepo(e.db), &scriptResolver{clients: []llm.Client{client}}, e.ts,
		profile.NewService(sqlite.NewProfileRepo(e.db)), testsvc.Settings(e.db), planContext(e.db), time.Now).WithRecorder(e.rec)

	evts, err := chatReq(context.Background(), svc, agent.ChatRequest{UserText: "three of us", Today: planToday, Start: planStart}, nil)
	require.NoError(t, err)
	var changes []llm.ChangePayload
	lastResult := -1
	for i, ev := range evts {
		switch pl := ev.Payload.(type) {
		case llm.ToolResultPayload:
			lastResult = i
		case llm.ChangePayload:
			assert.Equal(t, llm.EventChange, ev.Type)
			assert.Greater(t, i, lastResult)
			changes = append(changes, pl)
		}
	}
	require.Len(t, changes, 1, "the read is no change")
	c := changes[0]
	assert.NotZero(t, c.ID)
	assert.NotZero(t, c.RunID)
	assert.Equal(t, "set_plate_eaters", c.Tool)
	assert.Equal(t, "set_plate_eaters", c.TextKey)
	assert.Contains(t, c.Dates, p.DateString())
	assert.Contains(t, c.PlateIDs, p.ID)
	assert.True(t, c.Undoable)
}

// A user edit of a touched plate between the change's first and last event
// is neither in the change nor after it; undo must still refuse.
func TestChange_UndoStaleWhenUserEditedMidChange(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	target, idx := e.sidePlate()
	old := target.Components[idx]

	ch := e.change(ctx, "regenerate", fmt.Sprintf(`{"kind":"side","component_id":%d}`, old.ID))
	require.NotNil(t, ch.LastEventID)
	four := 4.0
	_, err := e.plates.Update(context.Background(), target.ID, plate.PlatePatch{Eaters: &four})
	require.NoError(t, err)
	// Make the user's event fall inside the change's id range.
	_, err = e.db.Exec(`UPDATE ai_changes SET last_event_id = (SELECT MAX(id) FROM plate_events) WHERE id = ?`, ch.ID)
	require.NoError(t, err)

	n := e.eventCount()
	_, err = e.changes.Undo(context.Background(), ch.ID)
	require.ErrorIs(t, err, domain.ErrAIChangeStale)
	assert.Equal(t, n, e.eventCount(), "nothing written")
	got, err := e.plates.Get(context.Background(), target.ID)
	require.NoError(t, err)
	assert.InDelta(t, 4.0, got.Eaters, 1e-9)
}
