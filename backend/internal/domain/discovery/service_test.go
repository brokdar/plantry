package discovery_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func jsonldPage(name string) string {
	return `<html><head><script type="application/ld+json">{"@type":"Recipe","name":"` + name +
		`","recipeYield":"2","recipeIngredient":["200 g Reis","1 Zwiebel"]}</script></head><body>ok</body></html>`
}

// recipeSite serves recipe pages under /<name> and a page without a recipe
// under /broken.
func recipeSite(t *testing.T, names ...string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	serve := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, body)
		}
	}
	for _, n := range names {
		mux.HandleFunc("/"+n, serve(jsonldPage(n)))
	}
	mux.HandleFunc("/broken", serve("<html><body>no recipe</body></html>"))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// localOnly resolves 127.0.0.1 and nothing else: an unresolvable host is
// blocked without a DNS query.
func localOnly(host string) ([]net.IP, error) {
	if host == "127.0.0.1" {
		return []net.IP{net.ParseIP(host)}, nil
	}
	return nil, errors.New("no such host")
}

type staticProfile struct{ p *profile.Profile }

func (s staticProfile) Get(context.Context) (*profile.Profile, error) { return s.p, nil }

// fakePrefs is the discovery settings.
type fakePrefs struct {
	disabled    bool
	cuisines    []string
	cuisinesErr error
}

func (p *fakePrefs) DiscoveryDailyEnabled(context.Context) (bool, error) { return !p.disabled, nil }
func (p *fakePrefs) DiscoveryCuisines(context.Context) ([]string, error) {
	return p.cuisines, p.cuisinesErr
}

type harness struct {
	db     *sql.DB
	svc    *discovery.Service
	repo   *sqlite.RecipeSuggestionRepo
	foods  *sqlite.FoodRepo
	prefs  *fakePrefs
	images string
	site   *httptest.Server
	// annotator is the service's (mutable for fault injection); logs its
	// log output (read after the run).
	annotator *discovery.Annotator
	logs      *syncBuffer
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newHarness(t *testing.T, client llm.Client, site *httptest.Server) *harness {
	t.Helper()
	return newHarnessRepo(t, client, site, func(r *sqlite.RecipeSuggestionRepo) discovery.Repository { return r })
}

// newHarnessRepo is newHarness with the service's store reading through
// wrap(repo).
func newHarnessRepo(t *testing.T, client llm.Client, site *httptest.Server, wrap func(*sqlite.RecipeSuggestionRepo) discovery.Repository) *harness {
	t.Helper()
	db := testhelper.NewTestDB(t)
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), logger, time.Now)
	repo := sqlite.NewRecipeSuggestionRepo(db)
	foods := sqlite.NewFoodRepo(db)
	dir := t.TempDir()
	images, err := imagestore.New(dir, nil)
	require.NoError(t, err)
	resolver := llm.StaticResolver(nil, "")
	if client != nil {
		resolver = llm.StaticResolver(rec.Meter(client, "fake"), "fake-model")
	}
	prefs := &fakePrefs{}
	annotator := &discovery.Annotator{Matcher: food.NewResolver(foods, nil, nil, nil), Used: foods, Nutrition: food.NewNutritionResolver(foods)}
	svc := discovery.NewService(discovery.Deps{
		LLM:       resolver,
		Prefs:     prefs,
		Foods:     foods,
		Recorder:  rec,
		Profile:   staticProfile{promptProfile()},
		Catalogue: foods,
		Verifier: &discovery.Verifier{
			Extractor: importer.NewService(httpfetch.New(), jsonld.Extractor{}, nil, nil, nil),
			Index:     repo,
			Guard:     func(u string) error { return httpfetch.PublicURL(u, localOnly, true) },
			Logger:    logger,
		},
		Annotator: annotator,
		Store:     &discovery.Store{Repo: wrap(repo), Images: images},
		Logger:    logger,
	})
	t.Cleanup(func() { _ = svc.Shutdown(context.Background()) })
	return &harness{db: db, svc: svc, repo: repo, foods: foods, prefs: prefs, images: dir, site: site, annotator: annotator, logs: logs}
}

