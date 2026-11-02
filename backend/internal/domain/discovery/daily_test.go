package discovery_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// batchClient answers call i with candidates for batches[i] (page names on
// site); past the last batch it answers with no candidates.
type batchClient struct {
	site    *httptest.Server
	batches [][]string
	mu      sync.Mutex
	calls   int
}

func (b *batchClient) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	b.mu.Lock()
	var names []string
	if b.calls < len(b.batches) {
		names = b.batches[b.calls]
	}
	b.calls++
	b.mu.Unlock()
	cands := make([]string, len(names))
	for i, n := range names {
		cands[i] = `{"url": "` + b.site.URL + `/` + n + `", "title": "` + n + `"}`
	}
	text := `{"candidates": [` + strings.Join(cands, ",") + `]}`
	return &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}},
		StopReason: llm.StopReasonEndTurn,
	}, nil
}

func (b *batchClient) Complete(context.Context, llm.Request) (string, error) {
	return "", errors.New("unused")
}

func (b *batchClient) Calls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

var (
	dayD  = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	dayD1 = dayD.AddDate(0, 0, 1)
)

// daily calls Daily and, when it started (or found) a run, waits for it.
func (h *harness) daily(t *testing.T, day time.Time) discovery.Daily {
	t.Helper()
	d, err := h.svc.Daily(context.Background(), day)
	require.NoError(t, err)
	return d
}

func (h *harness) another(t *testing.T, day time.Time) discovery.Daily {
	t.Helper()
	d, err := h.svc.Another(context.Background(), day)
	require.NoError(t, err)
	return d
}

func (h *harness) get(t *testing.T, id int64) *discovery.Suggestion {
	t.Helper()
	s, err := h.repo.Get(context.Background(), id)
	require.NoError(t, err)
	return s
}

// B5-AC1: no pending → a run; after it the same suggestion all day; the
// next day the next pending one without a new run.
func TestDaily_PoolAndShownOn(t *testing.T) {
	site := recipeSite(t, "r1", "r2", "r3")
	client := &batchClient{site: site, batches: [][]string{{"r1", "r2", "r3"}}}
	h := newHarness(t, client, site)
	h.prefs.cuisines = []string{"persian", "thai"}

	first := h.daily(t, dayD)
	require.Nil(t, first.Suggestion)
	require.NotZero(t, first.RunID, "202: a run started")
	h.svc.Wait()

	run := h.run(t, first.RunID)
	assert.Equal(t, "ok", run.Status, run.ErrorKey)
	cuisine := discovery.DailyCuisine(h.prefs.cuisines, dayD)
	assert.JSONEq(t, `{"mode":"daily","cuisine":"`+cuisine+`","day":"2026-11-02"}`, run.ScopeJSON)

	got := h.daily(t, dayD)
	require.NotNil(t, got.Suggestion, "200 after the run")
	require.NotNil(t, got.Suggestion.ShownOn)
	assert.Equal(t, "2026-11-02", *got.Suggestion.ShownOn)
	again := h.daily(t, dayD)
	require.NotNil(t, again.Suggestion)
	assert.Equal(t, got.Suggestion.ID, again.Suggestion.ID, "the same suggestion all day")

	next := h.daily(t, dayD1)
	require.NotNil(t, next.Suggestion)
	assert.NotEqual(t, got.Suggestion.ID, next.Suggestion.ID)
	assert.Equal(t, "2026-11-03", *next.Suggestion.ShownOn)
	assert.Equal(t, discovery.StatusPending, next.Suggestion.Status)
	assert.Equal(t, 1, client.Calls(), "no new run on D+1")
}

// B5-AC2: daily disabled or no provider → nothing and no run.
func TestDaily_DisabledOrNoProvider(t *testing.T) {
	site := recipeSite(t, "r1")
	client := &batchClient{site: site, batches: [][]string{{"r1"}}}
	h := newHarness(t, client, site)
	h.prefs.disabled = true
	assert.Equal(t, discovery.Daily{}, h.daily(t, dayD))
	assert.Equal(t, discovery.Daily{}, h.another(t, dayD))
	assert.Zero(t, client.Calls())

	none := newHarness(t, nil, site)
	assert.Equal(t, discovery.Daily{}, none.daily(t, dayD))
	assert.Equal(t, discovery.Daily{}, none.another(t, dayD))
}

