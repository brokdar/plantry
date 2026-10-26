package food_test

import (
	"context"
	"errors"
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
}

func (r *leafRepo) List(_ context.Context, q food.ListQuery) (*food.ListResult, error) {
	if q.Offset >= len(r.leaves) {
		return &food.ListResult{Total: len(r.leaves)}, nil
	}
	end := min(q.Offset+q.Limit, len(r.leaves))
	return &food.ListResult{Items: r.leaves[q.Offset:end], Total: len(r.leaves)}, nil
}

// stubProvider returns fixed candidates and records the queries it receives.
type stubProvider struct {
	results []food.Candidate
	queries []string
}

func (p *stubProvider) SearchByName(_ context.Context, query string, _ int) ([]food.Candidate, error) {
	p.queries = append(p.queries, query)
	return p.results, nil
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