// fakeClient loads a script; {{SITE}} becomes the site's URL.
func fakeClient(t *testing.T, site *httptest.Server, script string) *fake.Client {
	t.Helper()
	var s fake.Script
	require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(script, "{{SITE}}", site.URL)), &s))
	return fake.NewFromScript(&s)
}

type runRow struct {
	Kind, PromptVersion, ScopeJSON, Status, Notes, ErrorKey, Instruction string
	WebSearches, RepairAttempts                                          int
	Outcome                                                              sql.NullString
}

func (h *harness) run(t *testing.T, id int64) runRow {
	t.Helper()
	var r runRow
	require.NoError(t, h.db.QueryRow(`SELECT kind, prompt_version, COALESCE(scope_json,''), status, COALESCE(notes,''),
		COALESCE(error_key,''), COALESCE(instruction,''), web_search_requests, repair_attempts, outcome FROM ai_runs WHERE id = ?`, id).
		Scan(&r.Kind, &r.PromptVersion, &r.ScopeJSON, &r.Status, &r.Notes, &r.ErrorKey, &r.Instruction,
			&r.WebSearches, &r.RepairAttempts, &r.Outcome))
	return r
}

func (h *harness) suggestions(t *testing.T, runID int64) []discovery.Suggestion {
	t.Helper()
	got, err := h.repo.List(context.Background(), discovery.ListFilter{RunID: &runID, Limit: 100})
	require.NoError(t, err)
	return got
}

func (h *harness) runAndWait(t *testing.T, req discovery.Request) int64 {
	t.Helper()
	id, err := h.svc.Run(context.Background(), req)
	require.NoError(t, err)
	require.NotZero(t, id)
	h.svc.Wait()
	return id
}

const candidatesJSON = `{\"candidates\": [` +
	`{\"url\": \"{{SITE}}/fesenjan\", \"title\": \"Fesenjan\", \"cuisine\": \"persian\", \"why\": \"Walnuts and pomegranate.\"},` +
	`{\"url\": \"{{SITE}}/pasta\", \"title\": \"Pasta\", \"cuisine\": \"italian\", \"why\": \"Quick.\"},` +
	`{\"url\": \"{{SITE}}/broken\", \"title\": \"Broken\"},` +
	`{\"url\": \"http://nowhere.invalid/x\", \"title\": \"Gone\"}]}`

const searchScript = `{"capabilities": {"web_search": true}, "turns": [
	{"match_user": "persian", "stop_reason": "pause_turn", "web_search_requests": 1,
	 "search_results": [{"url": "{{SITE}}/fesenjan", "title": "Fesenjan"}],
	 "message": {"content": [
		{"type": "server", "raw": {"type": "server_tool_use", "id": "srv_1", "name": "web_search", "input": {"query": "persian dinner"}}},
		{"type": "server", "raw": {"type": "web_search_tool_result", "tool_use_id": "srv_1", "content": [{"type": "web_search_result", "url": "{{SITE}}/fesenjan", "encrypted_content": "abc"}]}}]}},
	{"match_user": "persian", "stop_reason": "end_turn", "web_search_requests": 1,
	 "message": {"content": [{"type": "text", "text": "Here you go.\n` + candidatesJSON + `"}]}}
]}`

