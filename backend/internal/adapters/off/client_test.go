package off

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := New(WithBaseURL(srv.URL))
	return srv, client
}

func fixtureHandler(t *testing.T, fixture string) http.HandlerFunc {
	t.Helper()
	data, err := os.ReadFile("testdata/" + fixture)
	require.NoError(t, err)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}
}

func TestLookupBarcode_Found(t *testing.T) {
	_, client := newTestServer(t, fixtureHandler(t, "barcode_3017620422003.json"))

	candidates, err := client.LookupBarcode(context.Background(), "3017620422003", "en")
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	c := candidates[0]
	assert.Equal(t, "Nutella", c.Name)
	assert.Equal(t, "Ferrero", c.Brand)
	assert.Equal(t, "3017620422003", c.Barcode)
	assert.Equal(t, "https://images.openfoodfacts.org/images/products/301/762/042/2003/front_en.820.200.jpg", c.ImageURL)

	require.NotNil(t, c.Kcal100g)
	assert.InDelta(t, 539.0, *c.Kcal100g, 0.01)
	require.NotNil(t, c.Protein100g)
	assert.InDelta(t, 6.3, *c.Protein100g, 0.01)
	require.NotNil(t, c.Fat100g)
	assert.InDelta(t, 30.9, *c.Fat100g, 0.01)
	require.NotNil(t, c.Carbs100g)
	assert.InDelta(t, 57.5, *c.Carbs100g, 0.01)
	assert.Nil(t, c.Fiber100g)
	require.NotNil(t, c.Sodium100g)
	assert.InDelta(t, 0.0428, *c.Sodium100g, 0.0001)
}

func TestLookupBarcode_NotFound(t *testing.T) {
	_, client := newTestServer(t, fixtureHandler(t, "barcode_not_found.json"))

	candidates, err := client.LookupBarcode(context.Background(), "0000000000000", "en")
	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestSearchByName_MultipleResults(t *testing.T) {
	_, client := newTestServer(t, fixtureHandler(t, "search_oats.json"))

	candidates, err := client.SearchByName(context.Background(), "oats", "en", 10)
	require.NoError(t, err)
	require.Len(t, candidates, 3)

	assert.Equal(t, "Rolled Oats", candidates[0].Name)
	assert.Equal(t, "Bob's Red Mill", candidates[0].Brand)

	assert.Equal(t, "Instant Oatmeal", candidates[1].Name)
	assert.Equal(t, "Quaker", candidates[1].Brand)

	assert.Equal(t, "Steel Cut Oats", candidates[2].Name)
}

func TestSearchByName_EmptyResults(t *testing.T) {
	_, client := newTestServer(t, fixtureHandler(t, "search_empty.json"))

	candidates, err := client.SearchByName(context.Background(), "xyznonexistent", "en", 10)
	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestLookupBarcode_ServerError(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	candidates, err := client.LookupBarcode(context.Background(), "3017620422003", "en")
	require.Error(t, err)
	assert.Nil(t, candidates)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestLookupBarcode_PartialNutriments(t *testing.T) {
	_, client := newTestServer(t, fixtureHandler(t, "barcode_partial_nutriments.json"))

	candidates, err := client.LookupBarcode(context.Background(), "5000159484695", "en")
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	c := candidates[0]
	assert.Equal(t, "Twix Ice cream", c.Name)

	// These are present in the fixture (even if zero-valued).
	require.NotNil(t, c.Kcal100g)
	assert.InDelta(t, 0.0, *c.Kcal100g, 0.01)
	require.NotNil(t, c.Protein100g)
	assert.InDelta(t, 0.0, *c.Protein100g, 0.01)

	// These are absent from the fixture and should be nil.
	assert.Nil(t, c.Fat100g)
	assert.Nil(t, c.Carbs100g)
	assert.Nil(t, c.Fiber100g)
	assert.Nil(t, c.Sodium100g)
}

func TestLookupBarcode_NameLocalization(t *testing.T) {
	// barcode_8000500310427 has distinct base/en/de names so both branches are meaningful.
	_, client := newTestServer(t, fixtureHandler(t, "barcode_8000500310427.json"))

	// English: should pick product_name_en, not the French base name.
	enCandidates, err := client.LookupBarcode(context.Background(), "8000500310427", "en")
	require.NoError(t, err)
	require.Len(t, enCandidates, 1)
	assert.Equal(t, "nutella biscuits", enCandidates[0].Name)

	// German: should pick product_name_de.
	deCandidates, err := client.LookupBarcode(context.Background(), "8000500310427", "de")
	require.NoError(t, err)
	require.Len(t, deCandidates, 1)
	assert.Equal(t, "Knusprige Kekse mit einem cremigen Herz aus Nutella\u00ae", deCandidates[0].Name)
}

func TestLookupBarcode_ExtendedNutriments(t *testing.T) {
	_, client := newTestServer(t, fixtureHandler(t, "barcode_extended.json"))

	candidates, err := client.LookupBarcode(context.Background(), "1111111111111", "en")
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	c := candidates[0]

	require.NotNil(t, c.SaturatedFat100g)
	assert.InDelta(t, 1.2, *c.SaturatedFat100g, 0.001)
	require.NotNil(t, c.TransFat100g)
	assert.InDelta(t, 0.05, *c.TransFat100g, 0.001)
	require.NotNil(t, c.Cholesterol100g)
	assert.InDelta(t, 20.0, *c.Cholesterol100g, 0.001)
	require.NotNil(t, c.Sugar100g)
	assert.InDelta(t, 12.0, *c.Sugar100g, 0.001)
	require.NotNil(t, c.Potassium100g)
	assert.InDelta(t, 400.0, *c.Potassium100g, 0.001)
	require.NotNil(t, c.Calcium100g)
	assert.InDelta(t, 150.0, *c.Calcium100g, 0.001)
	require.NotNil(t, c.Iron100g)
	assert.InDelta(t, 2.1, *c.Iron100g, 0.001)
	require.NotNil(t, c.Magnesium100g)
	assert.InDelta(t, 60.0, *c.Magnesium100g, 0.001)
	require.NotNil(t, c.Phosphorus100g)
	assert.InDelta(t, 180.0, *c.Phosphorus100g, 0.001)
	require.NotNil(t, c.Zinc100g)
	assert.InDelta(t, 1.5, *c.Zinc100g, 0.001)
	require.NotNil(t, c.VitaminA100g)
	assert.InDelta(t, 80.0, *c.VitaminA100g, 0.001)
	require.NotNil(t, c.VitaminC100g)
	assert.InDelta(t, 20.0, *c.VitaminC100g, 0.001)
	require.NotNil(t, c.VitaminD100g)
	assert.InDelta(t, 1.5, *c.VitaminD100g, 0.001)
	require.NotNil(t, c.VitaminB12100g)
	assert.InDelta(t, 0.8, *c.VitaminB12100g, 0.001)
	require.NotNil(t, c.VitaminB6100g)
	assert.InDelta(t, 0.3, *c.VitaminB6100g, 0.001)
	require.NotNil(t, c.Folate100g)
	assert.InDelta(t, 90.0, *c.Folate100g, 0.001)
}

func TestSearchByName_ContextCancelled(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// This handler would block, but the context is already cancelled.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"page":1,"page_size":10,"products":[]}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	candidates, err := client.SearchByName(ctx, "oats", "en", 10)
	require.Error(t, err)
	assert.Nil(t, candidates)
	assert.ErrorIs(t, err, context.Canceled)
}
