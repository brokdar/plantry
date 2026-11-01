package handlers_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type runsAPI struct {
	t         *testing.T
	db        *sql.DB
	repo      *sqlite.AIRunRepo
	router    chi.Router
	overrides map[string]airun.Price
}

func newRunsAPI(t *testing.T) *runsAPI {
	t.Helper()
	db := testhelper.NewTestDB(t)
	a := &runsAPI{t: t, db: db, repo: sqlite.NewAIRunRepo(db)}
	svc := airun.NewInsights(a.repo, func(context.Context) (map[string]airun.Price, error) { return a.overrides, nil })
	h := handlers.NewAIRunsHandler(svc, func(context.Context) (string, error) { return "claude-sonnet-5-5", nil })
	r := chi.NewRouter()
	r.Get("/api/ai/runs", h.List)
	r.Get("/api/ai/runs/{id}", h.Get)
	r.Get("/api/ai/insights", h.Insights)
	r.Get("/api/ai/prices", h.Prices)
	a.router = r
	return a
}

// run stores a finished run started at day 12:00 UTC.
func (a *runsAPI) run(kind airun.Kind, day string, model, provider string, input int64) int64 {
	a.t.Helper()
	started, err := time.Parse(time.DateOnly, day)
	require.NoError(a.t, err)
	started = started.Add(12 * time.Hour)
	ctx := context.Background()
	id, err := a.repo.CreateRun(ctx, airun.RunStart{Kind: kind, StartedAt: started})
	require.NoError(a.t, err)
	require.NoError(a.t, a.repo.FinishRun(ctx, airun.RunEnd{
		ID: id, Status: airun.StatusOK, FinishedAt: started.Add(time.Second), Model: model, Provider: provider, InputTokens: input,
	}))
	return id
}

func (a *runsAPI) get(path string, out any) *httptest.ResponseRecorder {
	a.t.Helper()
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if out != nil && rec.Code == http.StatusOK {
		require.NoError(a.t, json.Unmarshal(rec.Body.Bytes(), out))
	}
	return rec
}

type runsPage struct {
	Items []struct {
		ID              int64    `json:"id"`
		Kind            string   `json:"kind"`
		StartedAt       string   `json:"started_at"`
		Model           string   `json:"model"`
		CostEstimateUSD *float64 `json:"cost_estimate_usd"`
		Chooser         *string  `json:"chooser"`
		Score           any      `json:"score"`
	} `json:"items"`
	NextCursor *int64 `json:"next_cursor"`
}

func (p runsPage) ids() []int64 {
	out := make([]int64, len(p.Items))
	for i, it := range p.Items {
		out[i] = it.ID
	}
	return out
}

// B4-AC2
func TestAIRuns_ListPagesFiltersAndPrices(t *testing.T) {
	a := newRunsAPI(t)
	c1 := a.run(airun.KindChat, "2026-10-01", "claude-sonnet-4-6", "anthropic", 1_000_000)
	g2 := a.run(airun.KindGenerate, "2026-10-02", "", "", 0)
	c3 := a.run(airun.KindChat, "2026-10-03", "fake", "fake", 500)
	g4 := a.run(airun.KindGenerate, "2026-10-04", "claude-sonnet-4-6", "anthropic", 0)

	var page runsPage
	first := a.get("/api/ai/runs?limit=3", &page)
	require.Equal(t, http.StatusOK, first.Code)
	assert.Contains(t, first.Body.String(), `"provider":null,"model":null,"prompt_version":null`, "empty strings are null")
	assert.Equal(t, []int64{g4, c3, g2}, page.ids())
	require.NotNil(t, page.NextCursor)
	assert.Equal(t, g2, *page.NextCursor)
	assert.Nil(t, page.Items[1].CostEstimateUSD, "fake model is unpriced")
	assert.Equal(t, "2026-10-03T12:00:00Z", page.Items[1].StartedAt)

	var next runsPage
	a.get(fmt.Sprintf("/api/ai/runs?limit=3&cursor=%d", *page.NextCursor), &next)
	assert.Equal(t, []int64{c1}, next.ids())
	assert.Nil(t, next.NextCursor)
	require.NotNil(t, next.Items[0].CostEstimateUSD)
	assert.InDelta(t, 3.0, *next.Items[0].CostEstimateUSD, 1e-9)

	rec := a.get("/api/ai/runs?kind=chat", &page)
	assert.Equal(t, []int64{c3, c1}, page.ids())
	assert.Contains(t, rec.Body.String(), `"cost_estimate_usd":null`)

	a.get("/api/ai/runs?from=2026-10-02&to=2026-10-03", &page)
	assert.Equal(t, []int64{c3, g2}, page.ids(), "to is inclusive")

	a.get("/api/ai/runs?kind=generate&from=2026-10-03", &page)
	assert.Equal(t, []int64{g4}, page.ids())

	// An override prices the fake model on read.
	a.overrides = map[string]airun.Price{"fake": {In: 2, Out: 2}}
	a.get("/api/ai/runs?kind=chat", &page)
	require.NotNil(t, page.Items[0].CostEstimateUSD)
	assert.InDelta(t, 0.001, *page.Items[0].CostEstimateUSD, 1e-9)
}