// B4-AC1: a daily run with a search-capable provider.
func TestService_DailyRunWithSearch(t *testing.T) {
	site := recipeSite(t, "fesenjan", "pasta")
	client := fakeClient(t, site, searchScript)
	h := newHarness(t, client, site)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily, Cuisine: "persian"})

	r := h.run(t, id)
	assert.Equal(t, "discovery", r.Kind)
	assert.Equal(t, "d1", r.PromptVersion)
	assert.JSONEq(t, `{"mode":"daily","cuisine":"persian"}`, r.ScopeJSON)
	assert.Equal(t, "persian", r.Instruction)
	assert.Equal(t, "ok", r.Status, r.ErrorKey)
	assert.Equal(t, 2, r.WebSearches)
	assert.Contains(t, r.Notes, "2 of 4 verified")
	assert.False(t, r.Outcome.Valid, "outcome stays NULL until a decision")

	sugs := h.suggestions(t, id)
	require.Len(t, sugs, 2)
	urls := map[string]discovery.Suggestion{}
	for _, s := range sugs {
		assert.Equal(t, discovery.StatusPending, s.Status)
		assert.True(t, s.Grounded)
		assert.Equal(t, discovery.ModeDaily, s.Mode)
		require.NotNil(t, s.Query)
		assert.Equal(t, "persian", *s.Query)
		urls[s.SourceURL] = s
	}
	fes := urls[site.URL+"/fesenjan"]
	assert.Equal(t, "Walnuts and pomegranate.", fes.Annotations.Why)
	assert.Equal(t, "persian", fes.Annotations.Cuisine)
	assert.Len(t, fes.Annotations.Lines, 2, "annotated")
	assert.Contains(t, urls, site.URL+"/pasta")

	// The pause turn was continued: the second request replays the
	// assistant's server blocks verbatim, with no new user message (D2).
	reqs := client.Requests()
	require.Len(t, reqs, 2)
	require.NotNil(t, reqs[0].WebSearch)
	assert.Equal(t, 5, reqs[0].WebSearch.MaxUses)
	assert.Empty(t, reqs[0].Tools)
	assert.Nil(t, reqs[0].ToolChoice, "no forced tool (D5)")
	require.Len(t, reqs[1].Messages, 2)
	assistant := reqs[1].Messages[1]
	assert.Equal(t, llm.RoleAssistant, assistant.Role)
	require.Len(t, assistant.Content, 2)
	assert.Equal(t, llm.ContentTypeServer, assistant.Content[0].Type)
	assert.Contains(t, string(assistant.Content[1].Raw), `"encrypted_content": "abc"`, "replayed verbatim")
}

// B4-AC2: the recorded request's system prompt carries the context.
func TestService_PromptContext(t *testing.T) {
	site := recipeSite(t, "fesenjan", "pasta")
	client := fakeClient(t, site, searchScript)
	h := newHarness(t, client, site)
	tooLong := discovery.ReasonTooLong
	old := &discovery.Suggestion{
		Mode: discovery.ModeDaily, SourceURL: "https://x.test/old",
		Draft: importer.Draft{Name: "Tofu Bowl", SourceURL: "https://x.test/old"},
	}
	require.NoError(t, h.repo.Create(context.Background(), old))
	_, err := h.repo.Decide(context.Background(), old.ID, discovery.Decision{Status: discovery.StatusRejected, Reason: &tooLong})
	require.NoError(t, err)

	h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily, Cuisine: "persian"})

	system := client.Requests()[0].System
	for _, want := range []string{"vegetarian", "peanuts", "We cook for two", "mushroom", "Tofu Bowl (takes too long)", "persian"} {
		assert.Contains(t, system, want)
	}
}

const knowledgeScript = `{"turns": [
	{"stop_reason": "end_turn", "message": {"content": [{"type": "text", "text": "` + candidatesJSON + `"}]}}
]}`

// B4-AC3: without the capability no search tool is sent and nothing is
// grounded; verification is the same.
func TestService_KnowledgeMode(t *testing.T) {
	site := recipeSite(t, "fesenjan", "pasta")
	client := fakeClient(t, site, knowledgeScript)
	h := newHarness(t, client, site)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeExplore, Cuisine: "thai"})

	reqs := client.Requests()
	require.Len(t, reqs, 1)
	assert.Nil(t, reqs[0].WebSearch)
	r := h.run(t, id)
	assert.Equal(t, "ok", r.Status)
	assert.Zero(t, r.WebSearches)
	assert.JSONEq(t, `{"mode":"explore","cuisine":"thai"}`, r.ScopeJSON)
	sugs := h.suggestions(t, id)
	require.Len(t, sugs, 2)
	for _, s := range sugs {
		assert.False(t, s.Grounded)
	}
}

