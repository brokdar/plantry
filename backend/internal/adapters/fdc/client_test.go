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
	assert.Equal(t, "Chicken, broilers or fryers, breast, meat only, cooked, roasted", r.Name)
	assert.Equal(t, 171477, r.FdcID)
	assert.Equal(t, "SR Legacy", r.DataType)
	assert.Equal(t, "Poultry Products", r.Category)
	require.NotNil(t, r.Kcal100g)
	assert.InDelta(t, 165.0, *r.Kcal100g, 0.01)
	require.NotNil(t, r.Protein100g)
	assert.InDelta(t, 31.0, *r.Protein100g, 0.01)
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

	// Second result: sodium 106 mg → 0.106 g
	require.NotNil(t, results[1].Sodium100g)
	assert.InDelta(t, 0.106, *results[1].Sodium100g, 0.0001)

	// Third result: sodium 92 mg → 0.092 g
	require.NotNil(t, results[2].Sodium100g)
	assert.InDelta(t, 0.092, *results[2].Sodium100g, 0.0001)
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

func TestSearchByName_ExtendedNutrients(t *testing.T) {
	srv := fixtureServer(t, "search_extended.json")
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	results, err := client.SearchByName(context.Background(), "test", nil, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	r := results[0]

	require.NotNil(t, r.SaturatedFat100g)
	assert.InDelta(t, 1.5, *r.SaturatedFat100g, 0.001)
	require.NotNil(t, r.TransFat100g)
	assert.InDelta(t, 0.1, *r.TransFat100g, 0.001)
	require.NotNil(t, r.Cholesterol100g)
	assert.InDelta(t, 25.0, *r.Cholesterol100g, 0.001)
	require.NotNil(t, r.Sugar100g)
	assert.InDelta(t, 8.0, *r.Sugar100g, 0.001)
	require.NotNil(t, r.Potassium100g)
	assert.InDelta(t, 300.0, *r.Potassium100g, 0.001)
	require.NotNil(t, r.Calcium100g)
	assert.InDelta(t, 120.0, *r.Calcium100g, 0.001)
	require.NotNil(t, r.Iron100g)
	assert.InDelta(t, 2.5, *r.Iron100g, 0.001)
	require.NotNil(t, r.Magnesium100g)
	assert.InDelta(t, 45.0, *r.Magnesium100g, 0.001)
	require.NotNil(t, r.Phosphorus100g)
	assert.InDelta(t, 200.0, *r.Phosphorus100g, 0.001)
	require.NotNil(t, r.Zinc100g)
	assert.InDelta(t, 1.8, *r.Zinc100g, 0.001)
	require.NotNil(t, r.VitaminA100g)
	assert.InDelta(t, 50.0, *r.VitaminA100g, 0.001)
	require.NotNil(t, r.VitaminC100g)
	assert.InDelta(t, 15.0, *r.VitaminC100g, 0.001)
	require.NotNil(t, r.VitaminD100g)
	assert.InDelta(t, 1.2, *r.VitaminD100g, 0.001)
	require.NotNil(t, r.VitaminB12100g)
	assert.InDelta(t, 0.6, *r.VitaminB12100g, 0.001)
	require.NotNil(t, r.VitaminB6100g)
	assert.InDelta(t, 0.4, *r.VitaminB6100g, 0.001)
	require.NotNil(t, r.Folate100g)
	assert.InDelta(t, 100.0, *r.Folate100g, 0.001)
}

func TestSearchByName_ExtendedNutrientsNilWhenMissing(t *testing.T) {
	srv := fixtureServer(t, "search_chicken.json")
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	results, err := client.SearchByName(context.Background(), "chicken", nil, 10)
	require.NoError(t, err)

	// Chicken fixture only has the original 6 nutrients; extended should be nil.
	r := results[0]
	assert.Nil(t, r.SaturatedFat100g)
	assert.Nil(t, r.Sugar100g)
	assert.Nil(t, r.Potassium100g)
	assert.Nil(t, r.VitaminC100g)
	assert.Nil(t, r.Folate100g)
}

func TestGetFood_ParsesFoodPortions(t *testing.T) {
	srv := fixtureServer(t, "food_honey.json")
	defer srv.Close()

	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	food, err := client.GetFood(context.Background(), 169640)
	require.NoError(t, err)
	require.NotNil(t, food)
	assert.Equal(t, 169640, food.FdcID)
	assert.Equal(t, "Honey", food.Description)

	// Zero-gramWeight entries are filtered out.
	require.Len(t, food.FoodPortions, 3)

	tbsp := food.FoodPortions[0]
	assert.InDelta(t, 21.0, tbsp.GramWeight, 0.001)
	assert.Equal(t, "undetermined", tbsp.MeasureUnitName)
	assert.Equal(t, "tbsp", tbsp.Modifier)

	tsp := food.FoodPortions[1]
	assert.InDelta(t, 7.0, tsp.GramWeight, 0.001)
	assert.Equal(t, "tsp", tsp.Modifier)

	cup := food.FoodPortions[2]
	assert.InDelta(t, 339.0, cup.GramWeight, 0.001)
	assert.Equal(t, "cup", cup.MeasureUnitName)
	assert.Empty(t, cup.Modifier)
}

func TestGetFood_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	food, err := client.GetFood(context.Background(), 999999)
	require.Error(t, err)
	assert.Nil(t, food)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetFood_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	client := fdc.New("test-key", fdc.WithBaseURL(srv.URL))
	_, err := client.GetFood(context.Background(), 169640)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
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
