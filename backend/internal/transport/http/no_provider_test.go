package http_test

import (
	"context"
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
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type noProfile struct{}

func (noProfile) Get(context.Context) (*profile.Profile, error) { return &profile.Profile{}, nil }

// Z5-AC8 (spec §17.8): a router whose provider resolver finds nothing still
// plans, accepts, shops and shows Today and Setup; only the assistant and
// discovery answer that they need a provider.
func TestNoProvider_EverythingButChatAndDiscoveryWorks(t *testing.T) {
	db := testhelper.NewTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := func(q string) {
		t.Helper()
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
	exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'sun', 1, 1)`)
	exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`)
	for i := range 10 {
		exec(fmt.Sprintf(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Main %d', 'composed', 'main', 4)`, i))
	}
	exec(`UPDATE user_profile SET household_portions = 1 WHERE id = 1`)
	set := testsvc.Settings(db)
	require.NoError(t, set.Set(context.Background(), "plan.shopping_day", "5"))
	require.NoError(t, set.Set(context.Background(), "plan.batch_cooking", "true"))

	// The wiring of cmd/plantry without a provider: the LLM chooser is
	// registered over a resolver that finds none.
	none := llm.StaticResolver(nil, "")
	deps := testsvc.PlanningDeps(db)
	deps.Choosers[airun.ChooserLLM] = planning.NewLLMChooser(none)
	deps.RunLog = planning.RecorderLog{Recorder: airun.NewRecorder(sqlite.NewAIRunRepo(db), logger, time.Now)}
	deps.Runs = sqlite.NewAIRunRepo(db)
	planSvc := planning.NewService(deps)
	t.Cleanup(planSvc.Wait)

	foods := sqlite.NewFoodRepo(db)
	discoverySvc := discovery.NewService(discovery.Deps{
		LLM: none, Prefs: set, Foods: foods, Recorder: airun.NewRecorder(sqlite.NewAIRunRepo(db), logger, time.Now),
		Profile: noProfile{}, Catalogue: foods, Logger: logger,
	})
	t.Cleanup(func() { _ = discoverySvc.Shutdown(context.Background()) })

	router := transport.NewRouter(logger, http.NotFoundHandler(), transport.Handlers{
		Plates:       handlers.NewPlateHandler(testsvc.Plates(db)),
		PlanWrite:    handlers.NewPlanWriteHandler(testsvc.Plates(db)),
		PlanRead:     handlers.NewPlanReadHandler(testsvc.PlanView(db)),
		PlanGenerate: handlers.NewPlanGenerateHandler(planSvc),
		Today:        handlers.NewTodayHandler(testsvc.PlanView(db)),
		Shopping: handlers.NewShoppingHandler(shopping.NewService(shopping.Deps{
			Source: sqlite.NewShoppingRepo(db), Foods: foods, Settings: set, Slots: sqlite.NewSlotRepo(db),
		})),
		Setup:     handlers.NewSetupHandler(setup.NewService(set, foods)),
		AI:        handlers.NewAIHandler(nil, nil, none),
		Discovery: handlers.NewDiscoveryHandler(discoverySvc),
	})
	do := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		out := map[string]any{}
		if w.Body.Len() > 0 {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
		}
		return w.Code, out
	}

	// Setup works.
	code, body := do(http.MethodGet, "/api/setup", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.EqualValues(t, 10, body["recipe_count"])

	// Generate defaults to the heuristic, then accept.
	code, body = do(http.MethodPost, "/api/plan/generate?today=2026-11-06", `{"start":"2026-11-07"}`)
	require.Equal(t, http.StatusAccepted, code, body)
	runID := int64(body["run_id"].(float64))
	planSvc.Wait()
	code, run := do(http.MethodGet, fmt.Sprintf("/api/plan/runs/%d", runID), "")
	require.Equal(t, http.StatusOK, code, run)
	assert.Equal(t, "ok", run["status"], run)
	assert.Equal(t, "heuristic", run["chooser"])

	code, body = do(http.MethodPost, "/api/plan/accept", `{"from":"2026-11-07","to":"2026-11-13"}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.NotEmpty(t, body["plate_ids"])

	// Shopping and Today answer.
	code, body = do(http.MethodGet, "/api/shopping-list?from=2026-11-07&to=2026-11-13", "")
	require.Equal(t, http.StatusOK, code, body)
	code, body = do(http.MethodGet, "/api/today?date=2026-11-08", "")
	require.Equal(t, http.StatusOK, code, body)

	// Chat and discovery say a provider is missing.
	code, body = do(http.MethodPost, "/api/ai/chat", `{"message":"hi","today":"2026-11-06","start":"2026-11-07"}`)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "error.ai.provider_missing", body["message_key"])
	code, body = do(http.MethodPost, "/api/discovery", `{"mode":"explore","cuisine":"persian"}`)
	assert.Equal(t, http.StatusServiceUnavailable, code, body)
	assert.Equal(t, "error.ai.provider_missing", body["message_key"])
	code, _ = do(http.MethodGet, "/api/discovery/daily?date=2026-11-08", "")
	assert.Equal(t, http.StatusNoContent, code, "no daily sheet without a provider")
}