// B4-AC3: a search-capable provider whose request ran without searches
// (the adapter retried it without the tool after an org-disabled 400) is
// not grounded.
func TestService_NoSearchesIsNotGrounded(t *testing.T) {
	site := recipeSite(t, "fesenjan", "pasta")
	client := fakeClient(t, site, strings.Replace(knowledgeScript, `{"turns"`, `{"capabilities": {"web_search": true}, "turns"`, 1))
	h := newHarness(t, client, site)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeKnown, Focus: "chickpeas"})

	assert.NotNil(t, client.Requests()[0].WebSearch)
	r := h.run(t, id)
	assert.JSONEq(t, `{"mode":"known","focus":"chickpeas"}`, r.ScopeJSON)
	for _, s := range h.suggestions(t, id) {
		assert.False(t, s.Grounded)
	}
}

// B4-AC1: at most three suggestions are stored per run.
func TestService_StoresAtMostThree(t *testing.T) {
	site := recipeSite(t, "a", "b", "c", "d")
	cands := `{\"candidates\": [{\"url\": \"{{SITE}}/a\"}, {\"url\": \"{{SITE}}/b\"}, {\"url\": \"{{SITE}}/c\"}, {\"url\": \"{{SITE}}/d\"}]}`
	client := fakeClient(t, site, `{"turns": [{"stop_reason": "end_turn", "message": {"content": [{"type": "text", "text": "`+cands+`"}]}}]}`)
	h := newHarness(t, client, site)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily})

	assert.Len(t, h.suggestions(t, id), 3)
	assert.Equal(t, "ok", h.run(t, id).Status)
}

// B4-AC4: invalid JSON once → one repair call.
func TestService_RepairsInvalidOutputOnce(t *testing.T) {
	site := recipeSite(t, "fesenjan", "pasta")
	client := fakeClient(t, site, `{"turns": [
		{"stop_reason": "end_turn", "message": {"content": [{"type": "text", "text": "Sure! Here are some ideas: fesenjan, pasta."}]}},
		{"match_user": "not valid JSON", "stop_reason": "end_turn", "message": {"content": [{"type": "text", "text": "`+candidatesJSON+`"}]}}
	]}`)
	h := newHarness(t, client, site)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily})

	reqs := client.Requests()
	require.Len(t, reqs, 2)
	require.Len(t, reqs[1].Messages, 3, "user, the invalid reply, the repair")
	assert.Equal(t, llm.RoleAssistant, reqs[1].Messages[1].Role)
	r := h.run(t, id)
	assert.Equal(t, "ok", r.Status)
	assert.Equal(t, 1, r.RepairAttempts)
	assert.Len(t, h.suggestions(t, id), 2)
	assert.Contains(t, h.logs.String(), "discovery.research repair", "the first parse error is logged")
	assert.Contains(t, h.logs.String(), "Here are some ideas")
}

// B4-AC4: invalid twice → error, nothing stored.
func TestService_InvalidOutputTwiceFails(t *testing.T) {
	site := recipeSite(t)
	client := fakeClient(t, site, `{"turns": [
		{"stop_reason": "end_turn", "message": {"content": [{"type": "text", "text": "{\"candidates\": \"nope\"}"}]}}
	]}`)
	h := newHarness(t, client, site)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily})

	assert.Len(t, client.Requests(), 2)
	r := h.run(t, id)
	assert.Equal(t, "error", r.Status)
	assert.Equal(t, "error.discovery.invalid_output", r.ErrorKey)
	assert.Empty(t, h.suggestions(t, id))
}

// B4-AC4: every candidate discarded → partial with "0 of N verified".
func TestService_AllDiscardedIsPartial(t *testing.T) {
	site := recipeSite(t)
	cands := `{\"candidates\": [{\"url\": \"{{SITE}}/broken\"}, {\"url\": \"http://nowhere.invalid/x\"}]}`
	client := fakeClient(t, site, `{"turns": [{"stop_reason": "end_turn", "message": {"content": [{"type": "text", "text": "`+cands+`"}]}}]}`)
	h := newHarness(t, client, site)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily})

	r := h.run(t, id)
	assert.Equal(t, "partial", r.Status)
	assert.Contains(t, r.Notes, "0 of 2 verified")
	assert.Empty(t, h.suggestions(t, id))
	assert.Contains(t, h.logs.String(), `level=WARN msg="discovery.run no_results"`)
	assert.Contains(t, h.logs.String(), "discovery.verify discarded")
}

