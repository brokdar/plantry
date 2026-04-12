package fdc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtureServer(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile("testdata/" + fixture)
	require.NoError(t, err)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
}

func TestSearchByName_MultipleResults(t *testing.T) {
	srv := fixtureServer(t, "search_chicken.json")
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	results, err := client.SearchByName(context.Background(), "chicken", nil, 10)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// First result: chicken breast
	r := results[0]
	assert.Equal(t, "Chicken, breast, meat only, cooked, roasted", r.Name)
	assert.Equal(t, 171077, r.FdcID)
	assert.Equal(t, "SR Legacy", r.DataType)
	assert.Equal(t, "Poultry Products", r.Category)
	require.NotNil(t, r.Kcal100g)
	assert.InDelta(t, 165.0, *r.Kcal100g, 0.01)
	require.NotNil(t, r.Protein100g)
	assert.InDelta(t, 31.02, *r.Protein100g, 0.01)
	require.NotNil(t, r.Fat100g)
	assert.InDelta(t, 3.57, *r.Fat100g, 0.01)
	require.NotNil(t, r.Carbs100g)
	assert.InDelta(t, 0.0, *r.Carbs100g, 0.01)
	require.NotNil(t, r.Fiber100g)
	assert.InDelta(t, 0.0, *r.Fiber100g, 0.01)
}

func TestSearchByName_EmptyResults(t *testing.T) {
	srv := fixtureServer(t, "search_empty.json")
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	results, err := client.SearchByName(context.Background(), "xyznonexistent", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchByName_SodiumConversion(t *testing.T) {
	srv := fixtureServer(t, "search_chicken.json")
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	results, err := client.SearchByName(context.Background(), "chicken", nil, 10)
	require.NoError(t, err)

	// First result: sodium 74 mg → 0.074 g
	require.NotNil(t, results[0].Sodium100g)
	assert.InDelta(t, 0.074, *results[0].Sodium100g, 0.0001)

	// Second result: sodium 700 mg → 0.7 g
	require.NotNil(t, results[1].Sodium100g)
	assert.InDelta(t, 0.7, *results[1].Sodium100g, 0.0001)

	// Third result: sodium 82 mg → 0.082 g
	require.NotNil(t, results[2].Sodium100g)
	assert.InDelta(t, 0.082, *results[2].Sodium100g, 0.0001)
}

func TestSearchByName_PartialNutrients(t *testing.T) {
	srv := fixtureServer(t, "search_chicken.json")
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	results, err := client.SearchByName(context.Background(), "chicken", nil, 10)
	require.NoError(t, err)

	// Third result (wing) has no fiber nutrient → nil, not zero.
	r := results[2]
	assert.Nil(t, r.Fiber100g, "missing nutrient should be nil, not zero")

	// But other nutrients should still be present.
	require.NotNil(t, r.Kcal100g)
	assert.InDelta(t, 203.0, *r.Kcal100g, 0.01)
}

func TestSearchByName_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	results, err := client.SearchByName(context.Background(), "chicken", nil, 10)
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "500")
}

func TestSearchByName_DataTypeFilter(t *testing.T) {
	var capturedQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalHits":0,"currentPage":1,"totalPages":0,"foods":[]}`))
	}))
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	_, err := client.SearchByName(context.Background(), "chicken", []string{"Foundation", "SR Legacy"}, 5)
	require.NoError(t, err)

	dtValues := capturedQuery["dataType"]
	require.Len(t, dtValues, 2)
	assert.Equal(t, "Foundation", dtValues[0])
	assert.Equal(t, "SR Legacy", dtValues[1])

	assert.Equal(t, "chicken", capturedQuery.Get("query"))
	assert.Equal(t, "5", capturedQuery.Get("pageSize"))
	assert.Equal(t, "1", capturedQuery.Get("pageNumber"))
}

func TestSearchByName_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalHits":0,"currentPage":1,"totalPages":0,"foods":[]}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	results, err := client.SearchByName(ctx, "chicken", nil, 10)
	require.Error(t, err)
	assert.Nil(t, results)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSearchByName_APIKeyInRequest(t *testing.T) {
	var capturedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.URL.Query().Get("api_key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalHits":0,"currentPage":1,"totalPages":0,"foods":[]}`))
	}))
	defer srv.Close()

	client := fdc.New("my-secret-key", fdc.WithBaseURL(srv.URL))
	_, err := client.SearchByName(context.Background(), "chicken", nil, 10)
	require.NoError(t, err)
	assert.Equal(t, "my-secret-key", capturedKey)
}
