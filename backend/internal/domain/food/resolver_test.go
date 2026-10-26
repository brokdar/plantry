package food_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// leafRepo serves a fixed list of leaf foods from List, honouring Limit and
// Offset so the resolver can page through the catalogue.
type leafRepo struct {
	fakeRepo
	leaves []food.Food
	err    error
}

func (r *leafRepo) List(_ context.Context, q food.ListQuery) (*food.ListResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	if q.Offset >= len(r.leaves) {
		return &food.ListResult{Total: len(r.leaves)}, nil
	}
	end := min(q.Offset+q.Limit, len(r.leaves))
	return &food.ListResult{Items: r.leaves[q.Offset:end], Total: len(r.leaves)}, nil
}

// stubProvider returns fixed candidates and records the queries it receives.
type stubProvider struct {
	results []food.Candidate
	err     error
	queries []string
}

func (p *stubProvider) SearchByName(_ context.Context, query string, _ int) ([]food.Candidate, error) {
	p.queries = append(p.queries, query)
	return p.results, p.err
}

func (p *stubProvider) LookupBarcode(_ context.Context, _ string) ([]food.Candidate, error) {
	return nil, nil
}

// spyLLM counts how often the translator / picker ask for an LLM client.
type spyLLM struct{ calls int }

func (s *spyLLM) Current(_ context.Context) (llm.Client, string, error) {
	s.calls++
	return nil, "", errors.New("disabled")
}

func leaf(id int64, name string) food.Food {
	return food.Food{ID: id, Kind: food.KindLeaf, Name: name}
}

func TestResolver_Query_LocalMatchBeforeFDC(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(1, "Gelbe Möhre"), leaf(2, "Möhre"), leaf(3, "Möhrensaft")}}
	fdc := &stubProvider{results: []food.Candidate{{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 11124}}}
	r := food.NewResolver(repo, nil, fdc, nil)

	results, rec, err := r.Lookup(context.Background(), "", "möhre", "de", 5)
	require.NoError(t, err)

	require.Len(t, results, 4)
	// exact, then prefix, then contains, then FDC.
	assert.Equal(t, "Möhre", results[0].Name)
	assert.Equal(t, "Möhrensaft", results[1].Name)
	assert.Equal(t, "Gelbe Möhre", results[2].Name)
	for _, c := range results[:3] {
		assert.Equal(t, food.SourceLocal, c.Source)
		require.NotNil(t, c.ExistingID)
	}
	assert.Equal(t, int64(2), *results[0].ExistingID)
	assert.Equal(t, food.SourceFDC, results[3].Source)
	assert.Equal(t, 0, rec)
}

func TestResolver_Query_OnlyExactLocalMatchIsMarkedExact(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(1, "Zwiebelsuppe"), leaf(2, " ZWIEBEL "), leaf(3, "Rote Zwiebel")}}
	r := food.NewResolver(repo, nil, nil, nil)

	results, _, err := r.Lookup(context.Background(), "", "Zwiebel", "de", 5)
	require.NoError(t, err)

	require.Len(t, results, 3)
	assert.Equal(t, int64(2), *results[0].ExistingID)
	assert.True(t, results[0].Exact, "normalized-equal local match")
	assert.False(t, results[1].Exact, "prefix match")
	assert.False(t, results[2].Exact, "contains match")
}

func TestResolver_Query_LocalCappedAtFive(t *testing.T) {
	repo := &leafRepo{}
	for i := range 8 {
		repo.leaves = append(repo.leaves, leaf(int64(i+1), "Reis "+string(rune('A'+i))))
	}
	r := food.NewResolver(repo, nil, nil, nil)

	results, _, err := r.Lookup(context.Background(), "", "reis", "de", 5)
	require.NoError(t, err)
	assert.Len(t, results, 5)
}

func TestResolver_Query_NoFDCKeyGivesLocalAndOFF(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(7, "Karotte")}}
	off := &stubProvider{results: []food.Candidate{{Name: "Bio Karotten", Source: food.SourceOFF, Barcode: "4001"}}}
	spy := &spyLLM{}
	r := food.NewResolver(repo, off, nil, spy)

	results, rec, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)

	require.Len(t, results, 2)
	assert.Equal(t, food.SourceLocal, results[0].Source)
	assert.Equal(t, food.SourceOFF, results[1].Source)
	assert.Equal(t, "Bio Karotten", results[1].Name)
	assert.Equal(t, []string{"Karotte"}, off.queries, "OFF gets the original query")
	assert.Equal(t, 0, rec)
	assert.Zero(t, spy.calls, "translator and picker only run when FDC is queried")
}