// failFirstNutrition fails the first draft it computes.
type failFirstNutrition struct {
	discovery.DraftNutrition
	mu     sync.Mutex
	called bool
}

func (f *failFirstNutrition) PerPortionOfDraft(ctx context.Context, lines []food.DraftLine, portions float64) (food.Detailed, error) {
	f.mu.Lock()
	first := !f.called
	f.called = true
	f.mu.Unlock()
	if first {
		return food.Detailed{}, errors.New("database is locked")
	}
	return f.DraftNutrition.PerPortionOfDraft(ctx, lines, portions)
}

// DoD sfh-4: a candidate that cannot be annotated is discarded; the run
// keeps the others.
func TestService_AnnotateFailureDiscardsCandidate(t *testing.T) {
	site := recipeSite(t, "fesenjan", "pasta")
	h := newHarness(t, fakeClient(t, site, knowledgeScript), site)
	h.annotator.Nutrition = &failFirstNutrition{DraftNutrition: h.annotator.Nutrition}

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily})

	r := h.run(t, id)
	assert.Equal(t, "ok", r.Status)
	assert.Len(t, h.suggestions(t, id), 1)
	assert.Contains(t, r.Notes, "annotate 1")
	assert.Contains(t, h.logs.String(), "discovery.annotate failed")
}

func TestService_RunValidates(t *testing.T) {
	site := recipeSite(t)
	h := newHarness(t, fakeClient(t, site, knowledgeScript), site)
	_, err := h.svc.Run(context.Background(), discovery.Request{Mode: "weekly"})
	assert.ErrorIs(t, err, domain.ErrDiscoveryInvalidMode)
	_, err = h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeExplore, Cuisine: "  "})
	assert.ErrorIs(t, err, domain.ErrDiscoveryCuisineRequired)
	_, err = h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeKnown, Focus: strings.Repeat("f", 12000)})
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "focus over 80 runes")
	_, err = h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeExplore, Cuisine: strings.Repeat("ä", 81)})
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "cuisine over 80 runes")

	none := newHarness(t, nil, site)
	_, err = none.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeDaily})
	assert.ErrorIs(t, err, domain.ErrAIProviderMissing)
}

// blockingClient answers once release is closed (or its ctx ends).
type blockingClient struct {
	started chan struct{}
	release chan struct{}
	text    string
}

func (b *blockingClient) Stream(ctx context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	b.started <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.release:
	}
	return &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: b.text}}},
		StopReason: llm.StopReasonEndTurn,
	}, nil
}

func (b *blockingClient) Complete(context.Context, llm.Request) (string, error) {
	return "", errors.New("unused")
}

func newBlocking(site *httptest.Server) *blockingClient {
	return &blockingClient{
		started: make(chan struct{}, 4), release: make(chan struct{}),
		text: `{"candidates": [{"url": "` + site.URL + `/fesenjan"}]}`,
	}
}

// B4-AC5: one run at a time; a run survives its request's ctx.
func TestService_OneRunAtATimeAndDetached(t *testing.T) {
	site := recipeSite(t, "fesenjan")
	client := newBlocking(site)
	h := newHarness(t, client, site)

	ctx, cancel := context.WithCancel(context.Background())
	id, err := h.svc.Run(ctx, discovery.Request{Mode: discovery.ModeDaily})
	require.NoError(t, err)
	<-client.started
	cancel() // the request ends; the run goes on

	_, err = h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeDaily})
	assert.ErrorIs(t, err, domain.ErrDiscoveryRunning)

	close(client.release)
	h.svc.Wait()
	assert.Equal(t, "ok", h.run(t, id).Status)
	assert.Len(t, h.suggestions(t, id), 1)

	// The slot is free again.
	_, err = h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeDaily})
	assert.NoError(t, err)
	h.svc.Wait()
}