// B5-AC2: a run that failed today → nothing for the rest of the day; the
// next day tries again.
func TestDaily_FailedRunSilentForTheDay(t *testing.T) {
	site := recipeSite(t, "r1")
	client := &batchClient{site: site, batches: [][]string{{"broken"}, {"r1"}}}
	h := newHarness(t, client, site)

	require.NotZero(t, h.daily(t, dayD).RunID)
	h.svc.Wait()
	assert.Equal(t, discovery.Daily{}, h.daily(t, dayD))
	assert.Equal(t, discovery.Daily{}, h.another(t, dayD))
	assert.Equal(t, 1, client.Calls())

	require.NotZero(t, h.daily(t, dayD1).RunID, "a new day tries again")
	h.svc.Wait()
	got := h.daily(t, dayD1)
	require.NotNil(t, got.Suggestion)
	assert.Equal(t, site.URL+"/r1", got.Suggestion.SourceURL)
}

// A running run answers with its id instead of starting another.
func TestDaily_RunningRunIsReturned(t *testing.T) {
	site := recipeSite(t, "fesenjan")
	client := newBlocking(site)
	h := newHarness(t, client, site)

	id, err := h.svc.Run(context.Background(), discovery.Request{Mode: discovery.ModeExplore, Cuisine: "thai"})
	require.NoError(t, err)
	<-client.started
	assert.Equal(t, discovery.Daily{RunID: id}, h.daily(t, dayD))
	close(client.release)
	h.svc.Wait()
}

// B5-AC3: another → the next pending (the previous stays pending, shown_on
// cleared), then a run when the pool is empty.
func TestDaily_Another(t *testing.T) {
	site := recipeSite(t, "r1", "r2", "r3", "r4")
	client := &batchClient{site: site, batches: [][]string{{"r1", "r2", "r3"}, {"r4"}}}
	h := newHarness(t, client, site)

	h.daily(t, dayD)
	h.svc.Wait()
	s1 := h.daily(t, dayD).Suggestion
	require.NotNil(t, s1)

	s2 := h.another(t, dayD).Suggestion
	require.NotNil(t, s2)
	assert.NotEqual(t, s1.ID, s2.ID)
	assert.Equal(t, "2026-11-02", *s2.ShownOn)
	prev := h.get(t, s1.ID)
	assert.Nil(t, prev.ShownOn, "shown_on cleared")
	assert.Equal(t, discovery.StatusPending, prev.Status, "still pending")

	s3 := h.another(t, dayD).Suggestion
	require.NotNil(t, s3)
	assert.NotContains(t, []int64{s1.ID, s2.ID}, s3.ID)

	started := h.another(t, dayD)
	require.Nil(t, started.Suggestion)
	require.NotZero(t, started.RunID, "pool empty → a run")
	h.svc.Wait()
	assert.Nil(t, h.get(t, s3.ID).ShownOn)

	s4 := h.daily(t, dayD).Suggestion
	require.NotNil(t, s4, "the new run's suggestion is today's")
	assert.Equal(t, site.URL+"/r4", s4.SourceURL)
	assert.Equal(t, 2, client.Calls())
}

func (h *harness) newRecipe(t *testing.T, image *string) *food.Food {
	t.Helper()
	portions := 2.0
	f := &food.Food{Name: "Saved " + t.Name(), Kind: food.KindComposed, Role: ptr(food.RoleMain), ReferencePortions: &portions, ImagePath: image}
	require.NoError(t, h.foods.Create(context.Background(), f))
	return f
}

// withImage stores an image file for the suggestion.
func (h *harness) withImage(t *testing.T, id int64) []byte {
	t.Helper()
	data := pngBytes(t)
	require.NoError(t, os.MkdirAll(filepath.Join(h.images, discovery.ImageCategory), 0o755))
	path := filepath.Join(discovery.ImageCategory, fmt.Sprintf("%d.jpg", id))
	require.NoError(t, os.WriteFile(filepath.Join(h.images, path), data, 0o644))
	require.NoError(t, h.repo.SetImagePath(context.Background(), id, &path))
	return data
}

