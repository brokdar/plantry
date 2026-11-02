package handlers_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// B5 (§3.1): the discovery endpoints over SQLite, a recipe site on
// httptest and a scripted research client.

// discoverySite serves a JSON-LD recipe page per name, /broken without one.
func discoverySite(t *testing.T, names ...string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for _, n := range names {
		page := `<html><head><script type="application/ld+json">{"@type":"Recipe","name":"` + n +
			`","totalTime":"PT35M","recipeYield":"2","recipeIngredient":["200 g Reis","1 Zwiebel"]}</script></head><body>ok</body></html>`
		mux.HandleFunc("/"+n, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, page)
		})
	}
	mux.HandleFunc("/broken", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><body>no recipe</body></html>")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// researchClient answers call i with candidates for batches[i]; with gate
// set each call waits for it to close.
type researchClient struct {
	site    *httptest.Server
	batches [][]string
	gate    chan struct{}
	mu      sync.Mutex
	calls   int
}

func (c *researchClient) Stream(ctx context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	if c.gate != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.gate:
		}
	}
	c.mu.Lock()
	var names []string
	if c.calls < len(c.batches) {
		names = c.batches[c.calls]
	}
	c.calls++
	c.mu.Unlock()
	cands := make([]string, len(names))
	for i, n := range names {
		cands[i] = `{"url": "` + c.site.URL + `/` + n + `", "cuisine": "persian", "why": "Try ` + n + `."}`
	}
	return &llm.Response{
		Message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{
			Type: llm.ContentTypeText, Text: `{"candidates": [` + strings.Join(cands, ",") + `]}`,
		}}},
		StopReason: llm.StopReasonEndTurn,
	}, nil
}

func (c *researchClient) Complete(context.Context, llm.Request) (string, error) {
	return "", errors.New("unused")
}

func (c *researchClient) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type staticProfile struct{}

func (staticProfile) Get(context.Context) (*profile.Profile, error) { return &profile.Profile{}, nil }

type discoveryAPI struct {
	t      *testing.T
	db     *sql.DB
	router http.Handler
	svc    *discovery.Service
	foods  *sqlite.FoodRepo
}

// newDiscoveryAPI wires the endpoints; a nil client means no provider.
func newDiscoveryAPI(t *testing.T, client llm.Client) *discoveryAPI {
	t.Helper()
	db := testhelper.NewTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), logger, time.Now)
	repo := sqlite.NewRecipeSuggestionRepo(db)
	foods := sqlite.NewFoodRepo(db)
	images, err := imagestore.New(t.TempDir(), nil)
	require.NoError(t, err)
	resolver := llm.StaticResolver(nil, "")
	if client != nil {
		resolver = llm.StaticResolver(rec.Meter(client, "fake"), "fake-model")
	}
	svc := discovery.NewService(discovery.Deps{
		LLM: resolver, Prefs: testsvc.Settings(db), Foods: foods, Recorder: rec,
		Profile: staticProfile{}, Catalogue: foods,
		Verifier: &discovery.Verifier{
			Extractor: importer.NewService(httpfetch.New(), jsonld.Extractor{}, nil, nil, nil),
			Index:     repo,
			Guard:     func(u string) error { return httpfetch.PublicURL(u, nil, true) },
		},
		Annotator: &discovery.Annotator{Matcher: food.NewResolver(foods, nil, nil, nil), Used: foods, Nutrition: food.NewNutritionResolver(foods)},
		Store:     &discovery.Store{Repo: repo, Images: images},
		Logger:    logger,
	})
	t.Cleanup(func() { _ = svc.Shutdown(context.Background()) })
	return &discoveryAPI{
		t: t, db: db, svc: svc, foods: foods,
		router: transport.NewRouter(logger, http.NotFoundHandler(), transport.Handlers{Discovery: handlers.NewDiscoveryHandler(svc)}),
	}
}

func (a *discoveryAPI) do(method, path, body string) (int, map[string]any) {
	a.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)
	var out map[string]any
	if w.Body.Len() > 0 {
		require.NoError(a.t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	}
	return w.Code, out
}

// suggestion asserts a 200 {suggestion} and returns it.
func (a *discoveryAPI) suggestion(method, path, body string) map[string]any {
	a.t.Helper()
	code, out := a.do(method, path, body)
	require.Equal(a.t, http.StatusOK, code, out)
	sug, ok := out["suggestion"].(map[string]any)
	require.True(a.t, ok, out)
	return sug
}

