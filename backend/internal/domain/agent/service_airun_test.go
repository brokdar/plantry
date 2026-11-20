package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

const chatModel = "fake-model"

// newChatService wires agent.Service against a real SQLite DB, with the fake
// client metered by a recorder writing to the same DB.
func newChatService(t *testing.T, script *fake.Script) (*agent.Service, *sql.DB) {
	t.Helper()
	return newChatServiceWith(t, script, nil)
}

// newChatServiceWith is newChatService with the plan reader wrapped by wrap
// (nil keeps it): call is the 1-based number of the Build call, the prompt's
// read being the first and the closing `after` read the second.
func newChatServiceWith(t *testing.T, script *fake.Script, wrap func(call int, st planning.State, err error) (planning.State, error)) (*agent.Service, *sql.DB) {
	t.Helper()
	return buildChatService(t, fake.NewFromScript(script), wrap)
}

// newChatServiceWithClient is newChatService over a client the test keeps, to
// read its recorded requests.
func newChatServiceWithClient(t *testing.T, fc *fake.Client) (*agent.Service, *sql.DB) {
	t.Helper()
	return buildChatService(t, fc, nil)
}

func buildChatService(t *testing.T, fc llm.Client, wrap func(call int, st planning.State, err error) (planning.State, error)) (*agent.Service, *sql.DB) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	client := rec.Meter(fc, "fake")

	foodRepo := sqlite.NewFoodRepo(db)
	tools, err := agent.NewToolSet(agent.Services{
		Foods:             food.NewService(foodRepo),
		NutritionResolver: food.NewNutritionResolver(foodRepo),
		Settings:          testsvc.Settings(db),
	})
	require.NoError(t, err)

	svc := agent.NewService(
		sqlite.NewAIRepo(db),
		llm.StaticResolver(client, chatModel),
		tools,
		profile.NewService(sqlite.NewProfileRepo(db)),
		testsvc.Settings(db),
		wrapPlan(planContext(db), wrap),
		time.Now,
	).WithRecorder(rec)
	return svc, db
}

func wrapPlan(pc agent.PlanContext, wrap func(call int, st planning.State, err error) (planning.State, error)) agent.PlanContext {
	if wrap == nil {
		return pc
	}
	calls := 0
	build := pc.Build
	pc.Build = func(ctx context.Context, w window.Window, today, from, to time.Time) (planning.State, error) {
		calls++
		st, err := build(ctx, w, today, from, to)
		return wrap(calls, st, err)
	}
	return pc
}

// chatWindowStart is the planning window chat() sends (a Monday, the default
// shopping day).
var chatWindowStart = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

// chat runs one Chat turn over the default window, draining events. onEvent
// (optional) sees each event as it arrives.
func chat(ctx context.Context, svc *agent.Service, text string, onEvent func(llm.Event)) ([]llm.Event, error) {
	return chatReq(ctx, svc, agent.ChatRequest{UserText: text, Today: chatWindowStart, Start: chatWindowStart}, onEvent)
}

// chatReq runs one Chat turn for req, draining events.
func chatReq(ctx context.Context, svc *agent.Service, req agent.ChatRequest, onEvent func(llm.Event)) ([]llm.Event, error) {
	out := make(chan llm.Event, 32)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		errCh <- svc.Chat(ctx, req, out)
	}()
	var evts []llm.Event
	for e := range out {
		if onEvent != nil {
			onEvent(e)
		}
		evts = append(evts, e)
	}
	return evts, <-errCh
}

type runRow struct {
	Kind, Provider, Model, PromptVersion, Status string
	Instruction, ErrorKey                        sql.NullString
	WindowStart, WindowEnd                       sql.NullString
	Iterations                                   int
	InputTokens, OutputTokens                    int64
	CacheReadTokens, CacheWriteTokens            int64
	FinishedAt                                   sql.NullString
}

type stepRow struct {
	Seq          int
	Type, Name   string
	IsError      bool
	InputTokens  int64
	OutputTokens int64
}

// onlyRun asserts there is exactly one ai_runs row and returns it with its
// steps in seq order.
func onlyRun(t *testing.T, db *sql.DB) (runRow, []stepRow) {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&n))
	require.Equal(t, 1, n, "want exactly one ai_runs row")

	var r runRow
	var id int64
	require.NoError(t, db.QueryRow(`SELECT id, kind, provider, model, prompt_version, status,
		instruction, error_key, window_start, window_end, iterations, input_tokens, output_tokens,
		cache_read_tokens, cache_write_tokens, finished_at FROM ai_runs`).Scan(
		&id, &r.Kind, &r.Provider, &r.Model, &r.PromptVersion, &r.Status,
		&r.Instruction, &r.ErrorKey, &r.WindowStart, &r.WindowEnd, &r.Iterations, &r.InputTokens, &r.OutputTokens,
		&r.CacheReadTokens, &r.CacheWriteTokens, &r.FinishedAt))

	rows, err := db.Query(`SELECT seq, type, name, is_error, input_tokens, output_tokens
		FROM ai_run_steps WHERE run_id = ? ORDER BY seq`, id)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var steps []stepRow
	for rows.Next() {
		var s stepRow
		require.NoError(t, rows.Scan(&s.Seq, &s.Type, &s.Name, &s.IsError, &s.InputTokens, &s.OutputTokens))
		steps = append(steps, s)
	}
	require.NoError(t, rows.Err())
	return r, steps
}

