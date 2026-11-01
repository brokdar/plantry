package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// The plan-writing tools run against the "Batch cooker with freezer"
// fixture (plan §0.1): Saturday shopping, window Sat 2026-11-07 … Fri 11-13,
// today Fri 11-06, slots breakfast/lunch/dinner, household 1, one cooking
// occasion a day, batch cooking, keep 3 days, freezing on, 10 mains (Chili
// first) and 4 sides. The week is generated with the heuristic as drafts.

var (
	planToday = time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	planStart = time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
)

type planToolEnv struct {
	t       *testing.T
	ctx     context.Context // the chat turn's: today and window set
	db      *sql.DB
	deps    planning.Deps
	planner *planning.Service
	plates  *plate.Service
	avail   *availability.Service
	ts      *agent.ToolSet
	sides   []int64
	rec     *airun.Recorder
	changes *agent.Changes
}

func newPlanToolEnv(t *testing.T) *planToolEnv {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	e := &planToolEnv{t: t, db: db}
	set := testsvc.Settings(db)
	for k, v := range map[string]string{
		"plan.shopping_day": "5", "plan.batch_cooking": "true", "plan.default_keep_days": "3",
		"plan.max_cook_occasions_per_day": "1", "plan.variety_weeks": "3", "plan.freezing_enabled": "true",
	} {
		require.NoError(t, set.Set(ctx, k, v))
	}
	e.sql(`UPDATE user_profile SET household_portions = 1, diet_type = 'omnivore' WHERE id = 1`)
	for i, name := range []string{"breakfast", "lunch", "dinner"} {
		e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES (?, 'moon', ?, 1)`, "slot."+name, i)
	}
	beans := e.insert(`INSERT INTO foods (name, kind, source) VALUES ('Kichererbsen', 'leaf', 'manual')`)
	for i := range 10 {
		name := fmt.Sprintf("Main %02d", i+1)
		if i == 0 {
			name = "Chili"
		}
		id := e.insert(`INSERT INTO foods (name, kind, role, reference_portions, freezable) VALUES (?, 'composed', 'main', 4, 1)`, name)
		e.sql(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (?, ?, 600, 'g', 600)`, id, beans)
	}
	for i := range 4 {
		id := e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES (?, 'composed', 'side_veg', 4)`, fmt.Sprintf("Side %d", i+1))
		e.sql(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (?, ?, 400, 'g', 400)`, id, beans)
		e.sides = append(e.sides, id)
	}

	e.deps = testsvc.PlanningDeps(db)
	e.rec = airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	e.deps.RunLog, e.deps.Runs = planning.RecorderLog{Recorder: e.rec}, sqlite.NewAIRunRepo(db)
	e.deps.Go = func(f func()) { f() }
	e.planner = planning.NewService(e.deps)
	e.plates, e.avail = testsvc.Plan(db)
	foods := sqlite.NewFoodRepo(db)
	var err error
	e.ts, err = agent.NewToolSet(agent.Services{
		Foods: food.NewService(foods), NutritionResolver: food.NewNutritionResolver(foods),
		Plates: e.plates, Profile: profile.NewService(sqlite.NewProfileRepo(db)),
		Slots: slot.NewService(sqlite.NewSlotRepo(db)), Settings: set,
		PlanView: testsvc.PlanView(db), Planner: e.planner, Cells: e.avail,
		Changes: sqlite.NewAIChangeRepo(db),
	})
	require.NoError(t, err)
	e.changes = testsvc.Changes(db, e.plates, e.avail)
	e.ctx = agent.WithWindow(agent.WithToday(ctx, planToday), window.Window{Start: planStart, End: planStart.AddDate(0, 0, 6)})

	id, err := e.planner.Generate(ctx, planning.Request{Start: planStart, Today: planToday, Chooser: airun.ChooserHeuristic})
	require.NoError(t, err)
	v, err := e.planner.Run(ctx, id)
	require.NoError(t, err)
	require.Equal(t, airun.StatusOK, v.Status, v.ErrorKey)
	return e
}

func (e *planToolEnv) insert(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.Exec(q, args...)
	require.NoError(e.t, err, q)
	id, err := res.LastInsertId()
	require.NoError(e.t, err)
	return id
}

func (e *planToolEnv) sql(q string, args ...any) {
	e.t.Helper()
	_, err := e.db.Exec(q, args...)
	require.NoError(e.t, err, q)
}

