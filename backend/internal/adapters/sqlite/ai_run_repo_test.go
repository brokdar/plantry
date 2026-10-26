package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestAIRunRepo_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	started := time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC)
	ws := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	we := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	instr := "more fish"
	id, err := repo.CreateRun(ctx, airun.RunStart{
		Kind:          airun.KindGenerate,
		StartedAt:     started,
		PromptVersion: "v1",
		WindowStart:   &ws,
		WindowEnd:     &we,
		Instruction:   &instr,
	})
	require.NoError(t, err)
	assert.NotZero(t, id)

	require.NoError(t, repo.UpsertPrompt(ctx, "abc123", "system prompt"))
	require.NoError(t, repo.AddStep(ctx, airun.Step{
		RunID: id, Seq: 1, Type: airun.StepLLMCall, Name: "claude-x",
		DurationMs: 1200, InputTokens: 100, OutputTokens: 50,
	}))
	require.NoError(t, repo.AddStep(ctx, airun.Step{
		RunID: id, Seq: 2, Type: airun.StepToolCall, Name: "search_foods",
		ArgsJSON: `{"q":"fish"}`, ResultJSON: `{"error":"boom"}`, IsError: true, DurationMs: 5,
	}))

	var status string
	require.NoError(t, db.QueryRow(`SELECT status FROM ai_runs WHERE id = ?`, id).Scan(&status))
	assert.Equal(t, "running", status)

	finished := started.Add(3 * time.Second)
	require.NoError(t, repo.FinishRun(ctx, airun.RunEnd{
		ID: id, Status: airun.StatusOK, FinishedAt: finished,
		Provider: "anthropic", Model: "claude-x", PromptHash: "abc123",
		Iterations: 1, InputTokens: 100, OutputTokens: 50, CacheReadTokens: 7, CacheWriteTokens: 3,
	}))

	var (
		kind, startedAt, provider, model, promptVersion string
		finishedAt, promptHash, windowStart, windowEnd  sql.NullString
		instruction, errorKey                           sql.NullString
		iterations                                      int
		in, out, cr, cw                                 int64
	)
	require.NoError(t, db.QueryRow(`SELECT kind, started_at, finished_at, status, provider, model,
		prompt_version, prompt_hash, window_start, window_end, instruction, iterations,
		input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, error_key
		FROM ai_runs WHERE id = ?`, id).Scan(&kind, &startedAt, &finishedAt, &status, &provider, &model,
		&promptVersion, &promptHash, &windowStart, &windowEnd, &instruction, &iterations,
		&in, &out, &cr, &cw, &errorKey))
	assert.Equal(t, "generate", kind)
	assert.Equal(t, "2026-03-02 09:30:00", startedAt)
	assert.Equal(t, "2026-03-02 09:30:03", finishedAt.String)
	assert.Equal(t, "ok", status)
	assert.Equal(t, "anthropic", provider)
	assert.Equal(t, "claude-x", model)
	assert.Equal(t, "v1", promptVersion)
	assert.Equal(t, "abc123", promptHash.String)
	assert.Equal(t, "2026-03-02", windowStart.String)
	assert.Equal(t, "2026-03-08", windowEnd.String)
	assert.Equal(t, "more fish", instruction.String)
	assert.Equal(t, 1, iterations)
	assert.Equal(t, []int64{100, 50, 7, 3}, []int64{in, out, cr, cw})
	assert.False(t, errorKey.Valid)

	rows, err := db.Query(`SELECT seq, type, name, args_json, result_json, is_error, duration_ms,
		input_tokens, output_tokens FROM ai_run_steps WHERE run_id = ? ORDER BY seq`, id)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	type stepRow struct {
		seq                int
		typ, name          string
		args, result       sql.NullString
		isErr              bool
		dur, inTok, outTok int64
	}
	var steps []stepRow
	for rows.Next() {
		var s stepRow
		require.NoError(t, rows.Scan(&s.seq, &s.typ, &s.name, &s.args, &s.result, &s.isErr, &s.dur, &s.inTok, &s.outTok))
		steps = append(steps, s)
	}
	require.NoError(t, rows.Err())
	require.Len(t, steps, 2)
	assert.Equal(t, stepRow{seq: 1, typ: "llm_call", name: "claude-x", dur: 1200, inTok: 100, outTok: 50}, steps[0])
	assert.Equal(t, "tool_call", steps[1].typ)
	assert.Equal(t, `{"q":"fish"}`, steps[1].args.String)
	assert.Equal(t, `{"error":"boom"}`, steps[1].result.String)
	assert.True(t, steps[1].isErr)
}