func TestAIRuns_ListEmptyIsArray(t *testing.T) {
	a := newRunsAPI(t)
	rec := a.get("/api/ai/runs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"items":[],"next_cursor":null}`, rec.Body.String())
}

func TestAIRuns_InvalidQueries(t *testing.T) {
	a := newRunsAPI(t)
	for path, key := range map[string]string{
		"/api/ai/runs?kind=dance":                        "error.invalid_query",
		"/api/ai/runs?cursor=abc":                        "error.invalid_query",
		"/api/ai/runs?cursor=0":                          "error.invalid_query",
		"/api/ai/runs?limit=0":                           "error.invalid_query",
		"/api/ai/runs?limit=201":                         "error.invalid_query",
		"/api/ai/runs?from=10-01":                        "error.invalid_from",
		"/api/ai/runs?to=tomorrow":                       "error.invalid_to",
		"/api/ai/runs?from=2026-10-05&to=2026-10-01":     "error.invalid_date_range",
		"/api/ai/runs/abc":                               "error.invalid_id",
		"/api/ai/runs/999":                               "error.not_found",
		"/api/ai/insights?bucket=month":                  "error.invalid_query",
		"/api/ai/insights?from=x":                        "error.invalid_from",
		"/api/ai/insights?from=2026-10-05&to=2026-10-01": "error.invalid_date_range",
		"/api/ai/insights?from=2024-01-01&to=2026-10-01": "error.date_range_too_large",
	} {
		rec := a.get(path, nil)
		want := http.StatusBadRequest
		if key == "error.not_found" {
			want = http.StatusNotFound
		}
		assert.Equal(t, want, rec.Code, path)
		assert.Contains(t, rec.Body.String(), key, path)
	}
}

// B4-AC6
func TestAIRuns_DetailStepsAndPrompt(t *testing.T) {
	a := newRunsAPI(t)
	ctx := context.Background()
	id, err := a.repo.CreateRun(ctx, airun.RunStart{
		Kind: airun.KindGenerate, StartedAt: time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC),
		Chooser: airun.ChooserLLM, ScopeJSON: `{"kind":"day","date":"2026-11-02"}`,
	})
	require.NoError(t, err)
	require.NoError(t, a.repo.UpsertPrompt(ctx, "h", "You are Plantry's planner."))
	require.NoError(t, a.repo.FinishRun(ctx, airun.RunEnd{
		ID: id, Status: airun.StatusOK, FinishedAt: time.Date(2026, 10, 30, 8, 0, 5, 0, time.UTC),
		Provider: "anthropic", Model: "claude-sonnet-5-5", PromptHash: "h", InputTokens: 100, OutputTokens: 50,
		RepairAttempts: 1, ViolationsJSON: `[{"rule":"H8","code":"variety_relaxed"}]`, ScoreJSON: `{"coverage":1}`,
		Notes: "Chili twice.",
	}))
	huge := `"` + strings.Repeat("a", 10000) + `"`
	for _, s := range []airun.Step{
		{RunID: id, Seq: 3, Type: airun.StepToolCall, Name: "submit_plan", ArgsJSON: `{"plates":[]}`, ResultJSON: huge, IsError: true},
		{RunID: id, Seq: 1, Type: airun.StepLLMCall, Name: "claude-sonnet-5-5", RequestJSON: `{"messages":[]}`, ResponseJSON: `{"content":[]}`, InputTokens: 60, OutputTokens: 30, DurationMs: 900},
		{RunID: id, Seq: 2, Type: airun.StepLLMCall, Name: "claude-sonnet-5-5", RequestJSON: `{"messages":[1]}`, InputTokens: 40, OutputTokens: 20, IsError: true},
	} {
		require.NoError(t, a.repo.AddStep(ctx, s))
	}

	var d struct {
		ID         int64   `json:"id"`
		Chooser    string  `json:"chooser"`
		Scope      any     `json:"scope"`
		Notes      string  `json:"notes"`
		PromptText string  `json:"prompt_text"`
		Violations []any   `json:"violations"`
		Score      any     `json:"score"`
		FinishedAt string  `json:"finished_at"`
		Cost       float64 `json:"cost_estimate_usd"`
		Steps      []struct {
			Seq             int     `json:"seq"`
			Type            string  `json:"type"`
			Args            *string `json:"args"`
			Result          *string `json:"result"`
			ResultTruncated bool    `json:"result_truncated"`
			IsError         bool    `json:"is_error"`
			DurationMs      int64   `json:"duration_ms"`
			Tokens          struct {
				Input  int64 `json:"input"`
				Output int64 `json:"output"`
			} `json:"tokens"`
		} `json:"steps"`
	}
	rec := a.get(fmt.Sprintf("/api/ai/runs/%d", id), &d)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "You are Plantry's planner.", d.PromptText)
	assert.Equal(t, "llm", d.Chooser)
	assert.Equal(t, map[string]any{"kind": "day", "date": "2026-11-02"}, d.Scope)
	assert.Equal(t, "Chili twice.", d.Notes)
	assert.Len(t, d.Violations, 1)
	assert.Equal(t, map[string]any{"coverage": float64(1)}, d.Score)
	assert.Equal(t, "2026-10-30T08:00:05Z", d.FinishedAt)
	assert.Positive(t, d.Cost)

	require.Len(t, d.Steps, 3)
	for i, s := range d.Steps {
		assert.Equal(t, i+1, s.Seq, "steps in seq order")
	}
	assert.Equal(t, "llm_call", d.Steps[0].Type)
	assert.Equal(t, int64(60), d.Steps[0].Tokens.Input)
	assert.Equal(t, int64(900), d.Steps[0].DurationMs)
	assert.Nil(t, d.Steps[1].Result, "a failed call has no response")
	require.NotNil(t, d.Steps[2].Result)
	assert.True(t, d.Steps[2].ResultTruncated)
	assert.LessOrEqual(t, len([]rune(*d.Steps[2].Result)), 8192)
	assert.Equal(t, `{"plates":[]}`, *d.Steps[2].Args)
	assert.True(t, d.Steps[2].IsError)
}

