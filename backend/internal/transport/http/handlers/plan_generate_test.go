package handlers_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// Phase 3 B6 (§3.2): generation endpoints over SQLite with the heuristic.
// Saturday shopping; the window under test is Sat 2026-11-07 … Fri 11-13,
// today Fri 11-06.

const genToday = "?today=2026-11-06"

type genAPI struct {
	t      *testing.T
	db     *sql.DB
	router http.Handler
	svc    *planning.Service
	// queued holds the generations the Go hook has not run yet.
	queued []func()
	dinner int64
}

func newGenAPI(t *testing.T) *genAPI {
	t.Helper()
	db := testhelper.NewTestDB(t)
	a := &genAPI{t: t, db: db}
	a.exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'sun', 1, 1)`)
	a.dinner = a.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`)
	for i := range 10 {
		a.exec(fmt.Sprintf(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Main %d', 'composed', 'main', 4)`, i))
	}
	a.exec(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Rice', 'composed', 'side_starch', 4)`)
	a.exec(`UPDATE user_profile SET household_portions = 1 WHERE id = 1`)
	set := testsvc.Settings(db)
	require.NoError(t, set.Set(context.Background(), "plan.shopping_day", "5"))
	require.NoError(t, set.Set(context.Background(), "plan.batch_cooking", "true"))

	deps := testsvc.PlanningDeps(db)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	deps.RunLog, deps.Runs = planning.RecorderLog{Recorder: rec}, sqlite.NewAIRunRepo(db)
	deps.Go = func(f func()) { a.queued = append(a.queued, f) }
	a.svc = planning.NewService(deps)
	a.router = transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Plates:       handlers.NewPlateHandler(testsvc.Plates(db)),
		PlanGenerate: handlers.NewPlanGenerateHandler(a.svc),
	})
	return a
}

func (a *genAPI) exec(q string) {
	a.t.Helper()
	_, err := a.db.Exec(q)
	require.NoError(a.t, err, q)
}

func (a *genAPI) insert(q string) int64 {
	a.t.Helper()
	res, err := a.db.Exec(q)
	require.NoError(a.t, err, q)
	id, err := res.LastInsertId()
	require.NoError(a.t, err)
	return id
}

// serve runs one request with ctx and decodes the JSON reply.
func (a *genAPI) serve(ctx context.Context, method, path, body string) (int, map[string]any) {
	a.t.Helper()
	req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)
	out := map[string]any{}
	if w.Body.Len() > 0 {
		require.NoError(a.t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	}
	return w.Code, out
}

func (a *genAPI) do(method, path, body string) (int, map[string]any) {
	return a.serve(context.Background(), method, path, body)
}

// drain runs the queued generations.
func (a *genAPI) drain() {
	for len(a.queued) > 0 {
		f := a.queued[0]
		a.queued = a.queued[1:]
		f()
	}
	a.svc.Wait()
}

