package airun_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// replayStep is one llm_call step read back from the run log.
type replayStep struct {
	request  llm.Request
	response *responseBody // nil when the call failed
}

// reconstruct rebuilds the llm.Request of every llm_call step of a run, in
// seq order. Call i's messages are call i-1's messages cut at its
// message_offset plus its delta; the system prompt and tools are looked up
// by hash. Recorded responses are returned separately and never inserted:
// callers append assistant replies to their history themselves, so those
// replies are already part of the deltas.
func reconstruct(t *testing.T, db *sql.DB, runID int64) []replayStep {
	t.Helper()
	// Read all steps first: the test DB has one connection, so the prompt and
	// tool lookups below cannot run while rows is open.
	type stored struct {
		model, request string
		response       sql.NullString
	}
	var steps []stored
	rows, err := db.Query(`SELECT name, request_json, response_json FROM ai_run_steps
		WHERE run_id = ? AND type = 'llm_call' ORDER BY seq`, runID)
	require.NoError(t, err)
	for rows.Next() {
		var s stored
		require.NoError(t, rows.Scan(&s.model, &s.request, &s.response))
		steps = append(steps, s)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	var out []replayStep
	var history []llm.Message
	for _, s := range steps {
		model, reqJSON, respJSON := s.model, s.request, s.response

		var body requestBody
		require.NoError(t, json.Unmarshal([]byte(reqJSON), &body))
		require.False(t, body.Truncated, "a truncated request cannot be replayed")
		require.LessOrEqual(t, body.MessageOffset, len(history))
		history = append(append([]llm.Message{}, history[:body.MessageOffset]...), body.Messages...)

		req := llm.Request{
			Model:       model,
			Messages:    history,
			ToolChoice:  body.ToolChoice,
			Temperature: body.Temperature,
			MaxTokens:   body.MaxTokens,
		}
		if body.SystemHash != nil {
			require.NoError(t, db.QueryRow(`SELECT text FROM ai_prompts WHERE hash = ?`, *body.SystemHash).Scan(&req.System))
		}
		if body.ToolsHash != nil {
			var toolsJSON string
			require.NoError(t, db.QueryRow(`SELECT json FROM ai_tool_schemas WHERE hash = ?`, *body.ToolsHash).Scan(&toolsJSON))
			require.NoError(t, json.Unmarshal([]byte(toolsJSON), &req.Tools))
		}

		step := replayStep{request: req}
		if respJSON.Valid {
			var resp responseBody
			require.NoError(t, json.Unmarshal([]byte(respJSON.String), &resp))
			step.response = &resp
		}
		out = append(out, step)
	}
	return out
}

func onlyRunID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int
	var id int64
	require.NoError(t, db.QueryRow(`SELECT COUNT(*), MAX(id) FROM ai_runs`).Scan(&n, &id))
	require.Equal(t, 1, n)
	return id
}

// replyTurn is a scripted text reply. The fake rewinds after a terminal turn,
// so every turn but the last must not be end_turn.
func replyTurn(text, stopReason string) fake.Turn {
	return fake.Turn{
		StopReason: stopReason,
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: text}}},
	}
}

func assertReplays(t *testing.T, db *sql.DB, inner *fake.Client, replies []string, stopReasons []string) {
	t.Helper()
	got := reconstruct(t, db, onlyRunID(t, db))
	want := inner.Requests()
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i], got[i].request, "request %d", i)
		require.NotNil(t, got[i].response, "response %d", i)
		assert.Equal(t, []llm.ContentBlock{{Type: llm.ContentTypeText, Text: replies[i]}}, got[i].response.Content, "response %d", i)
		assert.Equal(t, stopReasons[i], got[i].response.StopReason, "response %d", i)
	}
}

type fdcStub struct{ results []food.Candidate }

func (p fdcStub) SearchByName(context.Context, string, int) ([]food.Candidate, error) {
	return p.results, nil
}

func TestReplay_LookupRun(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		replyTurn(`["carrot raw"]`, "max_tokens"),
		replyTurn(`[1]`, "end_turn"),
	}})
	resolver := food.NewResolver(sqlite.NewFoodRepo(db), nil, fdcStub{results: []food.Candidate{
		{Name: "Carrot juice", Source: food.SourceFDC, FdcID: 1},
		{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 2},
	}}, llm.StaticResolver(rec.Meter(inner, "fake"), "fake-lookup")).WithRecorder(rec)

	_, picked, err := resolver.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)
	require.Equal(t, 1, picked)

	assertReplays(t, db, inner, []string{`["carrot raw"]`, `[1]`}, []string{"max_tokens", "end_turn"})

	// Translate and pick-best use different system prompts: both are stored,
	// each step points at its own, and the run keeps the first.
	reqs := inner.Requests()
	require.Len(t, reqs, 2)
	require.NotEqual(t, reqs[0].System, reqs[1].System)
	var prompts int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_prompts`).Scan(&prompts))
	assert.Equal(t, 2, prompts)
	var runHash string
	require.NoError(t, db.QueryRow(`SELECT prompt_hash FROM ai_runs`).Scan(&runHash))
	assert.Equal(t, sha(reqs[0].System), runHash)
	rows, err := db.Query(`SELECT json_extract(request_json, '$.system_hash') FROM ai_run_steps ORDER BY seq`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var hashes []string
	for rows.Next() {
		var h string
		require.NoError(t, rows.Scan(&h))
		hashes = append(hashes, h)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{sha(reqs[0].System), sha(reqs[1].System)}, hashes)
}

type noRecipe struct{}

func (noRecipe) ExtractRecipe(string) (*importer.RawRecipe, error) { return &importer.RawRecipe{}, nil }

func TestReplay_ImportFallbackRetry(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	validJSON := `{"name":"Soup","servings":2,"instructions":["Cook"],"ingredient_lines":["1 l Water"],"language":"en"}`
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		replyTurn("not json", "max_tokens"),
		replyTurn(validJSON, "end_turn"),
	}})
	svc := importer.NewService(nil, noRecipe{}, llm.StaticResolver(rec.Meter(inner, "fake"), "fake-import"), nil, nil).
		WithRecorder(rec)

	d, err := svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html><body>soup</body></html>"})
	require.NoError(t, err)
	require.Equal(t, "Soup", d.Name)

	reqs := inner.Requests()
	require.Len(t, reqs, 2)
	require.Len(t, reqs[1].Messages, 3, "user prompt, assistant reply, correction")
	assertReplays(t, db, inner, []string{"not json", validJSON}, []string{"max_tokens", "end_turn"})
}