// withoutEvaluations drops the plan.evaluation steps, so a test can assert
// the model and tool steps alone.
func withoutEvaluations(steps []stepRow) []stepRow {
	var out []stepRow
	for _, s := range steps {
		if s.Name != planning.EvaluationStep {
			out = append(out, s)
		}
	}
	for i := range out {
		out[i].Seq = i + 1
	}
	return out
}

// evaluationPhases are the phases of the only run's plan.evaluation steps in
// seq order; each result must decode as a planning.Evaluation.
func evaluationPhases(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT args_json, result_json FROM ai_run_steps WHERE name = ? ORDER BY seq`, planning.EvaluationStep)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var phases []string
	for rows.Next() {
		var args, result string
		require.NoError(t, rows.Scan(&args, &result))
		var a struct {
			Phase string `json:"phase"`
		}
		require.NoError(t, json.Unmarshal([]byte(args), &a))
		var ev planning.Evaluation
		require.NoError(t, json.Unmarshal([]byte(result), &ev))
		phases = append(phases, a.Phase)
	}
	require.NoError(t, rows.Err())
	return phases
}

func toolUseTurn(usage fake.ScriptedUsage, names ...string) fake.Turn {
	blocks := make([]fake.ScriptedBlock, len(names))
	for i, n := range names {
		blocks[i] = fake.ScriptedBlock{Type: "tool_use", ToolUseID: "tu_" + n, ToolName: n, ToolInput: json.RawMessage(`{}`)}
	}
	return fake.Turn{StopReason: "tool_use", Message: fake.ScriptedMessage{Content: blocks}, Usage: usage}
}

func endTurn(usage fake.ScriptedUsage) fake.Turn {
	return fake.Turn{
		StopReason: "end_turn",
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: "Done."}}},
		Usage:      usage,
	}
}

func TestChat_RecordsChatRun(t *testing.T) {
	t.Parallel()
	svc, db := newChatService(t, &fake.Script{Turns: []fake.Turn{
		toolUseTurn(fake.ScriptedUsage{InputTokens: 100, OutputTokens: 10, CacheReadTokens: 40, CacheWriteTokens: 4}, "find_foods", "no_such_tool"),
		endTurn(fake.ScriptedUsage{InputTokens: 200, OutputTokens: 20, CacheReadTokens: 60, CacheWriteTokens: 6}),
	}})

	evts, err := chat(context.Background(), svc, "plan my dinners", nil)
	require.NoError(t, err)
	last := evts[len(evts)-1]
	require.Equal(t, llm.EventDone, last.Type)
	assert.Equal(t, llm.Usage{InputTokens: 300, OutputTokens: 30, CacheReadTokens: 100, CacheWriteTokens: 10},
		last.Payload.(llm.DonePayload).Usage)

	run, all := onlyRun(t, db)
	steps := withoutEvaluations(all)
	assert.Equal(t, "chat", run.Kind)
	assert.Equal(t, "fake", run.Provider)
	assert.Equal(t, chatModel, run.Model)
	assert.Equal(t, agent.PromptVersion, run.PromptVersion)
	assert.Equal(t, "ok", run.Status)
	assert.Equal(t, "plan my dinners", run.Instruction.String)
	assert.Equal(t, "2026-11-02", run.WindowStart.String)
	assert.Equal(t, "2026-11-08", run.WindowEnd.String)
	assert.False(t, run.ErrorKey.Valid)
	assert.True(t, run.FinishedAt.Valid)
	assert.Equal(t, 2, run.Iterations)
	assert.Equal(t, int64(300), run.InputTokens)
	assert.Equal(t, int64(30), run.OutputTokens)
	assert.Equal(t, int64(100), run.CacheReadTokens)
	assert.Equal(t, int64(10), run.CacheWriteTokens)

	assert.Equal(t, []stepRow{
		{Seq: 1, Type: "llm_call", Name: chatModel, InputTokens: 100, OutputTokens: 10},
		{Seq: 2, Type: "tool_call", Name: "find_foods"},
		{Seq: 3, Type: "tool_call", Name: "no_such_tool", IsError: true},
		{Seq: 4, Type: "llm_call", Name: chatModel, InputTokens: 200, OutputTokens: 20},
	}, steps)
}

// A chat run records the plan before the turn and after it, also when the
// turn is cancelled.
func TestChat_RecordsPlanEvaluationBeforeAndAfter(t *testing.T) {
	t.Parallel()
	svc, db := newChatService(t, &fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{InputTokens: 1})}})
	_, err := chat(context.Background(), svc, "hi", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"before", "after"}, evaluationPhases(t, db))
	_, steps := onlyRun(t, db)
	assert.Equal(t, planning.EvaluationStep, steps[0].Name, "before is the first step")
	assert.Equal(t, planning.EvaluationStep, steps[len(steps)-1].Name, "after is the last step")
}

func TestChat_RecordsPlanEvaluationOnCancel(t *testing.T) {
	t.Parallel()
	svc, db := newChatService(t, &fake.Script{Turns: []fake.Turn{{
		StopReason: "end_turn",
		Events: []fake.ScriptedEvent{
			{Type: string(llm.EventAssistantDelta), Payload: json.RawMessage(`{"text":"early"}`)},
			{DelayMs: 10_000, Type: string(llm.EventAssistantDelta), Payload: json.RawMessage(`{"text":"late"}`)},
		},
	}}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := chat(ctx, svc, "hi", func(e llm.Event) {
		if e.Type == llm.EventAssistantDelta {
			cancel()
		}
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"before", "after"}, evaluationPhases(t, db))
}

func TestChat_IterationsExceededStatus(t *testing.T) {
	t.Parallel()
	// The fake replays the last (tool_use) turn forever.
	svc, db := newChatService(t, &fake.Script{Turns: []fake.Turn{
		toolUseTurn(fake.ScriptedUsage{InputTokens: 1, OutputTokens: 1}, "find_foods"),
	}})

	_, err := chat(context.Background(), svc, "loop forever", nil)
	require.ErrorIs(t, err, agent.ErrIterationsExceeded)

	run, all := onlyRun(t, db)
	steps := withoutEvaluations(all)
	assert.Equal(t, "iterations_exceeded", run.Status)
	assert.Equal(t, agent.MaxIterations, run.Iterations)
	assert.Equal(t, int64(agent.MaxIterations), run.InputTokens)
	var llmCalls, toolCalls int
	for _, s := range steps {
		switch s.Type {
		case "llm_call":
			llmCalls++
		case "tool_call":
			toolCalls++
		}
	}
	assert.Equal(t, agent.MaxIterations, llmCalls)
	assert.Equal(t, agent.MaxIterations, toolCalls)
}

func TestChat_ErrorStatusAndKey(t *testing.T) {
	t.Parallel()
	// An empty script makes every Stream call fail.
	svc, db := newChatService(t, &fake.Script{})

	_, err := chat(context.Background(), svc, "hi", nil)
	require.Error(t, err)

	run, all := onlyRun(t, db)
	steps := withoutEvaluations(all)
	assert.Equal(t, "error", run.Status)
	assert.Equal(t, "error.ai.stream_interrupted", run.ErrorKey.String)
	require.Len(t, steps, 1)
	assert.True(t, steps[0].IsError)
	// The turn failed, but the plan after it was still recorded.
	assert.Equal(t, []string{"before", "after"}, evaluationPhases(t, db))
}

func TestChat_CancelledStatus(t *testing.T) {
	t.Parallel()
	// The test cancels on the first delta, once the loop (and so the run) has
	// started; the second scripted event waits long enough for that.
	svc, db := newChatService(t, &fake.Script{Turns: []fake.Turn{{
		StopReason: "end_turn",
		Events: []fake.ScriptedEvent{
			{Type: string(llm.EventAssistantDelta), Payload: json.RawMessage(`{"text":"early"}`)},
			{DelayMs: 10_000, Type: string(llm.EventAssistantDelta), Payload: json.RawMessage(`{"text":"late"}`)},
		},
	}}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := chat(ctx, svc, "hi", func(e llm.Event) {
		if e.Type == llm.EventAssistantDelta {
			cancel()
		}
	})
	require.ErrorIs(t, err, context.Canceled)

	run, _ := onlyRun(t, db)
	assert.Equal(t, "cancelled", run.Status)
	assert.True(t, run.FinishedAt.Valid)
}

func TestChat_NoRecorderRecordsNothing(t *testing.T) {
	t.Parallel()
	db := testhelper.NewTestDB(t)
	foodRepo := sqlite.NewFoodRepo(db)
	tools, err := agent.NewToolSet(agent.Services{Foods: food.NewService(foodRepo)})
	require.NoError(t, err)
	svc := agent.NewService(sqlite.NewAIRepo(db),
		llm.StaticResolver(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{})}}), chatModel),
		tools, profile.NewService(sqlite.NewProfileRepo(db)), testsvc.Settings(db), planContext(db), time.Now)

	_, err = chat(context.Background(), svc, "hi", nil)
	require.NoError(t, err)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&n))
	assert.Zero(t, n)
}

func TestChat_ProviderMissingRecordsNothing(t *testing.T) {
	t.Parallel()
	db := testhelper.NewTestDB(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	tools, err := agent.NewToolSet(agent.Services{Foods: food.NewService(sqlite.NewFoodRepo(db))})
	require.NoError(t, err)
	svc := agent.NewService(sqlite.NewAIRepo(db), llm.StaticResolver(nil, ""), tools,
		profile.NewService(sqlite.NewProfileRepo(db)), testsvc.Settings(db), planContext(db), time.Now).WithRecorder(rec)

	_, err = chat(context.Background(), svc, "hi", nil)
	require.Error(t, err)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&n))
	assert.Zero(t, n)
}

func TestChat_RunStoresConversationID(t *testing.T) {
	t.Parallel()
	svc, db := newChatService(t, &fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{})}})

	evts, err := chat(context.Background(), svc, "first", nil)
	require.NoError(t, err)
	var convID int64
	for _, e := range evts {
		if e.Type == llm.EventConversationReady {
			convID = e.Payload.(llm.ConversationReadyPayload).ConversationID
		}
	}
	require.NotZero(t, convID)

	// A follow-up turn in the same conversation is a second run with the same id.
	_, err = chatReq(context.Background(), svc, agent.ChatRequest{
		ConversationID: &convID, UserText: "second", Today: chatWindowStart, Start: chatWindowStart,
	}, nil)
	require.NoError(t, err)

	rows, err := db.Query(`SELECT conversation_id FROM ai_runs ORDER BY id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var got []sql.NullInt64
	for rows.Next() {
		var c sql.NullInt64
		require.NoError(t, rows.Scan(&c))
		got = append(got, c)
	}
	require.NoError(t, rows.Err())
	want := sql.NullInt64{Int64: convID, Valid: true}
	assert.Equal(t, []sql.NullInt64{want, want}, got)

	// The chat's tool set is stored once, though both runs sent it.
	var schemas int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_tool_schemas`).Scan(&schemas))
	assert.Equal(t, 1, schemas)
}

func evaluationSteps(t *testing.T, db *sql.DB) (results []planning.Evaluation, errs []bool) {
	t.Helper()
	rows, err := db.Query(`SELECT result_json, is_error FROM ai_run_steps WHERE name = ? ORDER BY seq`, planning.EvaluationStep)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var result string
		var isErr bool
		require.NoError(t, rows.Scan(&result, &isErr))
		var ev planning.Evaluation
		_ = json.Unmarshal([]byte(result), &ev)
		results, errs = append(results, ev), append(errs, isErr)
	}
	require.NoError(t, rows.Err())
	return results, errs
}

// before and after describe the plan at their moment: the plan that changed
// during the turn shows in the pair.
func TestChat_EvaluationBeforeAndAfterDiffer(t *testing.T) {
	t.Parallel()
	limit := 1
	svc, db := newChatServiceWith(t, &fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{InputTokens: 1})}},
		func(call int, st planning.State, err error) (planning.State, error) {
			if call > 1 { // the plan changed during the turn
				st.Profile.MeatDaysPerWeekMax = &limit
			}
			return st, err
		})
	_, err := chat(context.Background(), svc, "hi", nil)
	require.NoError(t, err)
	evs, errs := evaluationSteps(t, db)
	require.Len(t, evs, 2)
	assert.Equal(t, []bool{false, false}, errs)
	assert.Nil(t, evs[0].MeatDaysMax)
	require.NotNil(t, evs[1].MeatDaysMax)
	assert.Equal(t, 1, *evs[1].MeatDaysMax)
}

// A plan that cannot be read when the turn ends leaves a failed after step.
func TestChat_EvaluationAfterReadErrorIsRecorded(t *testing.T) {
	t.Parallel()
	svc, db := newChatServiceWith(t, &fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{InputTokens: 1})}},
		func(call int, st planning.State, err error) (planning.State, error) {
			if call > 1 {
				return st, errors.New("db gone")
			}
			return st, err
		})
	_, err := chat(context.Background(), svc, "hi", nil)
	require.NoError(t, err)
	_, errs := evaluationSteps(t, db)
	assert.Equal(t, []bool{false, true}, errs)
	var result string
	require.NoError(t, db.QueryRow(`SELECT result_json FROM ai_run_steps WHERE name = ? AND is_error = 1`, planning.EvaluationStep).Scan(&result))
	assert.Contains(t, result, "db gone")
}