// B6-AC4: 202 at once, 409 while the window's run runs, and the run
// survives the client going away.
func TestPlanGenerateHTTP_RunOutlivesTheRequest(t *testing.T) {
	a := newGenAPI(t)
	ctx, cancel := context.WithCancel(context.Background())

	code, body := a.serve(ctx, http.MethodPost, "/api/plan/generate"+genToday, `{"start":"2026-11-07"}`)
	require.Equal(t, http.StatusAccepted, code, body)
	id := int64(body["run_id"].(float64))
	cancel() // the client disconnects before the run does anything
	require.Len(t, a.queued, 1, "the run has not started yet")

	code, body = a.do(http.MethodPost, "/api/plan/generate"+genToday, `{"start":"2026-11-07"}`)
	require.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.plan.generation_running", body["message_key"])
	code, run := a.do(http.MethodGet, fmt.Sprintf("/api/plan/runs/%d", id), "")
	require.Equal(t, http.StatusOK, code, run)
	assert.Equal(t, "running", run["status"])
	assert.Equal(t, "resolving", run["stage"])

	a.drain()

	code, run = a.do(http.MethodGet, fmt.Sprintf("/api/plan/runs/%d", id), "")
	require.Equal(t, http.StatusOK, code, run)
	assert.Equal(t, "ok", run["status"], run)
	assert.Nil(t, run["stage"])
	assert.Equal(t, "generate", run["kind"])
	assert.Equal(t, map[string]any{"start": "2026-11-07", "end": "2026-11-13"}, run["window"])
	assert.Equal(t, []any{}, run["issues"])
	assert.NotNil(t, run["score"])
	counts := run["counts"].(map[string]any)
	assert.InDelta(t, 14, counts["plates"], 1e-9, "every lunch and dinner")
	assert.Positive(t, counts["sessions"])
	assert.Positive(t, counts["cook_days"])
	assert.Contains(t, counts, "kept_confirmed")
	assert.Equal(t, "heuristic", run["chooser"])
	var plates int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM plates WHERE ai_run_id = ? AND state = 'draft'`, id).Scan(&plates))
	assert.Equal(t, 14, plates)

	// Every run is an ai_runs row, the heuristic's too (D9).
	var kind, status, chooser, provider, model, version, violations, scope string
	require.NoError(t, a.db.QueryRow(`SELECT kind, status, chooser, provider, model, prompt_version, violations_json, scope_json FROM ai_runs WHERE id = ?`, id).
		Scan(&kind, &status, &chooser, &provider, &model, &version, &violations, &scope))
	assert.Equal(t, []string{"generate", "ok", "heuristic", "", "", planning.HeuristicVersion, "[]"}, []string{kind, status, chooser, provider, model, version, violations})
	assert.JSONEq(t, `{"kind":"window","plate_ids":[],"session_ids":[],"kept_confirmed":0}`, scope)

	code, body = a.do(http.MethodPost, "/api/plan/generate"+genToday, `{"start":"2026-11-07"}`)
	assert.Equal(t, http.StatusAccepted, code, "the window is free again: %v", body)
}

func TestPlanGenerateHTTP_Regenerate(t *testing.T) {
	a := newGenAPI(t)
	a.do(http.MethodPost, "/api/plan/generate"+genToday, `{"start":"2026-11-07"}`)
	a.drain()
	var pid int64
	require.NoError(t, a.db.QueryRow(`SELECT id FROM plates WHERE date = '2026-11-08' AND slot_id = ?`, a.dinner).Scan(&pid))

	code, body := a.do(http.MethodPost, "/api/plan/regenerate"+genToday, fmt.Sprintf(`{"start":"2026-11-07","scope":{"kind":"plate","plate_id":%d}}`, pid))
	require.Equal(t, http.StatusAccepted, code, body)
	a.drain()
	_, run := a.do(http.MethodGet, fmt.Sprintf("/api/plan/runs/%.0f", body["run_id"]), "")
	assert.Equal(t, "regenerate", run["kind"])
	assert.Equal(t, "ok", run["status"], run)

	code, body = a.do(http.MethodPost, "/api/plan/regenerate"+genToday, `{"start":"2026-11-07","scope":{"kind":"day","date":"2026-11-10"}}`)
	require.Equal(t, http.StatusAccepted, code, body)
	a.drain()

	require.NoError(t, a.db.QueryRow(`SELECT id FROM plates WHERE date = '2026-11-08' AND slot_id = ?`, a.dinner).Scan(&pid))
	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/confirm", pid), "")
	require.Equal(t, http.StatusOK, code, body)
	code, body = a.do(http.MethodPost, "/api/plan/regenerate"+genToday, fmt.Sprintf(`{"start":"2026-11-07","scope":{"kind":"plate","plate_id":%d}}`, pid))
	require.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.plan.scope_confirmed", body["message_key"])
}

func TestPlanGenerateHTTP_Errors(t *testing.T) {
	a := newGenAPI(t)
	tests := []struct {
		name, method, path, body string
		status                   int
		key                      string
	}{
		{"malformed body", http.MethodPost, "/api/plan/generate", `{`, http.StatusBadRequest, "error.invalid_body"},
		{"start missing", http.MethodPost, "/api/plan/generate", `{}`, http.StatusBadRequest, "error.plan.invalid_start"},
		{"start not a shopping day", http.MethodPost, "/api/plan/generate", `{"start":"2026-11-08"}`, http.StatusBadRequest, "error.plan.invalid_start"},
		{"bad today", http.MethodPost, "/api/plan/generate?today=x", `{"start":"2026-11-07"}`, http.StatusBadRequest, "error.invalid_body"},
		{"guests outside the window", http.MethodPost, "/api/plan/generate", `{"start":"2026-11-07","eaters_by_date":{"2026-12-01":2}}`, http.StatusBadRequest, "error.invalid_body"},
		{"window scope", http.MethodPost, "/api/plan/regenerate", `{"start":"2026-11-07","scope":{"kind":"window"}}`, http.StatusBadRequest, "error.plan.invalid_scope"},
		{"bad scope date", http.MethodPost, "/api/plan/regenerate", `{"start":"2026-11-07","scope":{"kind":"day","date":"tue"}}`, http.StatusBadRequest, "error.plan.invalid_scope"},
		{"unknown plate", http.MethodPost, "/api/plan/regenerate" + genToday, `{"start":"2026-11-07","scope":{"kind":"plate","plate_id":999}}`, http.StatusNotFound, "error.not_found"},
		{"no llm chooser", http.MethodPost, "/api/plan/generate", `{"start":"2026-11-07","chooser":"llm"}`, http.StatusServiceUnavailable, "error.ai.provider_missing"},
		{"run id not a number", http.MethodGet, "/api/plan/runs/abc", "", http.StatusBadRequest, "error.invalid_id"},
		{"unknown run", http.MethodGet, "/api/plan/runs/999", "", http.StatusNotFound, "error.not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := a.do(tt.method, tt.path, tt.body)
			assert.Equal(t, tt.status, code, body)
			assert.Equal(t, tt.key, body["message_key"])
		})
	}
	assert.Empty(t, a.queued, "no run started")
}

// B6-AC7: below the onboarding target of 10 recipes.
func TestPlanGenerateHTTP_NotEnoughRecipes(t *testing.T) {
	a := newGenAPI(t)
	a.exec(`DELETE FROM foods WHERE name IN ('Main 0', 'Rice')`)
	code, body := a.do(http.MethodPost, "/api/plan/generate"+genToday, `{"start":"2026-11-07"}`)
	require.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "error.plan.not_enough_recipes", body["message_key"])
}
