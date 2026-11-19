package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// E015: what a chat leaves behind when the provider fails, refuses, is cut
// off or the user goes away. The requirement for every case: the plan is
// unchanged or fully changed, the user gets a message key the frontend can
// translate, and the run log holds the status, the error key and the steps.

// apiErr is a provider error as the adapters return it.
type apiErr struct{ status int }

func (e apiErr) Error() string {
	return fmt.Sprintf("anthropic: %d some_error: raw provider text req_0123", e.status)
}
func (e apiErr) ProviderStatus() int     { return e.status }
func (apiErr) ProviderErrorType() string { return "some_error" }
func (e apiErr) ProviderInfo() llm.ProviderErrorInfo {
	return llm.ProviderErrorInfo{Status: e.status, RequestID: "req_0123"}
}

// failOn fails the n-th Stream call (1-based) with err and passes every other
// call to the client it wraps.
type failOn struct {
	llm.Client
	n     int
	err   error
	calls int
}

func (c *failOn) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	if c.calls++; c.calls == c.n {
		close(out)
		return nil, c.err
	}
	return c.Client.Stream(ctx, req, out)
}

// providerFailures are the first-call failures of case 1 with the key the
// user is shown.
var providerFailures = []struct {
	name string
	err  error
	key  string
}{
	{"generic", errors.New("read tcp 10.0.0.2:5555: connection reset by peer"), "error.ai.stream_interrupted"},
	{"rate limit", apiErr{429}, "error.ai.rate_limited"},
	{"timeout", fmt.Errorf("post \"https://api.example/v1/messages\": %w", context.DeadlineExceeded), "error.ai.timeout"},
	{"key rejected", apiErr{401}, "error.ai.key_rejected"},
}

// i18nHas reports whether the frontend has text for key in both languages.
func i18nHas(t *testing.T, key string) bool {
	t.Helper()
	for _, lang := range []string{"en", "de"} {
		raw, err := os.ReadFile("../../../../frontend/src/lib/i18n/" + lang + ".json")
		require.NoError(t, err)
		var node any
		require.NoError(t, json.Unmarshal(raw, &node))
		for _, part := range strings.Split(key, ".") {
			m, _ := node.(map[string]any)
			var ok bool
			if node, ok = m[part]; !ok {
				return false
			}
		}
		if _, ok := node.(string); !ok {
			return false
		}
	}
	return true
}

// failureEvent returns the only error event of a stream.
func failureEvent(t *testing.T, evts []llm.Event) llm.StreamErrorPayload {
	t.Helper()
	var errs []llm.StreamErrorPayload
	for _, e := range evts {
		if e.Type == llm.EventError {
			errs = append(errs, e.Payload.(llm.StreamErrorPayload))
		}
	}
	require.Len(t, errs, 1, "one error event")
	assert.True(t, i18nHas(t, errs[0].MessageKey), "the frontend translates %s", errs[0].MessageKey)
	return errs[0]
}

// chatRunSteps returns the chat run's status, error key and its steps as
// "type:name" (":error" suffix for a failed step), without the plan
// evaluations.
func chatRunSteps(t *testing.T, db *sql.DB) (status, errKey string, steps []string) {
	t.Helper()
	var id int64
	var key sql.NullString
	require.NoError(t, db.QueryRow(`SELECT id, status, error_key FROM ai_runs WHERE kind = 'chat'`).Scan(&id, &status, &key))
	rows, err := db.Query(`SELECT type, name, is_error FROM ai_run_steps WHERE run_id = ? AND name != 'plan.evaluation' ORDER BY seq`, id)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var typ, name string
		var isErr bool
		require.NoError(t, rows.Scan(&typ, &name, &isErr))
		s := typ + ":" + name
		if isErr {
			s += ":error"
		}
		steps = append(steps, s)
	}
	require.NoError(t, rows.Err())
	var fin sql.NullString
	require.NoError(t, db.QueryRow(`SELECT finished_at FROM ai_runs WHERE id = ?`, id).Scan(&fin))
	assert.True(t, fin.Valid, "the run is finished")
	return status, key.String, steps
}

func count(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(q).Scan(&n))
	return n
}