func TestAIRunRepo_CreateRunNullableFields(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: time.Now()})
	require.NoError(t, err)

	var ws, we, instr, hash sql.NullString
	require.NoError(t, db.QueryRow(`SELECT window_start, window_end, instruction, prompt_hash FROM ai_runs WHERE id = ?`, id).
		Scan(&ws, &we, &instr, &hash))
	assert.False(t, ws.Valid)
	assert.False(t, we.Valid)
	assert.False(t, instr.Valid)
	assert.False(t, hash.Valid)
}

func TestAIRunRepo_FinishRunErrorKey(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindLookup, StartedAt: time.Now()})
	require.NoError(t, err)
	require.NoError(t, repo.FinishRun(ctx, airun.RunEnd{
		ID: id, Status: airun.StatusError, FinishedAt: time.Now(), ErrorKey: "error.ai.timeout",
	}))

	var status string
	var errKey, hash sql.NullString
	require.NoError(t, db.QueryRow(`SELECT status, error_key, prompt_hash FROM ai_runs WHERE id = ?`, id).Scan(&status, &errKey, &hash))
	assert.Equal(t, "error", status)
	assert.Equal(t, "error.ai.timeout", errKey.String)
	assert.False(t, hash.Valid, "empty prompt hash is stored as NULL")
}

func TestAIRunRepo_FinishRunUnknownPromptHashStoresNull(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: time.Now()})
	require.NoError(t, err)
	require.NoError(t, repo.FinishRun(ctx, airun.RunEnd{
		ID: id, Status: airun.StatusOK, FinishedAt: time.Now(), PromptHash: "never-upserted",
		Iterations: 2, InputTokens: 10, OutputTokens: 20,
	}))

	var status string
	var hash sql.NullString
	var iterations int
	var in, out int64
	require.NoError(t, db.QueryRow(`SELECT status, prompt_hash, iterations, input_tokens, output_tokens FROM ai_runs WHERE id = ?`, id).
		Scan(&status, &hash, &iterations, &in, &out))
	assert.Equal(t, "ok", status)
	assert.False(t, hash.Valid, "unknown prompt hash is stored as NULL")
	assert.Equal(t, 2, iterations)
	assert.Equal(t, []int64{10, 20}, []int64{in, out})
}