// configurableProvider is a stubProvider that reports whether it has a key.
type configurableProvider struct {
	stubProvider
	configured bool
}

func (p *configurableProvider) Configured(_ context.Context) bool { return p.configured }

func TestResolver_Query_UnconfiguredFDCIsSkipped(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(7, "Karottensaft")}}
	off := &stubProvider{results: []food.Candidate{{Name: "Bio Karotten", Source: food.SourceOFF, Barcode: "4001"}}}
	fdc := &configurableProvider{stubProvider: stubProvider{results: []food.Candidate{{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 1}}}}
	spy := &spyLLM{}
	r := food.NewResolver(repo, off, fdc, spy)

	results, rec, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)

	require.Len(t, results, 2)
	assert.Equal(t, food.SourceLocal, results[0].Source)
	assert.Equal(t, food.SourceOFF, results[1].Source)
	assert.Equal(t, 0, rec)
	assert.Empty(t, fdc.queries, "FDC is not searched without a key")
	assert.Zero(t, spy.calls, "translator and picker only run when FDC is queried")
}

func TestResolver_Query_ConfiguredFDCIsQueried(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(7, "Karottensaft")}}
	fdc := &configurableProvider{stubProvider: stubProvider{results: []food.Candidate{{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 1}}}, configured: true}
	spy := &spyLLM{}
	r := food.NewResolver(repo, nil, fdc, spy)

	results, _, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)

	require.Len(t, results, 2)
	assert.Equal(t, food.SourceFDC, results[1].Source)
	assert.Len(t, fdc.queries, 1)
	assert.Equal(t, 2, spy.calls, "translate + pick")
}

func TestResolver_Query_FDCDescriptionDoesNotOverrideLocalMatch(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(7, "Karotte"), leaf(8, "Carrots, raw")}}
	fdc := &stubProvider{results: []food.Candidate{{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 11124}}}
	r := food.NewResolver(repo, nil, fdc, nil)

	results, _, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)

	require.Len(t, results, 2)
	assert.Equal(t, food.SourceLocal, results[0].Source)
	assert.Equal(t, "Karotte", results[0].Name)
	require.NotNil(t, results[0].ExistingID)
	assert.Equal(t, int64(7), *results[0].ExistingID)

	fdcResult := results[1]
	assert.Equal(t, food.SourceFDC, fdcResult.Source)
	assert.Equal(t, "Karotte", fdcResult.Name)
	assert.Equal(t, "Carrots, raw", fdcResult.SourceName)
	require.NotNil(t, fdcResult.ExistingID, "FDC result is matched against the original query")
	assert.Equal(t, int64(7), *fdcResult.ExistingID)
}

func TestResolver_Query_DeduplicatesBySourceAndExternalID(t *testing.T) {
	repo := &leafRepo{}
	off := &stubProvider{results: []food.Candidate{
		{Name: "Hafer", Source: food.SourceOFF, Barcode: "1"},
		{Name: "Hafer", Source: food.SourceOFF, Barcode: "1"},
		{Name: "Haferflocken", Source: food.SourceOFF, Barcode: "2"},
	}}
	r := food.NewResolver(repo, off, nil, nil)

	results, _, err := r.Lookup(context.Background(), "", "hafer", "de", 5)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestResolver_Query_AIRunsWhenFDCQueried(t *testing.T) {
	repo := &leafRepo{}
	fdc := &stubProvider{results: []food.Candidate{
		{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 1},
		{Name: "Carrots, cooked", Source: food.SourceFDC, FdcID: 2},
	}}
	spy := &spyLLM{}
	r := food.NewResolver(repo, nil, fdc, spy)

	_, _, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)
	assert.Equal(t, 2, spy.calls, "translate + pick")
}

func TestResolver_Query_ExactLocalRecommendedWithoutPicker(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(7, "Karotte")}}
	fdc := &stubProvider{results: []food.Candidate{
		{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 1},
		{Name: "Carrots, cooked", Source: food.SourceFDC, FdcID: 2},
	}}
	spy := &spyLLM{}
	r := food.NewResolver(repo, nil, fdc, spy)

	results, rec, err := r.Lookup(context.Background(), "", "Karotte", "en", 5)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, 0, rec)
	assert.Equal(t, food.SourceLocal, results[0].Source)
	assert.Zero(t, spy.calls, "picker is skipped when an exact local match exists")
}