// Case 1: the provider fails on the first call. Nothing is planned, the user
// gets a translatable key, the run log says what happened.
func TestChatFailure_ProviderErrorOnFirstCall(t *testing.T) {
	for _, tc := range providerFailures {
		t.Run(tc.name, func(t *testing.T) {
			client := &failOn{Client: fake.NewFromScript(&fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{})}}), n: 1, err: tc.err}
			svc, db := buildChatService(t, client, nil)

			evts, err := chat(context.Background(), svc, "plan my week", nil)

			require.Error(t, err)
			ev := failureEvent(t, evts)
			assert.Equal(t, tc.key, ev.MessageKey)
			// What the user can read, live and in the stored transcript, holds
			// no provider text, address or id.
			var stored string
			require.NoError(t, db.QueryRow(`SELECT content FROM ai_messages WHERE role = 'error'`).Scan(&stored))
			for _, text := range []string{ev.Message, stored} {
				assert.NotEmpty(t, text)
				for _, jargon := range []string{"anthropic", "req_0123", "some_error", "connection reset", "10.0.0.2", "api.example", "429", "401"} {
					assert.NotContains(t, text, jargon)
				}
			}
			status, key, steps := chatRunSteps(t, db)
			assert.Equal(t, "error", status)
			assert.Equal(t, tc.key, key)
			assert.Equal(t, []string{"llm_call:" + chatModel + ":error"}, steps)
			assert.Zero(t, count(t, db, `SELECT COUNT(*) FROM plates`))
		})
	}
}

// chatOverPlan builds a chat service over the plan-tool fixture (a generated
// week of drafts), so tools write real rows.
func (e *planToolEnv) chatOverPlan(c llm.Client, tx domain.Transactor) *agent.Service {
	e.t.Helper()
	foods := sqlite.NewFoodRepo(e.db)
	ts, err := agent.NewToolSet(agent.Services{
		Foods: food.NewService(foods), NutritionResolver: food.NewNutritionResolver(foods),
		Plates: e.plates, Profile: profile.NewService(sqlite.NewProfileRepo(e.db)),
		Slots: slot.NewService(sqlite.NewSlotRepo(e.db)), Settings: testsvc.Settings(e.db),
		PlanView: testsvc.PlanView(e.db), Planner: e.planner, Cells: e.avail, Availability: e.avail,
		Changes: sqlite.NewAIChangeRepo(e.db),
		State:   testsvc.PlanState(e.db), Tx: tx, Light: sqlite.NewPlanLightRepo(e.db), Undo: e.changes,
	})
	require.NoError(e.t, err)
	return agent.NewService(sqlite.NewAIRepo(e.db), llm.StaticResolver(e.rec.Meter(c, "fake"), chatModel), ts,
		profile.NewService(sqlite.NewProfileRepo(e.db)), testsvc.Settings(e.db), planContext(e.db),
		func() time.Time { return planToday }).WithRecorder(e.rec)
}

// planChat runs one chat turn over the fixture's window.
func (e *planToolEnv) planChat(ctx context.Context, svc *agent.Service, text string) ([]llm.Event, error) {
	return chatReq(ctx, svc, agent.ChatRequest{UserText: text, Today: planToday, Start: planStart}, nil)
}

func toolTurn(id, name, input string) fake.Turn {
	return fake.Turn{
		StopReason: "tool_use",
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "tool_use", ToolUseID: id, ToolName: name, ToolInput: json.RawMessage(input)}}},
		Usage:      fake.ScriptedUsage{InputTokens: 10, OutputTokens: 1},
	}
}

// eatersOf is the eaters of the plate id.
func (e *planToolEnv) eatersOf(id int64) float64 {
	e.t.Helper()
	p, err := e.plates.Get(context.Background(), id)
	require.NoError(e.t, err)
	return p.Eaters
}

// Case 2: the provider fails after the first tool call. The write the tool
// made is complete and recorded as one undoable change; the run log has the
// call and the failed second one.
func TestChatFailure_ProviderFailsAfterAWrite(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.freshMain()
	want := p.Eaters + 2
	script := &fake.Script{Turns: []fake.Turn{
		toolTurn("tu_1", "apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_eaters","plate_id":%d,"eaters":%v}]}`, p.ID, want)),
		endTurn(fake.ScriptedUsage{}),
	}}
	client := &failOn{Client: fake.NewFromScript(script), n: 2, err: apiErr{500}}
	changesBefore := count(t, e.db, `SELECT COUNT(*) FROM ai_changes`)

	evts, err := e.planChat(context.Background(), e.chatOverPlan(client, sqlite.NewTxRunner(e.db)), "more eaters")

	require.Error(t, err)
	failureEvent(t, evts)
	assert.InDelta(t, want, e.eatersOf(p.ID), 1e-9, "the write is whole")
	assert.Equal(t, changesBefore+1, count(t, e.db, `SELECT COUNT(*) FROM ai_changes`), "and undoable")
	status, key, steps := chatRunSteps(t, e.db)
	assert.Equal(t, "error", status)
	assert.Equal(t, "error.ai.provider_error", key)
	assert.Equal(t, []string{"llm_call:" + chatModel, "tool_call:apply_changes", "llm_call:" + chatModel + ":error"}, steps)
}

// Case 4: a reply cut off at the output limit during a tool call runs nothing
// and the run goes on.
func TestChatFailure_CutOffDuringAToolCall(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.freshMain()
	cut := toolTurn("tu_cut", "apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_eaters","plate_id":%d,"eaters":%v}]}`, p.ID, p.Eaters+2))
	cut.StopReason = "max_tokens"
	script := &fake.Script{Turns: []fake.Turn{cut, endTurn(fake.ScriptedUsage{})}}
	changesBefore := count(t, e.db, `SELECT COUNT(*) FROM ai_changes`)

	evts, err := e.planChat(context.Background(), e.chatOverPlan(fake.NewFromScript(script), sqlite.NewTxRunner(e.db)), "more eaters")

	require.NoError(t, err)
	assert.Equal(t, llm.EventDone, evts[len(evts)-1].Type)
	assert.InDelta(t, p.Eaters, e.eatersOf(p.ID), 1e-9, "nothing was run")
	assert.Equal(t, changesBefore, count(t, e.db, `SELECT COUNT(*) FROM ai_changes`))
	status, _, steps := chatRunSteps(t, e.db)
	assert.Equal(t, "ok", status)
	assert.Equal(t, []string{"llm_call:" + chatModel, "tool_call:apply_changes:error", "llm_call:" + chatModel}, steps)
}