func TestAIRuns_DetailOfChatRunHasNulls(t *testing.T) {
	a := newRunsAPI(t)
	id := a.run(airun.KindChat, "2026-10-01", "fake", "fake", 0)
	rec := a.get(fmt.Sprintf("/api/ai/runs/%d", id), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var d map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &d))
	for _, k := range []string{"chooser", "scope", "notes", "violations", "score", "error_key", "prompt_text", "outcome", "correction_rate", "instruction"} {
		v, ok := d[k]
		assert.True(t, ok, k)
		assert.Nil(t, v, k)
	}
	assert.Equal(t, []any{}, d["steps"])
	assert.Equal(t, float64(0), d["cost_estimate_usd"], "no tokens cost nothing")
}

// B4-AC2: week buckets report unpriced runs.
func TestAIRuns_InsightsWeeks(t *testing.T) {
	a := newRunsAPI(t)
	a.run(airun.KindChat, "2026-10-05", "claude-sonnet-4-6", "anthropic", 1_000_000)
	a.run(airun.KindChat, "2026-10-07", "fake", "fake", 10)
	a.run(airun.KindChat, "2026-10-13", "fake", "fake", 10)

	var rep struct {
		Weeks []struct {
			Start        string   `json:"start"`
			Runs         int      `json:"runs"`
			CostUSD      float64  `json:"cost_usd"`
			UnpricedRuns int      `json:"unpriced_runs"`
			CorrRate     *float64 `json:"correction_rate"`
		} `json:"weeks"`
		ByVersion []struct {
			Kind string `json:"kind"`
			Runs int    `json:"runs"`
		} `json:"by_version"`
		PricesAsOf string `json:"prices_as_of"`
	}
	rec := a.get("/api/ai/insights?from=2026-10-05&to=2026-10-18&bucket=week", &rep)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, rep.Weeks, 2)
	assert.Equal(t, "2026-10-05", rep.Weeks[0].Start)
	assert.Equal(t, 2, rep.Weeks[0].Runs)
	assert.Equal(t, 1, rep.Weeks[0].UnpricedRuns)
	assert.InDelta(t, 3.0, rep.Weeks[0].CostUSD, 1e-9)
	assert.Nil(t, rep.Weeks[0].CorrRate)
	assert.Equal(t, "2026-10-12", rep.Weeks[1].Start)
	assert.Equal(t, 1, rep.Weeks[1].UnpricedRuns)
	assert.Equal(t, airun.PricesAsOf, rep.PricesAsOf)
	require.Len(t, rep.ByVersion, 2, "chat runs split by model")

	// Without a range: the last 12 weeks.
	var def struct {
		Weeks []any `json:"weeks"`
	}
	require.Equal(t, http.StatusOK, a.get("/api/ai/insights", &def).Code)
	assert.GreaterOrEqual(t, len(def.Weeks), 12)
}

