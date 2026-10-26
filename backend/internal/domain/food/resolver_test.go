package food_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
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

func TestResolver_Query_FDCErrorFailsLookup(t *testing.T) {
	repo := &leafRepo{leaves: []food.Food{leaf(7, "Karotte")}}
	fdcErr := errors.New("fdc down")
	fdc := &stubProvider{err: fdcErr}
	r := food.NewResolver(repo, nil, fdc, nil)

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