// cancelBeforeCommit is a unit of work that cancels the request after its
// function ran and before the transaction commits: the user goes away while a
// write tool is executing.
type cancelBeforeCommit struct {
	domain.Transactor
	cancel context.CancelFunc
}

func (c cancelBeforeCommit) InTx(ctx context.Context, fn func(context.Context) error) error {
	return c.Transactor.InTx(ctx, func(ctx context.Context) error {
		err := fn(ctx)
		c.cancel()
		return err
	})
}

// Case 5: the request is cancelled while apply_changes writes. The batch
// rolls back whole, no change is recorded, and the run ends cancelled with
// its steps and the plan after.
func TestChatFailure_CancelWhileAWriteToolRuns(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.freshMain()
	script := &fake.Script{Turns: []fake.Turn{
		toolTurn("tu_1", "apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_eaters","plate_id":%d,"eaters":%v}]}`, p.ID, p.Eaters+2)),
		endTurn(fake.ScriptedUsage{}),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := e.chatOverPlan(fake.NewFromScript(script), cancelBeforeCommit{sqlite.NewTxRunner(e.db), cancel})
	changesBefore := count(t, e.db, `SELECT COUNT(*) FROM ai_changes`)
	eventsBefore := e.eventCount()

	_, err := e.planChat(ctx, svc, "more eaters")

	require.Error(t, err)
	assert.InDelta(t, p.Eaters, e.eatersOf(p.ID), 1e-9, "the write rolled back")
	assert.Equal(t, changesBefore, count(t, e.db, `SELECT COUNT(*) FROM ai_changes`))
	assert.Equal(t, eventsBefore, e.eventCount(), "no plate event is left")
	status, _, steps := chatRunSteps(t, e.db)
	assert.Equal(t, "cancelled", status)
	assert.Contains(t, steps, "tool_call:apply_changes:error")
	assert.Equal(t, 2, count(t, e.db, `SELECT COUNT(*) FROM ai_run_steps WHERE name = 'plan.evaluation'
		AND run_id = (SELECT id FROM ai_runs WHERE kind = 'chat') AND is_error = 0`), "the plan before and after is recorded")
}

// Case 6: the plan changes between the dry run and the write. The write
// checks the live plan again: a plate confirmed in between is refused and
// the batch rolls back whole; a plate the user changed (still a draft) is
// written over, and the result shows the plan as it is now.
func TestChatFailure_PlanChangesBetweenDryRunAndWrite(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	week := e.week()
	var a, b plate.Plate
	for _, p := range week {
		if p.State != plate.StateDraft {
			continue
		}
		if a.ID == 0 {
			a = p
		} else if b.ID == 0 && p.ID != a.ID {
			b = p
		}
	}
	require.NotZero(t, b.ID)
	input := fmt.Sprintf(`{"ops":[{"op":"set_eaters","plate_id":%d,"eaters":3},{"op":"set_eaters","plate_id":%d,"eaters":3}]}`, a.ID, b.ID)
	dry := strings.Replace(input, `{"ops"`, `{"dry_run":true,"ops"`, 1)
	_, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(dry))
	require.NoError(t, err)

	t.Run("confirmed in between", func(t *testing.T) {
		_, err := e.plates.Confirm(context.Background(), plate.RowSet{PlateIDs: []int64{b.ID}})
		require.NoError(t, err)
		before := e.dbCounts()

		_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(input))

		require.ErrorIs(t, err, domain.ErrRowConfirmed)
		assert.Equal(t, before, e.dbCounts())
		assert.InDelta(t, a.Eaters, e.eatersOf(a.ID), 1e-9, "the first op did not stay")
	})
	t.Run("removed in between", func(t *testing.T) {
		_, err := e.plates.Delete(context.Background(), a.ID)
		require.NoError(t, err)
		before := e.dbCounts()

		_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(input))

		require.ErrorIs(t, err, domain.ErrNotFound)
		assert.Equal(t, before, e.dbCounts())
	})
}