func TestResolver_Query_OFFErrorIsSkipped(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(7, "Karotte")}}
	fdc := &stubProvider{results: []food.Candidate{{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 1}}}
	off := &stubProvider{err: errors.New("off down")}
	r := food.NewResolver(repo, off, fdc, nil)

	results, _, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, food.SourceLocal, results[0].Source)
	assert.Equal(t, food.SourceFDC, results[1].Source)
}

func TestResolver_Query_FDCErrorKeepsOtherResults(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(7, "Karotte")}}
	fdc := &stubProvider{err: errors.New("fdc down")}
	off := &stubProvider{results: []food.Candidate{{Name: "Bio Karotten", Source: food.SourceOFF, Barcode: "4001"}}}
	r := food.NewResolver(repo, off, fdc, nil)

	results, rec, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, food.SourceLocal, results[0].Source)
	assert.Equal(t, food.SourceOFF, results[1].Source)
	assert.Equal(t, 0, rec)
}

func TestResolver_Query_FDCErrorWithoutOtherResultsFailsLookup(t *testing.T) {
	fdcErr := errors.New("fdc down")
	fdc := &stubProvider{err: fdcErr}
	r := food.NewResolver(&leafRepo{}, nil, fdc, nil)

	_, rec, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.ErrorIs(t, err, fdcErr)
	assert.Equal(t, -1, rec)
}

func TestResolver_Query_CatalogueErrorIsSkipped(t *testing.T) {
	repo := &leafRepo{err: errors.New("db locked")}
	off := &stubProvider{results: []food.Candidate{{Name: "Bio Karotten", Source: food.SourceOFF, Barcode: "4001"}}}
	r := food.NewResolver(repo, off, nil, nil)

	results, rec, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, food.SourceOFF, results[0].Source)
	assert.Nil(t, results[0].ExistingID)
	assert.Equal(t, 0, rec)
}

func TestResolver_Query_PagesThroughCatalogue(t *testing.T) {
	for _, n := range []int{1100, 1000} {
		t.Run(fmt.Sprintf("%d leaves", n), func(t *testing.T) {
			repo := &leafRepo{}
			for i := range n - 1 {
				repo.leaves = append(repo.leaves, leaf(int64(i+1), fmt.Sprintf("Filler %d", i)))
			}
			repo.leaves = append(repo.leaves, leaf(int64(n), "Pastinake"))
			r := food.NewResolver(repo, nil, nil, nil)

			results, _, err := r.Lookup(context.Background(), "", "pastinake", "de", 5)
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.NotNil(t, results[0].ExistingID)
			assert.Equal(t, int64(n), *results[0].ExistingID)
		})
	}
}

func TestResolver_Query_ExactMatchSurvivesCap(t *testing.T) {
	repo := &leafRepo{}
	for i := range 10 {
		repo.leaves = append(repo.leaves,
			leaf(int64(i+1), fmt.Sprintf("Apfel %d", i)),
			leaf(int64(i+100), fmt.Sprintf("Grüner Apfel %d", i)))
	}
	repo.leaves = append(repo.leaves, leaf(999, "Apfel"))
	r := food.NewResolver(repo, nil, nil, nil)

	results, _, err := r.Lookup(context.Background(), "", "äpfel", "de", 5)
	require.NoError(t, err)
	require.Len(t, results, 5)
	require.NotNil(t, results[0].ExistingID)
	assert.Equal(t, int64(999), *results[0].ExistingID)
}

func TestResolver_Query_BlankQuery(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(1, "Karotte")}}
	r := food.NewResolver(repo, nil, nil, nil)

	for _, q := range []string{"", "   "} {
		results, rec, err := r.Lookup(context.Background(), "", q, "de", 5)
		require.NoError(t, err)
		assert.Empty(t, results, "query %q", q)
		assert.Equal(t, -1, rec, "query %q", q)
	}
}

const lookupModel = "fake-lookup"