func (e *planToolEnv) exec(name, input string) (map[string]any, error) {
	e.t.Helper()
	out, _, _, err := e.ts.Execute(e.ctx, name, json.RawMessage(input))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	require.NoError(e.t, json.Unmarshal(out, &m))
	return m, nil
}

func (e *planToolEnv) mustExec(name, input string) map[string]any {
	e.t.Helper()
	m, err := e.exec(name, input)
	require.NoError(e.t, err)
	return m
}

// week returns the window's plates in date, slot order.
func (e *planToolEnv) week() []plate.Plate {
	e.t.Helper()
	plates, err := e.plates.Range(context.Background(), planStart, planStart.AddDate(0, 0, 6))
	require.NoError(e.t, err)
	require.NotEmpty(e.t, plates)
	return plates
}

// mainSession returns the first draft plate with a main and that main's
// session id.
func (e *planToolEnv) mainSession() (plate.Plate, int64) {
	e.t.Helper()
	for _, p := range e.week() {
		for _, c := range p.Components {
			if c.FoodRole != nil && *c.FoodRole == "main" && c.CookSessionID != nil && p.State == plate.StateDraft {
				return p, *c.CookSessionID
			}
		}
	}
	e.t.Fatal("no draft main in the week")
	return plate.Plate{}, 0
}

// sidePlate returns a draft plate with a side and the side's index.
func (e *planToolEnv) sidePlate() (plate.Plate, int) {
	e.t.Helper()
	for _, p := range e.week() {
		for i, c := range p.Components {
			if slices.Contains(e.sides, c.FoodID) {
				return p, i
			}
		}
	}
	e.t.Fatal("no plate with a side in the week")
	return plate.Plate{}, 0
}

func (e *planToolEnv) session(id int64) *plate.SessionView {
	e.t.Helper()
	v, err := e.plates.Session(context.Background(), id)
	require.NoError(e.t, err)
	return v
}