// started asserts a 202 {run_id}, waits for the run and returns its id.
func (a *discoveryAPI) started(method, path, body string) int64 {
	a.t.Helper()
	code, out := a.do(method, path, body)
	require.Equal(a.t, http.StatusAccepted, code, out)
	id := int64(out["run_id"].(float64))
	require.NotZero(a.t, id)
	a.svc.Wait()
	return id
}

func (a *discoveryAPI) setting(key, raw string) {
	a.t.Helper()
	require.NoError(a.t, testsvc.Settings(a.db).Set(context.Background(), key, raw))
}

func (a *discoveryAPI) outcome(runID int64) string {
	a.t.Helper()
	var o sql.NullString
	require.NoError(a.t, a.db.QueryRow(`SELECT outcome FROM ai_runs WHERE id = ?`, runID).Scan(&o))
	return o.String
}

func jsonID(v any) int64 { return int64(v.(float64)) }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func ptr[T any](v T) *T { return &v }

// B5-AC1: 202 run → 200 the same suggestion all day → next day the next
// pending one without a new run.
func TestDiscoveryAPI_Daily(t *testing.T) {
	site := discoverySite(t, "r1", "r2")
	client := &researchClient{site: site, batches: [][]string{{"r1", "r2"}}}
	a := newDiscoveryAPI(t, client)

	runID := a.started(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	sug := a.suggestion(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	assert.Equal(t, "2026-11-02", sug["shown_on"])
	assert.Equal(t, float64(runID), sug["run_id"])
	again := a.suggestion(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	assert.Equal(t, sug["id"], again["id"])

	next := a.suggestion(http.MethodGet, "/api/discovery/daily?date=2026-11-03", "")
	assert.NotEqual(t, sug["id"], next["id"])
	assert.Equal(t, "2026-11-03", next["shown_on"])
	assert.Equal(t, 1, client.Calls(), "no new run")

	code, out := a.do(http.MethodGet, "/api/discovery/daily", "")
	assert.Equal(t, http.StatusBadRequest, code, out)
	code, out = a.do(http.MethodGet, "/api/discovery/daily?date=02.11.2026", "")
	assert.Equal(t, http.StatusBadRequest, code, out)
}

// The suggestion's shape (§3.1).
func TestDiscoveryAPI_SuggestionShape(t *testing.T) {
	site := discoverySite(t, "r1")
	a := newDiscoveryAPI(t, &researchClient{site: site, batches: [][]string{{"r1"}}})
	a.started(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	sug := a.suggestion(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")

	for _, k := range []string{
		"id", "mode", "query", "source_url", "image_url", "grounded", "status", "reason", "run_id",
		"food_id", "shown_on", "created_at", "name", "cuisine", "total_minutes", "reference_portions", "diet_class",
		"per_portion", "nutrition_complete", "missing", "why", "known_share", "ingredients", "draft",
	} {
		assert.Contains(t, sug, k)
	}
	assert.Equal(t, "daily", sug["mode"])
	assert.Equal(t, site.URL+"/r1", sug["source_url"])
	assert.Nil(t, sug["image_url"])
	assert.Equal(t, false, sug["grounded"])
	assert.Equal(t, "pending", sug["status"])
	assert.Equal(t, "r1", sug["name"])
	assert.Equal(t, "persian", sug["cuisine"])
	assert.Equal(t, float64(35), sug["total_minutes"])
	assert.Equal(t, float64(2), sug["reference_portions"])
	assert.Equal(t, "Try r1.", sug["why"])
	assert.Equal(t, false, sug["nutrition_complete"])
	assert.Contains(t, []any{"vegan", "vegetarian", "pescetarian", "omnivore", "unknown"}, sug["diet_class"])
	ings := sug["ingredients"].([]any)
	require.Len(t, ings, 2)
	first := ings[0].(map[string]any)
	assert.Equal(t, "200 g Reis", first["raw_text"])
	assert.Equal(t, float64(200), first["amount"])
	assert.Equal(t, "g", first["unit"])
	assert.Nil(t, first["food_id"])
	assert.Contains(t, first, "candidate")
	assert.Nil(t, first["candidate"])
	assert.Equal(t, false, first["known"])
	assert.Equal(t, "r1", sug["draft"].(map[string]any)["name"])
	_, err := time.Parse(time.RFC3339, sug["created_at"].(string))
	assert.NoError(t, err)
}

// B5-AC2: daily disabled or no provider → 204 and no run; a failed run →
// 204 for the rest of the day.
func TestDiscoveryAPI_DailySilent(t *testing.T) {
	site := discoverySite(t, "r1")
	client := &researchClient{site: site, batches: [][]string{{"broken"}}}
	a := newDiscoveryAPI(t, client)

	a.setting("discovery.daily_enabled", "false")
	code, _ := a.do(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	assert.Equal(t, http.StatusNoContent, code)
	code, _ = a.do(http.MethodPost, "/api/discovery/daily/another", `{"date":"2026-11-02"}`)
	assert.Equal(t, http.StatusNoContent, code)
	assert.Zero(t, client.Calls())

	a.setting("discovery.daily_enabled", "true")
	runID := a.started(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	code, out := a.do(http.MethodGet, "/api/discovery/runs/"+itoa(runID), "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "partial", out["status"])
	code, _ = a.do(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	assert.Equal(t, http.StatusNoContent, code, "a failed run silences the day")
	assert.Equal(t, 1, client.Calls())

	none := newDiscoveryAPI(t, nil)
	code, _ = none.do(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	assert.Equal(t, http.StatusNoContent, code)
}

// B5-AC3: another → the next pending (the previous stays pending, shown_on
// cleared) → 202 when the pool is empty.
func TestDiscoveryAPI_Another(t *testing.T) {
	site := discoverySite(t, "r1", "r2", "r3")
	client := &researchClient{site: site, batches: [][]string{{"r1", "r2"}, {"r3"}}}
	a := newDiscoveryAPI(t, client)
	a.started(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	first := a.suggestion(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")

	second := a.suggestion(http.MethodPost, "/api/discovery/daily/another", `{"date":"2026-11-02"}`)
	assert.NotEqual(t, first["id"], second["id"])
	assert.Equal(t, "2026-11-02", second["shown_on"])

	code, out := a.do(http.MethodGet, "/api/discovery/suggestions?status=pending", "")
	require.Equal(t, http.StatusOK, code)
	items := out["items"].([]any)
	require.Len(t, items, 2, "both still pending")
	for _, it := range items {
		if it.(map[string]any)["id"] == first["id"] {
			assert.Nil(t, it.(map[string]any)["shown_on"], "shown_on cleared")
		}
	}

	a.started(http.MethodPost, "/api/discovery/daily/another", `{"date":"2026-11-02"}`)
	third := a.suggestion(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	assert.Equal(t, site.URL+"/r3", third["source_url"])

	code, _ = a.do(http.MethodPost, "/api/discovery/daily/another", `{}`)
	assert.Equal(t, http.StatusBadRequest, code)
}

// B5-AC4: accept and reject, decided twice → 409, outcomes.
func TestDiscoveryAPI_AcceptReject(t *testing.T) {
	site := discoverySite(t, "r1", "r2", "r3")
	client := &researchClient{site: site, batches: [][]string{{"r1"}, {"r2", "r3"}}}
	a := newDiscoveryAPI(t, client)
	ctx := context.Background()

	acceptRun := a.started(http.MethodPost, "/api/discovery", `{"mode":"explore","cuisine":"persian"}`)
	code, out := a.do(http.MethodGet, "/api/discovery/runs/"+itoa(acceptRun), "")
	require.Equal(t, http.StatusOK, code)
	sugID := jsonID(out["suggestion_ids"].([]any)[0])
	portions := 2.0
	f := &food.Food{Name: "Saved r1", Kind: food.KindComposed, Role: ptr(food.RoleMain), ReferencePortions: &portions}
	require.NoError(t, a.foods.Create(ctx, f))

	sug := a.suggestion(http.MethodPost, "/api/discovery/suggestions/"+itoa(sugID)+"/accept", `{"food_id":`+itoa(f.ID)+`}`)
	assert.Equal(t, "accepted", sug["status"])
	assert.Equal(t, float64(f.ID), sug["food_id"])
	assert.Equal(t, "accepted", a.outcome(acceptRun))

	code, out = a.do(http.MethodPost, "/api/discovery/suggestions/"+itoa(sugID)+"/accept", `{"food_id":`+itoa(f.ID)+`}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.discovery.decided", out["message_key"])
	code, out = a.do(http.MethodPost, "/api/discovery/suggestions/"+itoa(sugID)+"/reject", `{}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.discovery.decided", out["message_key"])

	rejectRun := a.started(http.MethodPost, "/api/discovery", `{"mode":"known","focus":"chickpeas"}`)
	code, out = a.do(http.MethodGet, "/api/discovery/suggestions?run_id="+itoa(rejectRun), "")
	require.Equal(t, http.StatusOK, code)
	items := out["items"].([]any)
	require.Len(t, items, 2)
	r1 := a.suggestion(http.MethodPost, "/api/discovery/suggestions/"+itoa(jsonID(items[0].(map[string]any)["id"]))+"/reject", `{"reason":"too_long"}`)
	assert.Equal(t, "rejected", r1["status"])
	assert.Equal(t, "too_long", r1["reason"])
	assert.Equal(t, "open", a.outcome(rejectRun))
	a.suggestion(http.MethodPost, "/api/discovery/suggestions/"+itoa(jsonID(items[1].(map[string]any)["id"]))+"/reject", ``)
	assert.Equal(t, "discarded", a.outcome(rejectRun), "all rejected")

	code, out = a.do(http.MethodGet, "/api/discovery/suggestions?status=rejected", "")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, out["items"], 2)

	for _, tc := range []struct {
		path, body string
		status     int
		key        string
	}{
		{"/api/discovery/suggestions/9999/accept", `{"food_id":` + itoa(f.ID) + `}`, http.StatusNotFound, "error.not_found"},
		{"/api/discovery/suggestions/9999/reject", `{}`, http.StatusNotFound, "error.not_found"},
		{"/api/discovery/suggestions/1/accept", `{}`, http.StatusBadRequest, "error.invalid_body"},
		{"/api/discovery/suggestions/1/reject", `{"reason":"bored"}`, http.StatusBadRequest, "error.invalid_body"},
		{"/api/discovery/suggestions/x/reject", `{}`, http.StatusBadRequest, "error.invalid_id"},
	} {
		code, out := a.do(http.MethodPost, tc.path, tc.body)
		assert.Equalf(t, tc.status, code, "%s %s", tc.path, tc.body)
		assert.Equalf(t, tc.key, out["message_key"], "%s %s", tc.path, tc.body)
	}
	code, _ = a.do(http.MethodGet, "/api/discovery/suggestions?status=maybe", "")
	assert.Equal(t, http.StatusBadRequest, code)
}

// B5-AC5: POST /api/discovery validates mode and cuisine; no provider →
// 503; running → 409.
func TestDiscoveryAPI_Start(t *testing.T) {
	site := discoverySite(t, "r1")
	client := &researchClient{site: site, batches: [][]string{{"r1"}}, gate: make(chan struct{})}
	a := newDiscoveryAPI(t, client)

	for _, tc := range []struct{ body, key string }{
		{`{"mode":"weekly"}`, "error.discovery.invalid_mode"},
		{`{}`, "error.discovery.invalid_mode"},
		{`{"mode":"explore"}`, "error.discovery.cuisine_required"},
		{`{"mode":"explore","cuisine":"  "}`, "error.discovery.cuisine_required"},
	} {
		code, out := a.do(http.MethodPost, "/api/discovery", tc.body)
		assert.Equal(t, http.StatusBadRequest, code, tc.body)
		assert.Equal(t, tc.key, out["message_key"], tc.body)
	}
	code, _ := a.do(http.MethodPost, "/api/discovery", `{"mode":"daily","today":"soon"}`)
	assert.Equal(t, http.StatusBadRequest, code)

	a.setting("discovery.cuisines", `["persian","thai"]`)
	code, out := a.do(http.MethodPost, "/api/discovery", `{"mode":"daily","today":"2026-11-02"}`)
	require.Equal(t, http.StatusAccepted, code, out)
	runID := jsonID(out["run_id"])

	code, out = a.do(http.MethodPost, "/api/discovery", `{"mode":"explore","cuisine":"thai"}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.discovery.running", out["message_key"])
	code, out = a.do(http.MethodGet, "/api/discovery/runs/"+itoa(runID), "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "running", out["status"])
	code, out = a.do(http.MethodGet, "/api/discovery/daily?date=2026-11-02", "")
	assert.Equal(t, http.StatusAccepted, code, "the running run")
	assert.Equal(t, float64(runID), out["run_id"])

	close(client.gate)
	a.svc.Wait()
	var scope string
	require.NoError(t, a.db.QueryRow(`SELECT scope_json FROM ai_runs WHERE id = ?`, runID).Scan(&scope))
	assert.JSONEq(t, `{"mode":"daily","cuisine":"`+discovery.DailyCuisine([]string{"persian", "thai"}, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))+`"}`, scope)

	code, out = a.do(http.MethodGet, "/api/discovery/runs/"+itoa(runID), "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", out["status"])
	assert.Len(t, out["suggestion_ids"], 1)
	assert.Equal(t, float64(0), out["discarded"])
	assert.Nil(t, out["error_key"])
	code, _ = a.do(http.MethodGet, "/api/discovery/runs/999", "")
	assert.Equal(t, http.StatusNotFound, code)

	none := newDiscoveryAPI(t, nil)
	code, out = none.do(http.MethodPost, "/api/discovery", `{"mode":"explore","cuisine":"thai"}`)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "error.ai.provider_missing", out["message_key"])
}