// B4-AC5: shutdown waits for the running run and refuses new ones.
func TestService_ShutdownWaits(t *testing.T) {
	site := recipeSite(t, "fesenjan")
	client := newBlocking(site)
	h := newHarness(t, client, site)

	id, err := h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeDaily})
	require.NoError(t, err)
	<-client.started

	done := make(chan error, 1)
	go func() { done <- h.svc.Shutdown(context.Background()) }()
	select {
	case <-done:
		t.Fatal("shutdown returned while the run was running")
	case <-time.After(50 * time.Millisecond):
	}
	close(client.release)
	require.NoError(t, <-done)
	assert.Equal(t, "ok", h.run(t, id).Status, "the run finished before shutdown returned")

	_, err = h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeDaily})
	assert.Error(t, err, "no run starts after shutdown")
}

// B4-AC5: a shutdown whose wait ends cancels the run, which still records
// its end.
func TestService_ShutdownDeadlineCancels(t *testing.T) {
	site := recipeSite(t, "fesenjan")
	client := newBlocking(site)
	h := newHarness(t, client, site)

	id, err := h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeDaily})
	require.NoError(t, err)
	<-client.started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, h.svc.Shutdown(ctx), context.DeadlineExceeded)
	r := h.run(t, id)
	assert.Equal(t, "error", r.Status)
}

// B4-AC6: pending suggestions older than 30 days are deleted, with their
// images, at the start of a run; decided ones stay.
func TestService_DeletesStalePending(t *testing.T) {
	site := recipeSite(t, "fesenjan", "pasta")
	h := newHarness(t, fakeClient(t, site, knowledgeScript), site)
	ctx := context.Background()
	mk := func(url string) int64 {
		s := &discovery.Suggestion{Mode: discovery.ModeDaily, SourceURL: url, Draft: importer.Draft{SourceURL: url}}
		require.NoError(t, h.repo.Create(ctx, s))
		return s.ID
	}
	stale, kept := mk("https://x.test/stale"), mk("https://x.test/rejected")
	_, err := h.repo.Decide(ctx, kept, discovery.Decision{Status: discovery.StatusRejected})
	require.NoError(t, err)
	_, err = h.db.Exec(`UPDATE recipe_suggestions SET created_at = datetime('now', '-31 days')`)
	require.NoError(t, err)
	img := filepath.Join(h.images, discovery.ImageCategory)
	require.NoError(t, os.MkdirAll(img, 0o755))
	staleImg := filepath.Join(img, "1.jpg")
	require.NoError(t, os.WriteFile(staleImg, []byte("x"), 0o644))
	require.Equal(t, int64(1), stale)

	h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily})

	_, err = h.repo.Get(ctx, stale)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.NoFileExists(t, staleImg)
	_, err = h.repo.Get(ctx, kept)
	assert.NoError(t, err)
}

// A failing stale cleanup is logged; the run still succeeds.
func TestService_StaleCleanupErrorDoesNotFailRun(t *testing.T) {
	site := recipeSite(t, "fesenjan", "pasta")
	h := newHarness(t, fakeClient(t, site, knowledgeScript), site)
	ctx := context.Background()
	s := &discovery.Suggestion{Mode: discovery.ModeDaily, SourceURL: "https://x.test/stale", Draft: importer.Draft{SourceURL: "https://x.test/stale"}}
	require.NoError(t, h.repo.Create(ctx, s))
	_, err := h.db.Exec(`UPDATE recipe_suggestions SET created_at = datetime('now', '-31 days')`)
	require.NoError(t, err)
	_, err = h.db.Exec(`CREATE TRIGGER no_delete BEFORE DELETE ON recipe_suggestions BEGIN SELECT RAISE(ABORT, 'boom'); END`)
	require.NoError(t, err)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily})

	assert.NotEqual(t, "error", h.run(t, id).Status)
}

func TestService_RunLowercasesCuisine(t *testing.T) {
	site := recipeSite(t)
	h := newHarness(t, fakeClient(t, site, knowledgeScript), site)
	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeExplore, Cuisine: "  Thai "})
	assert.Contains(t, h.run(t, id).ScopeJSON, `"cuisine":"thai"`)
}