// B4-AC1
func TestTool_SetSessionPortions(t *testing.T) {
	e := newPlanToolEnv(t)
	_, id := e.mainSession()

	out := e.mustExec("set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":6,"frozen_portions":2}`, id))
	assert.Equal(t, 6.0, out["portions"])
	assert.Equal(t, 2.0, out["frozen_portions"])
	assert.Equal(t, "draft", out["state"])
	assert.Contains(t, out, "consumed")
	assert.Contains(t, out, "warnings")
	v := e.session(id)
	assert.InDelta(t, 6.0, v.Portions, 1e-9)
	assert.InDelta(t, 2.0, v.FrozenPortions, 1e-9)
	assert.Equal(t, plate.StateDraft, v.State)

	e.mustExec("set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":7}`, id))
	assert.InDelta(t, 2.0, e.session(id).FrozenPortions, 1e-9, "omitted frozen_portions keeps the frozen portions")

	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.freezing_enabled", "false"))
	_, err := e.exec("set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":6,"frozen_portions":3}`, id))
	require.ErrorIs(t, err, domain.ErrSessionFreezingDisabled)
	key, _ := domain.MessageKey(err, "plate")
	assert.Equal(t, "error.session.freezing_disabled", key)
	assert.InDelta(t, 7.0, e.session(id).Portions, 1e-9, "a rejected call writes nothing")

	_, err = e.exec("set_session_portions", `{"session_id":999999,"portions":6}`)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// componentKey is what a regenerate may change about a component.
type componentKey struct {
	FoodID    int64
	SessionID int64
	Portions  float64
}

func components(p plate.Plate) []componentKey {
	out := make([]componentKey, len(p.Components))
	for i, c := range p.Components {
		out[i] = componentKey{FoodID: c.FoodID}
		if c.CookSessionID != nil {
			out[i].SessionID = *c.CookSessionID
		}
		if c.Portions != nil {
			out[i].Portions = *c.Portions
		}
	}
	return out
}

// B4-AC2
func TestTool_RegenerateSide(t *testing.T) {
	e := newPlanToolEnv(t)
	target, idx := e.sidePlate()
	old := target.Components[idx]
	before := e.week()

	out := e.mustExec("regenerate", fmt.Sprintf(`{"kind":"side","component_id":%d}`, old.ID))
	assert.Equal(t, "ok", out["status"])
	runID := int64(out["run_id"].(float64))
	require.NotZero(t, runID)
	changed := out["changed"].([]any)
	require.NotEmpty(t, changed)
	var newSide float64
	for _, c := range changed {
		row := c.(map[string]any)
		if row["date"] == target.DateString() && row["slot_id"] == float64(target.SlotID) {
			comps := row["plate"].(map[string]any)["components"].([]any)
			newSide = comps[idx].(map[string]any)["food_id"].(float64)
		}
	}
	require.NotZero(t, newSide, "the target plate is in changed")
	assert.NotEqual(t, float64(old.FoodID), newSide)
	assert.Contains(t, e.sides, int64(newSide))

	after := e.week()
	require.Len(t, after, len(before))
	for i, b := range before {
		a := after[i]
		require.Equal(t, b.ID, a.ID, "plates stay")
		want, got := components(b), components(a)
		require.Len(t, got, len(want))
		for j := range want {
			if b.Components[j].CookSessionID != nil && *b.Components[j].CookSessionID == *old.CookSessionID {
				assert.NotEqual(t, want[j].FoodID, got[j].FoodID, "the old side is replaced")
				continue
			}
			assert.Equal(t, want[j], got[j], "plate %d component %d unchanged", b.ID, j)
		}
	}

	var kind, scopeKind string
	require.NoError(t, e.db.QueryRow(`SELECT kind, json_extract(scope_json, '$.kind') FROM ai_runs WHERE id = ?`, runID).Scan(&kind, &scopeKind))
	assert.Equal(t, "regenerate", kind)
	assert.Equal(t, "side", scopeKind)

	_, err := e.exec("regenerate", `{"kind":"side"}`)
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "side needs component_id")
	_, err = e.exec("regenerate", `{"kind":"plate"}`)
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "plate needs plate_id")
	_, err = e.exec("regenerate", `{"kind":"day"}`)
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "day needs date")
	_, err = e.exec("regenerate", `{"kind":"day","date":"2026-11-05"}`)
	assert.ErrorIs(t, err, domain.ErrAIPastDate)
}

func TestTool_RegenerateConfirmedPlate(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.mainSession()
	_, err := e.plates.Confirm(context.Background(), plate.RowSet{PlateIDs: []int64{p.ID}})
	require.NoError(t, err)
	_, err = e.exec("regenerate", fmt.Sprintf(`{"kind":"plate","plate_id":%d}`, p.ID))
	assert.ErrorIs(t, err, domain.ErrScopeConfirmed)
}

// gateChooser blocks in Choose until release is closed.
type gateChooser struct {
	entered chan struct{}
	release chan struct{}
}

func (g gateChooser) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	close(g.entered)
	<-g.release
	return planning.Heuristic{}.Choose(ctx, in)
}

// B4-AC2: a window generation already running fails the tool call.
func TestTool_RegenerateWhileGenerationRuns(t *testing.T) {
	e := newPlanToolEnv(t)
	g := gateChooser{entered: make(chan struct{}), release: make(chan struct{})}
	deps := e.deps // shares the Progress registry
	deps.Go = nil
	deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: g}
	other := planning.NewService(deps)
	_, err := other.Generate(context.Background(), planning.Request{Start: planStart, Today: planToday, Chooser: airun.ChooserHeuristic})
	require.NoError(t, err)
	<-g.entered

	_, err = e.exec("regenerate", `{"kind":"window"}`)
	assert.ErrorIs(t, err, domain.ErrGenerationRunning)
	key, _ := domain.MessageKey(err, "plan")
	assert.Equal(t, "error.plan.generation_running", key)

	close(g.release)
	other.Wait()
}

// B4-AC4
func TestTool_SetAvailability(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx := context.Background()
	week := e.week()
	draft, confirmed := week[0], week[len(week)-1]
	require.Equal(t, plate.StateDraft, draft.State)
	_, err := e.plates.Confirm(ctx, plate.RowSet{PlateIDs: []int64{confirmed.ID}})
	require.NoError(t, err)

	out := e.mustExec("set_availability", fmt.Sprintf(`{"date":%q,"slot_id":%d,"mode":"none","note":"eating out"}`, draft.DateString(), draft.SlotID))
	assert.Equal(t, true, out["removed_plate"])
	_, err = e.plates.Get(ctx, draft.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound, "none removes the draft plate")
	mode, err := e.avail.ModeAt(ctx, draft.Date, draft.SlotID)
	require.NoError(t, err)
	assert.Equal(t, availability.ModeNone, mode)

	confirmed, err = func() (plate.Plate, error) {
		p, err := e.plates.Get(ctx, confirmed.ID)
		return *p, err
	}()
	require.NoError(t, err)
	_, err = e.exec("set_availability", fmt.Sprintf(`{"date":%q,"slot_id":%d,"mode":"none"}`, confirmed.DateString(), confirmed.SlotID))
	require.ErrorIs(t, err, domain.ErrRowConfirmed)
	_, err = e.plates.Get(ctx, confirmed.ID)
	assert.NoError(t, err, "the confirmed plate stays")
	mode, err = e.avail.ModeAt(ctx, confirmed.Date, confirmed.SlotID)
	require.NoError(t, err)
	assert.Equal(t, availability.ModePlan, mode, "nothing was written")

	e.mustExec("set_availability", fmt.Sprintf(`{"date":%q,"slot_id":%d,"mode":"default"}`, draft.DateString(), draft.SlotID))
	mode, err = e.avail.ModeAt(ctx, draft.Date, draft.SlotID)
	require.NoError(t, err)
	assert.Equal(t, availability.ModePlan, mode, "default clears the override")

	_, err = e.exec("set_availability", fmt.Sprintf(`{"date":"2026-11-05","slot_id":%d,"mode":"none"}`, draft.SlotID))
	assert.ErrorIs(t, err, domain.ErrAIPastDate)
}

// B4-AC5
func TestTool_ConfirmAndSetPlateEaters(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx := context.Background()
	p, sid := e.mainSession()

	// D11: a session that covers its links exactly keeps covering them.
	v := e.session(sid)
	e.mustExec("set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":%v}`, sid, v.Consumed))
	out := e.mustExec("set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, p.ID))
	assert.Equal(t, 3.0, out["eaters"])
	got, err := e.plates.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, got.Eaters, 1e-9)
	after := e.session(sid)
	assert.InDelta(t, after.Consumed, after.Portions, 1e-9, "the session is resized to cover its links")
	assert.Greater(t, after.Portions, v.Consumed)

	out = e.mustExec("confirm", fmt.Sprintf(`{"session_id":%d}`, sid))
	plateIDs := out["plate_ids"].([]any)
	assert.Contains(t, plateIDs, float64(p.ID))
	assert.Contains(t, out["session_ids"].([]any), float64(sid))
	assert.Equal(t, plate.StateConfirmed, e.session(sid).State)
	for _, id := range plateIDs {
		q, err := e.plates.Get(ctx, int64(id.(float64)))
		require.NoError(t, err)
		assert.Equal(t, plate.StateConfirmed, q.State, "the closure is confirmed")
	}
	var actor string
	require.NoError(t, e.db.QueryRow(`SELECT actor FROM plate_events WHERE action = 'confirm' AND plate_id = ?`, p.ID).Scan(&actor))
	assert.Equal(t, "agent", actor)

	_, err = e.exec("confirm", `{}`)
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "one id is required")
	_, err = e.exec("confirm", fmt.Sprintf(`{"plate_id":%d,"session_id":%d}`, p.ID, sid))
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "only one id")
	_, err = e.exec("set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":2}`, p.ID))
	assert.ErrorIs(t, err, domain.ErrRowConfirmed, "a confirmed plate is guarded")
}

// scriptResolver hands each Chat call the next scripted client.
type scriptResolver struct {
	mu      sync.Mutex
	clients []llm.Client
}

func (r *scriptResolver) Current(context.Context) (llm.Client, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.clients[0]
	r.clients = r.clients[1:]
	return c, chatModel, nil
}

// toolCalls is a chat turn's script: one assistant message calling the tools
// with the given inputs in order, then a closing message.
func toolCalls(inputs ...string) llm.Client {
	blocks := make([]fake.ScriptedBlock, len(inputs))
	for i, in := range inputs {
		blocks[i] = fake.ScriptedBlock{Type: "tool_use", ToolUseID: fmt.Sprintf("tu_%d", i), ToolName: "unlock", ToolInput: json.RawMessage(in)}
	}
	return fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		{StopReason: "tool_use", Message: fake.ScriptedMessage{Content: blocks}},
		endTurn(fake.ScriptedUsage{}),
	}})
}

// unlockResults returns the turn's unlock_request payloads and tool results.
func unlockResults(evts []llm.Event) ([]llm.UnlockRequestPayload, []llm.ToolResultPayload) {
	var reqs []llm.UnlockRequestPayload
	var results []llm.ToolResultPayload
	for _, ev := range evts {
		switch p := ev.Payload.(type) {
		case llm.UnlockRequestPayload:
			reqs = append(reqs, p)
		case llm.ToolResultPayload:
			results = append(results, p)
		}
	}
	return reqs, results
}

func messageKey(t *testing.T, r llm.ToolResultPayload) string {
	t.Helper()
	var m map[string]string
	require.NoError(t, json.Unmarshal(r.Output, &m))
	return m["message_key"]
}

// B4-AC3: unlock is two-phase; the yes must come in a later turn of the same
// conversation.
func TestTool_UnlockNeedsTheUsersYesInALaterTurn(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx := context.Background()
	p, sid := e.mainSession()
	closure, err := e.plates.Confirm(ctx, plate.RowSet{PlateIDs: []int64{p.ID}})
	require.NoError(t, err)
	other := e.week()
	var second plate.Plate
	for _, q := range other {
		if !slices.Contains(closure.PlateIDs, q.ID) && q.State == plate.StateDraft {
			second = q
			break
		}
	}
	require.NotZero(t, second.ID)
	_, err = e.plates.Confirm(ctx, plate.RowSet{PlateIDs: []int64{second.ID}})
	require.NoError(t, err)

	preview := fmt.Sprintf(`{"plate_id":%d}`, p.ID)
	yes := fmt.Sprintf(`{"plate_id":%d,"user_confirmed":true}`, p.ID)
	res := &scriptResolver{clients: []llm.Client{
		toolCalls(preview, yes),
		toolCalls(yes),
		toolCalls(fmt.Sprintf(`{"plate_id":%d}`, second.ID)),
		toolCalls(fmt.Sprintf(`{"plate_id":%d,"user_confirmed":true}`, second.ID)),
	}}
	svc := agent.NewService(sqlite.NewAIRepo(e.db), res, e.ts, profile.NewService(sqlite.NewProfileRepo(e.db)),
		testsvc.Settings(e.db), planContext(e.db), time.Now)
	turn := func(conv *int64, text string) []llm.Event {
		t.Helper()
		evts, err := chatReq(ctx, svc, agent.ChatRequest{ConversationID: conv, UserText: text, Today: planToday, Start: planStart}, nil)
		require.NoError(t, err)
		return evts
	}
	stateOf := func(id int64) plate.State {
		q, err := e.plates.Get(ctx, id)
		require.NoError(t, err)
		return q.State
	}

	// Turn 1: the preview changes nothing and asks; the yes in the same turn fails.
	evts := turn(nil, "remove the side")
	conv := evts[0].Payload.(llm.ConversationReadyPayload).ConversationID
	reqs, results := unlockResults(evts)
	require.Len(t, reqs, 1)
	require.NotEmpty(t, reqs[0].Rows)
	assert.Equal(t, p.DateString(), reqs[0].Rows[0].Date)
	assert.NotEmpty(t, reqs[0].Rows[0].Names)
	require.Len(t, results, 2)
	assert.False(t, results[0].IsError)
	assert.True(t, results[1].IsError)
	assert.Equal(t, "error.ai.unlock_needs_user", messageKey(t, results[1]))
	assert.Equal(t, plate.StateConfirmed, stateOf(p.ID), "the preview changes nothing")

	// Turn 2, same conversation: the yes unlocks the closure as the agent.
	_, results = unlockResults(turn(&conv, "yes"))
	require.Len(t, results, 1)
	require.False(t, results[0].IsError, string(results[0].Output))
	for _, id := range closure.PlateIDs {
		assert.Equal(t, plate.StateDraft, stateOf(id))
	}
	assert.Equal(t, plate.StateDraft, e.session(sid).State)
	var actor string
	require.NoError(t, e.db.QueryRow(`SELECT actor FROM plate_events WHERE action = 'unlock' AND plate_id = ?`, p.ID).Scan(&actor))
	assert.Equal(t, "agent", actor)

	// A preview in one conversation does not count in another.
	turn(&conv, "unlock the other one")
	_, results = unlockResults(turn(nil, "yes"))
	require.Len(t, results, 1)
	assert.True(t, results[0].IsError)
	assert.Equal(t, "error.ai.unlock_needs_user", messageKey(t, results[0]))
	assert.Equal(t, plate.StateConfirmed, stateOf(second.ID))
}

func TestTool_UnlockOutsideAChatFails(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.mainSession()
	_, err := e.exec("unlock", fmt.Sprintf(`{"plate_id":%d}`, p.ID))
	assert.ErrorIs(t, err, domain.ErrAIUnlockNeedsUser)
}