// B6-AC4: the by-version chat group carries runs, edits and tokens; B6-AC2: a
// run's edits are null without events.
func TestAIRuns_InsightsChatGroupEdits(t *testing.T) {
	a := newRunsAPI(t)
	edited := a.run(airun.KindChat, "2026-10-06", "fake", "fake", 7)
	quiet := a.run(airun.KindChat, "2026-10-07", "fake", "fake", 5)
	// One session edit (its linked plate's event carries cook_session_id) and one plate.
	for _, ev := range []struct{ plate, session any }{{nil, 3}, {10, 3}, {11, nil}} {
		_, err := a.db.Exec(`INSERT INTO plate_events (plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id)
			VALUES (?, ?, '2026-10-10', 1, 'update', 'agent', ?)`, ev.plate, ev.session, edited)
		require.NoError(t, err)
	}

	var rep struct {
		ByVersion []struct {
			Kind   string `json:"kind"`
			Runs   int    `json:"runs"`
			Edits  *int   `json:"edits"`
			Tokens struct {
				Input int64 `json:"input"`
			} `json:"tokens"`
		} `json:"by_version"`
	}
	rec := a.get("/api/ai/insights?from=2026-10-05&to=2026-10-11", &rep)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, rep.ByVersion, 1)
	g := rep.ByVersion[0]
	assert.Equal(t, "chat", g.Kind)
	assert.Equal(t, 2, g.Runs)
	require.NotNil(t, g.Edits)
	assert.Equal(t, 2, *g.Edits)
	assert.Equal(t, int64(12), g.Tokens.Input)

	var page struct {
		Items []struct {
			ID    int64 `json:"id"`
			Edits *int  `json:"edits"`
		} `json:"items"`
	}
	require.Equal(t, http.StatusOK, a.get("/api/ai/runs", &page).Code)
	require.Len(t, page.Items, 2)
	assert.Equal(t, quiet, page.Items[0].ID)
	assert.Nil(t, page.Items[0].Edits)
	require.NotNil(t, page.Items[1].Edits)
	assert.Equal(t, 2, *page.Items[1].Edits)
}

// score_avg leaves out per-date score entries (cook_occasions.per_day).
func TestAIRuns_InsightsScoreAvgSkipsDates(t *testing.T) {
	a := newRunsAPI(t)
	ctx := context.Background()
	started := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	id, err := a.repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindGenerate, StartedAt: started})
	require.NoError(t, err)
	require.NoError(t, a.repo.FinishRun(ctx, airun.RunEnd{
		ID: id, Status: airun.StatusOK, FinishedAt: started.Add(time.Second),
		ScoreJSON: `{"coverage":1,"cook_occasions":{"per_day":{"2026-10-10":1,"2026-10-11":0},"days_with_prepared":2}}`,
	}))

	var rep struct {
		ByVersion []struct {
			ScoreAvg map[string]float64 `json:"score_avg"`
		} `json:"by_version"`
	}
	rec := a.get("/api/ai/insights?from=2026-10-05&to=2026-10-11", &rep)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, rep.ByVersion, 1)
	assert.Equal(t, map[string]float64{"coverage": 1, "cook_occasions.days_with_prepared": 2}, rep.ByVersion[0].ScoreAvg)
}

func TestAIRuns_Prices(t *testing.T) {
	a := newRunsAPI(t)
	var p struct {
		Model  string       `json:"model"`
		Price  *airun.Price `json:"price"`
		Source string       `json:"source"`
		AsOf   string       `json:"as_of"`
	}
	a.get("/api/ai/prices", &p)
	assert.Equal(t, "claude-sonnet-5-5", p.Model, "defaults to the configured model")
	assert.Equal(t, "builtin", p.Source)
	require.NotNil(t, p.Price)
	assert.Equal(t, airun.PricesAsOf, p.AsOf)

	a.overrides = map[string]airun.Price{"local": {In: 1, Out: 2}}
	a.get("/api/ai/prices?model=local", &p)
	assert.Equal(t, "override", p.Source)

	rec := a.get("/api/ai/prices?model=llama", &p)
	assert.Equal(t, "none", p.Source)
	assert.Contains(t, rec.Body.String(), `"price":null`)
}