// B5-AC4: accept sets food_id and the outcome and copies the image to a
// food without one; a second decision is refused.
func TestService_AcceptCopiesImage(t *testing.T) {
	site := recipeSite(t, "r1", "r2")
	client := &batchClient{site: site, batches: [][]string{{"r1", "r2"}}}
	h := newHarness(t, client, site)
	runID := h.runAndWait(t, discovery.Request{Mode: discovery.ModeExplore, Cuisine: "thai"})
	sugs := h.suggestions(t, runID)
	require.Len(t, sugs, 2)
	ctx := context.Background()

	data := h.withImage(t, sugs[0].ID)
	f := h.newRecipe(t, nil)
	got, err := h.svc.Accept(ctx, sugs[0].ID, f.ID)
	require.NoError(t, err)
	assert.Equal(t, discovery.StatusAccepted, got.Status)
	require.NotNil(t, got.FoodID)
	assert.Equal(t, f.ID, *got.FoodID)
	assert.Equal(t, "accepted", h.run(t, runID).Outcome.String)

	saved, err := h.foods.Get(ctx, f.ID)
	require.NoError(t, err)
	require.NotNil(t, saved.ImagePath, "image copied")
	copied, err := os.ReadFile(filepath.Join(h.images, *saved.ImagePath))
	require.NoError(t, err)
	assert.Equal(t, data, copied)

	_, err = h.svc.Accept(ctx, sugs[0].ID, f.ID)
	assert.ErrorIs(t, err, domain.ErrDiscoveryDecided)
	_, err = h.svc.Reject(ctx, sugs[0].ID, nil)
	assert.ErrorIs(t, err, domain.ErrDiscoveryDecided)

	// A food with an image keeps it.
	h.withImage(t, sugs[1].ID)
	own := "foods/own.jpg"
	g := h.newRecipe(t, &own)
	_, err = h.svc.Accept(ctx, sugs[1].ID, g.ID)
	require.NoError(t, err)
	kept, err := h.foods.Get(ctx, g.ID)
	require.NoError(t, err)
	assert.Equal(t, own, *kept.ImagePath)

	_, err = h.svc.Accept(ctx, 9999, f.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// B5-AC4: accepting as a food that does not exist is not found, and the
// suggestion stays pending.
func TestService_AcceptUnknownFood(t *testing.T) {
	site := recipeSite(t, "r1")
	client := &batchClient{site: site, batches: [][]string{{"r1"}}}
	h := newHarness(t, client, site)
	runID := h.runAndWait(t, discovery.Request{Mode: discovery.ModeExplore, Cuisine: "thai"})
	sug := h.suggestions(t, runID)[0]

	_, err := h.svc.Accept(context.Background(), sug.ID, 4242)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, discovery.StatusPending, h.get(t, sug.ID).Status)
}

// GET /runs/{id}: status, suggestion ids and the discarded count.
func TestService_RunState(t *testing.T) {
	site := recipeSite(t, "r1")
	client := &batchClient{site: site, batches: [][]string{{"r1", "broken"}}}
	h := newHarness(t, client, site)
	runID := h.runAndWait(t, discovery.Request{Mode: discovery.ModeExplore, Cuisine: "thai"})

	st, err := h.svc.RunState(context.Background(), runID)
	require.NoError(t, err)
	assert.Equal(t, runID, st.RunID)
	assert.Equal(t, "ok", st.Status)
	assert.Equal(t, []int64{h.suggestions(t, runID)[0].ID}, st.SuggestionIDs)
	assert.Equal(t, 1, st.Discarded)
	assert.Nil(t, st.ErrorKey)

	_, err = h.svc.RunState(context.Background(), runID+100)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// A decided suggestion keeps the day: daily and another stay silent and
// start no run.
func TestDaily_DecidedSuggestionSilencesTheDay(t *testing.T) {
	for _, decide := range []string{"reject", "accept"} {
		t.Run(decide, func(t *testing.T) {
			site := recipeSite(t, "r1", "r2")
			client := &batchClient{site: site, batches: [][]string{{"r1", "r2"}}}
			h := newHarness(t, client, site)
			h.daily(t, dayD)
			h.svc.Wait()
			s1 := h.daily(t, dayD).Suggestion
			require.NotNil(t, s1)

			if decide == "reject" {
				_, err := h.svc.Reject(context.Background(), s1.ID, nil)
				require.NoError(t, err)
			} else {
				_, err := h.svc.Accept(context.Background(), s1.ID, h.newRecipe(t, nil).ID)
				require.NoError(t, err)
			}
			assert.Equal(t, discovery.Daily{}, h.daily(t, dayD))
			assert.Equal(t, discovery.Daily{}, h.another(t, dayD))
			assert.Equal(t, 1, client.Calls(), "no new run")
			require.NotNil(t, h.get(t, s1.ID).ShownOn, "shown_on kept")
			require.NotNil(t, h.daily(t, dayD1).Suggestion, "the next day shows the next pending one")
		})
	}
}

// Another with a failed run today and no further pending leaves the
// current suggestion shown.
func TestDaily_AnotherFailedRunKeepsCurrent(t *testing.T) {
	site := recipeSite(t, "r1")
	client := &batchClient{site: site, batches: [][]string{{"r1"}}}
	h := newHarness(t, client, site)
	h.daily(t, dayD)
	h.svc.Wait()
	s1 := h.daily(t, dayD).Suggestion
	require.NotNil(t, s1)
	_, err := h.db.Exec(`INSERT INTO ai_runs (kind, started_at, status, scope_json) VALUES ('discovery', datetime('now'), 'error', ?)`,
		`{"mode":"daily","day":"2026-11-02"}`)
	require.NoError(t, err)

	assert.Equal(t, discovery.Daily{}, h.another(t, dayD))
	require.NotNil(t, h.get(t, s1.ID).ShownOn, "shown_on intact")
	got := h.daily(t, dayD)
	require.NotNil(t, got.Suggestion)
	assert.Equal(t, s1.ID, got.Suggestion.ID)
}

// Concurrent daily requests while a run is being set up all get its id.
func TestDaily_ConcurrentRequestsGetTheRunID(t *testing.T) {
	site := recipeSite(t, "fesenjan")
	client := newBlocking(site)
	h := newHarness(t, client, site)

	const n = 8
	ids := make([]int64, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := h.svc.Daily(context.Background(), dayD)
			assert.NoError(t, err)
			ids[i] = d.RunID
		}()
	}
	wg.Wait()
	for _, id := range ids {
		assert.NotZero(t, id)
		assert.Equal(t, ids[0], id)
	}
	close(client.release)
	h.svc.Wait()
}

// staleShownOn reports no suggestion shown on the first lookup, as a
// concurrent request would leave it.
type staleShownOn struct {
	discovery.Repository
	once sync.Once
}

func (s *staleShownOn) ShownOn(ctx context.Context, day string) (*discovery.Suggestion, error) {
	stale := false
	s.once.Do(func() { stale = true })
	if stale {
		return nil, nil
	}
	return s.Repository.ShownOn(ctx, day)
}

// A run whose day was taken meanwhile still stores its suggestions.
func TestService_DayTakenMeanwhileKeepsSuggestions(t *testing.T) {
	site := recipeSite(t, "r1", "r2")
	client := &batchClient{site: site, batches: [][]string{{"r1"}, {"r2"}}}
	h := newHarnessRepo(t, client, site, func(r *sqlite.RecipeSuggestionRepo) discovery.Repository {
		return &staleShownOn{Repository: r}
	})
	// Explore runs look up no day, so the stale lookup is the daily run's.
	first := h.runAndWait(t, discovery.Request{Mode: discovery.ModeExplore, Cuisine: "thai"})
	taken, err := h.repo.ShowNext(context.Background(), "2026-11-02", 0)
	require.NoError(t, err)
	require.NotNil(t, taken)
	require.Len(t, h.suggestions(t, first), 1)

	id := h.runAndWait(t, discovery.Request{Mode: discovery.ModeDaily, Cuisine: "thai", Day: "2026-11-02"})
	assert.Equal(t, "ok", h.run(t, id).Status, h.run(t, id).ErrorKey)
	got := h.suggestions(t, id)
	require.Len(t, got, 1)
	assert.Nil(t, got[0].ShownOn)
}