func TestAIRunRepo_FinishRunMissingReturnsNotFound(t *testing.T) {
	repo := sqlite.NewAIRunRepo(testhelper.NewTestDB(t))

	err := repo.FinishRun(context.Background(), airun.RunEnd{ID: 999, Status: airun.StatusOK, FinishedAt: time.Now()})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestAIRunRepo_AddStepUnknownRunIsInvalidInput(t *testing.T) {
	repo := sqlite.NewAIRunRepo(testhelper.NewTestDB(t))

	err := repo.AddStep(context.Background(), airun.Step{RunID: 999, Seq: 1, Type: airun.StepLLMCall, Name: "m"})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestAIRunRepo_UpsertPromptIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	require.NoError(t, repo.UpsertPrompt(ctx, "h1", "prompt text"))
	require.NoError(t, repo.UpsertPrompt(ctx, "h1", "prompt text"))

	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM ai_prompts WHERE hash = 'h1'`).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestAIRunRepo_ResultJSONTruncation(t *testing.T) {
	const limit = 8192
	const marker = "…[truncated]"

	tests := []struct {
		name   string
		result string
		want   string
	}{
		{"short unchanged", `{"ok":true}`, `{"ok":true}`},
		{"exactly limit unchanged", strings.Repeat("a", limit), strings.Repeat("a", limit)},
		{"over limit ascii", strings.Repeat("a", limit+10), strings.Repeat("a", limit) + marker},
		// "é" is 2 bytes; an odd prefix pushes a rune across the 8192 boundary,
		// so the cut must back off one byte.
		{"over limit multibyte", "x" + strings.Repeat("é", limit), "x" + strings.Repeat("é", (limit-1)/2) + marker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := testhelper.NewTestDB(t)
			repo := sqlite.NewAIRunRepo(db)

			id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: time.Now()})
			require.NoError(t, err)
			require.NoError(t, repo.AddStep(ctx, airun.Step{
				RunID: id, Seq: 1, Type: airun.StepToolCall, Name: "t", ResultJSON: tt.result,
			}))

			var got string
			require.NoError(t, db.QueryRow(`SELECT result_json FROM ai_run_steps WHERE run_id = ?`, id).Scan(&got))
			assert.Equal(t, tt.want, got)
			assert.True(t, utf8.ValidString(got))
			assert.LessOrEqual(t, len(strings.TrimSuffix(got, marker)), limit)
		})
	}
}

func TestAIRunRepo_ArgsJSONTruncation(t *testing.T) {
	const limit = 8192
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: time.Now()})
	require.NoError(t, err)
	require.NoError(t, repo.AddStep(ctx, airun.Step{
		RunID: id, Seq: 1, Type: airun.StepToolCall, Name: "t", ArgsJSON: strings.Repeat("a", limit+10),
	}))

	var got string
	require.NoError(t, db.QueryRow(`SELECT args_json FROM ai_run_steps WHERE run_id = ?`, id).Scan(&got))
	assert.Equal(t, strings.Repeat("a", limit)+"…[truncated]", got)
}

func TestAIRunRepo_RunIdentityRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	conv := int64(42)
	withID, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: time.Now(), ConversationID: &conv, RequestID: "host/abc-000001"})
	require.NoError(t, err)
	without, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindLookup, StartedAt: time.Now()})
	require.NoError(t, err)

	var convID sql.NullInt64
	var reqID sql.NullString
	require.NoError(t, db.QueryRow(`SELECT conversation_id, request_id FROM ai_runs WHERE id = ?`, withID).Scan(&convID, &reqID))
	assert.Equal(t, sql.NullInt64{Int64: 42, Valid: true}, convID)
	assert.Equal(t, sql.NullString{String: "host/abc-000001", Valid: true}, reqID)

	require.NoError(t, db.QueryRow(`SELECT conversation_id, request_id FROM ai_runs WHERE id = ?`, without).Scan(&convID, &reqID))
	assert.False(t, convID.Valid)
	assert.False(t, reqID.Valid)
}

func TestAIRunRepo_StepTraceRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: time.Now()})
	require.NoError(t, err)
	// Bodies are capped by the meter, not the repo: 20 KB passes unchanged.
	big := `{"messages":["` + strings.Repeat("a", 20000) + `"]}`
	require.NoError(t, repo.AddStep(ctx, airun.Step{
		RunID: id, Seq: 1, Type: airun.StepLLMCall, Name: "m",
		RequestJSON: big, ResponseJSON: `{"content":[]}`, StopReason: "end_turn",
	}))
	require.NoError(t, repo.AddStep(ctx, airun.Step{RunID: id, Seq: 2, Type: airun.StepToolCall, Name: "t"}))

	var req, resp, stop sql.NullString
	require.NoError(t, db.QueryRow(`SELECT request_json, response_json, stop_reason FROM ai_run_steps WHERE run_id = ? AND seq = 1`, id).
		Scan(&req, &resp, &stop))
	assert.Equal(t, big, req.String)
	assert.Equal(t, `{"content":[]}`, resp.String)
	assert.Equal(t, "end_turn", stop.String)

	require.NoError(t, db.QueryRow(`SELECT request_json, response_json, stop_reason FROM ai_run_steps WHERE run_id = ? AND seq = 2`, id).
		Scan(&req, &resp, &stop))
	assert.False(t, req.Valid)
	assert.False(t, resp.Valid)
	assert.False(t, stop.Valid)
}

func TestAIRunRepo_UpsertToolSchemaIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	require.NoError(t, repo.UpsertToolSchema(ctx, "t1", `[{"name":"a"}]`))
	require.NoError(t, repo.UpsertToolSchema(ctx, "t1", `[{"name":"a"}]`))

	var n int
	var js string
	require.NoError(t, db.QueryRow(`SELECT count(*), max(json) FROM ai_tool_schemas WHERE hash = 't1'`).Scan(&n, &js))
	assert.Equal(t, 1, n)
	assert.Equal(t, `[{"name":"a"}]`, js)
}

func TestAIRunRepo_PruneSteps(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	addRun := func(age time.Duration) int64 {
		id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: now.Add(-age)})
		require.NoError(t, err)
		for seq := 1; seq <= 2; seq++ {
			require.NoError(t, repo.AddStep(ctx, airun.Step{RunID: id, Seq: seq, Type: airun.StepLLMCall, Name: "m", RequestJSON: `{}`}))
		}
		return id
	}
	old := addRun(200 * 24 * time.Hour)
	recent := addRun(10 * 24 * time.Hour)
	require.NoError(t, repo.UpsertPrompt(ctx, "p", "prompt"))

	n, err := repo.PruneSteps(ctx, now.Add(-180*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	countSteps := func(runID int64) int {
		var c int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM ai_run_steps WHERE run_id = ?`, runID).Scan(&c))
		return c
	}
	assert.Zero(t, countSteps(old))
	assert.Equal(t, 2, countSteps(recent))

	var runs, prompts int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM ai_runs`).Scan(&runs))
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM ai_prompts`).Scan(&prompts))
	assert.Equal(t, 2, runs, "runs survive the prune")
	assert.Equal(t, 1, prompts)

	n, err = repo.PruneSteps(ctx, now.Add(-180*24*time.Hour))
	require.NoError(t, err)
	assert.Zero(t, n)
}