// newRecordedResolver builds a resolver whose LLM calls are metered into a
// real SQLite DB. llmResolver nil means a metered fake client with turns.
func newRecordedResolver(t *testing.T, repo food.Repository, fdc food.FoodProvider, llmResolver llm.Resolver, turns ...fake.Turn) (*food.Resolver, *sql.DB) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	if llmResolver == nil {
		llmResolver = llm.StaticResolver(rec.Meter(fake.NewFromScript(&fake.Script{Turns: turns}), "fake"), lookupModel)
	}
	return food.NewResolver(repo, nil, fdc, llmResolver).WithRecorder(rec), db
}

func lookupTurn(text, stopReason string, in, out int) fake.Turn {
	return fake.Turn{
		StopReason: stopReason,
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: text}}},
		Usage:      fake.ScriptedUsage{InputTokens: in, OutputTokens: out},
	}
}

func countRuns(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&n))
	return n
}

func TestResolver_Query_TranslateAndPickShareOneLookupRun(t *testing.T) {
	tests := []struct {
		name       string
		turns      []fake.Turn
		wantStatus string
		wantIn     int64
		wantOut    int64
		wantErrs   int
	}{
		{
			name: "ok",
			// The first turn must not be terminal: the fake rewinds after one.
			turns:      []fake.Turn{lookupTurn(`["carrot raw"]`, "max_tokens", 10, 2), lookupTurn(`[1]`, "end_turn", 20, 1)},
			wantStatus: "ok",
			wantIn:     30,
			wantOut:    3,
		},
		{
			name:       "client errors",
			turns:      nil, // empty script: every call fails
			wantStatus: "error",
			wantErrs:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &leafRepo{leaves: []food.Food{leaf(7, "Karottensaft")}}
			fdc := &stubProvider{results: []food.Candidate{
				{Name: "Carrot juice", Source: food.SourceFDC, FdcID: 1},
				{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 2},
			}}
			r, db := newRecordedResolver(t, repo, fdc, nil, tt.turns...)

			_, _, err := r.Lookup(context.Background(), "", "Karotte", "de", 5)
			require.NoError(t, err)

			require.Equal(t, 1, countRuns(t, db))
			var kind, provider, model, promptVersion, status string
			var iterations int
			var in, out int64
			var errKey sql.NullString
			require.NoError(t, db.QueryRow(`SELECT kind, provider, model, prompt_version, status, error_key,
				iterations, input_tokens, output_tokens FROM ai_runs`).Scan(
				&kind, &provider, &model, &promptVersion, &status, &errKey, &iterations, &in, &out))
			assert.Equal(t, "lookup", kind)
			assert.Equal(t, "fake", provider)
			assert.Equal(t, lookupModel, model)
			assert.Equal(t, food.PromptVersion, promptVersion)
			assert.Equal(t, tt.wantStatus, status)
			assert.False(t, errKey.Valid)
			assert.Equal(t, 2, iterations, "translate + pick")
			assert.Equal(t, tt.wantIn, in)
			assert.Equal(t, tt.wantOut, out)

			var steps, errs int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(is_error), 0) FROM ai_run_steps
				WHERE type = 'llm_call' AND name = ?`, lookupModel).Scan(&steps, &errs))
			assert.Equal(t, 2, steps)
			assert.Equal(t, tt.wantErrs, errs)
		})
	}
}

func TestResolver_Query_NoLLMCallStartsNoRun(t *testing.T) {
	fdcResults := []food.Candidate{
		{Name: "Carrot juice", Source: food.SourceFDC, FdcID: 1},
		{Name: "Carrots, raw", Source: food.SourceFDC, FdcID: 2},
	}
	tests := []struct {
		name        string
		leaves      []food.Food
		lang        string
		llmResolver llm.Resolver
	}{
		{name: "english query with exact local match", leaves: []food.Food{leaf(7, "Carrot")}, lang: "en"},
		{name: "llm not configured", leaves: []food.Food{leaf(7, "Karottensaft")}, lang: "de", llmResolver: &spyLLM{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fdc := &stubProvider{results: fdcResults}
			r, db := newRecordedResolver(t, &leafRepo{leaves: tt.leaves}, fdc, tt.llmResolver,
				lookupTurn(`[0]`, "end_turn", 1, 1))

			_, _, err := r.Lookup(context.Background(), "", "Carrot", tt.lang, 5)
			require.NoError(t, err)
			assert.Len(t, fdc.queries, 1, "FDC is queried")
			assert.Zero(t, countRuns(t, db))
		})
	}
}
