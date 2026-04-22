package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type stubBarcodeProvider struct {
	candidates []ingredient.Candidate
}

func (s *stubBarcodeProvider) LookupBarcode(_ context.Context, _ string) ([]ingredient.Candidate, error) {
	return s.candidates, nil
}

func (s *stubBarcodeProvider) SearchByName(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
	return s.candidates, nil
}

type stubFoodProvider struct {
	candidates []ingredient.Candidate
}

func (s *stubFoodProvider) SearchByName(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
	return s.candidates, nil
}

func setupLookupRouter(t *testing.T, off ingredient.BarcodeProvider, fdc ingredient.FoodProvider) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	svc := ingredient.NewService(repo)
	resolver := ingredient.NewResolver(repo, off, fdc, nil)
	lh := handlers.NewLookupHandler(resolver, nil, svc)

	r := chi.NewRouter()
	r.Route("/api/ingredients", func(r chi.Router) {
		r.Get("/", handlers.NewIngredientHandler(svc).List)
		r.Post("/", handlers.NewIngredientHandler(svc).Create)
		r.Get("/lookup", lh.Lookup)
		r.Post("/resolve", lh.Resolve)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", handlers.NewIngredientHandler(svc).Get)
			r.Put("/", handlers.NewIngredientHandler(svc).Update)
			r.Post("/refetch", lh.Refetch)
		})
	})
	return r
}

func pf64(v float64) *float64 { return &v }

func TestLookup_WithBarcode(t *testing.T) {
	off := &stubBarcodeProvider{
		candidates: []ingredient.Candidate{
			{Name: "Test Product", Source: "off", Barcode: "123", Kcal100g: pf64(100)},
		},
	}
	r := setupLookupRouter(t, off, nil)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ingredients/lookup?barcode=123", nil)
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	results := got["results"].([]any)
	assert.Len(t, results, 1)
	first := results[0].(map[string]any)
	assert.Equal(t, "Test Product", first["name"])
}

func TestLookup_WithQuery(t *testing.T) {
	fdc := &stubFoodProvider{
		candidates: []ingredient.Candidate{
			{Name: "Chicken Breast", Source: "fdc", Kcal100g: pf64(165)},
		},
	}
	r := setupLookupRouter(t, nil, fdc)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ingredients/lookup?query=chicken", nil)
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	results := got["results"].([]any)
	assert.Len(t, results, 1)
}

func TestLookup_DebugParamAttachesTrace(t *testing.T) {
	fdc := &stubFoodProvider{
		candidates: []ingredient.Candidate{
			{Name: "Chicken", Source: "fdc", Kcal100g: pf64(120)},
		},
	}
	r := setupLookupRouter(t, nil, fdc)

	// Without ?debug=true — trace must be absent.
	respPlain := httptest.NewRecorder()
	reqPlain := httptest.NewRequest(http.MethodGet, "/api/ingredients/lookup?query=chicken", nil)
	r.ServeHTTP(respPlain, reqPlain)
	require.Equal(t, http.StatusOK, respPlain.Code)
	var plain map[string]any
	require.NoError(t, json.NewDecoder(respPlain.Body).Decode(&plain))
	_, hasTrace := plain["trace"]
	assert.False(t, hasTrace, "trace must not leak into the response when debug is off")

	// With ?debug=true — trace must be populated.
	respDbg := httptest.NewRecorder()
	reqDbg := httptest.NewRequest(http.MethodGet, "/api/ingredients/lookup?query=chicken&debug=true", nil)
	r.ServeHTTP(respDbg, reqDbg)
	require.Equal(t, http.StatusOK, respDbg.Code)
	var dbg map[string]any
	require.NoError(t, json.NewDecoder(respDbg.Body).Decode(&dbg))
	trace, ok := dbg["trace"].([]any)
	require.True(t, ok, "trace array must be present when debug=true")
	assert.NotEmpty(t, trace, "trace should have at least the FDC search entry")
}

func TestLookup_MissingParams(t *testing.T) {
	r := setupLookupRouter(t, nil, nil)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ingredients/lookup", nil)
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestLookup_NoResults(t *testing.T) {
	off := &stubBarcodeProvider{candidates: nil}
	fdc := &stubFoodProvider{candidates: nil}
	r := setupLookupRouter(t, off, fdc)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ingredients/lookup?barcode=000", nil)
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	results := got["results"].([]any)
	assert.Len(t, results, 0)
	assert.Equal(t, float64(-1), got["recommended_index"])
}

func TestResolve_CreatesIngredient(t *testing.T) {
	r := setupLookupRouter(t, nil, nil)

	body := `{"name":"Resolved Chicken","source":"fdc","kcal_100g":165,"protein_100g":31,"fat_100g":3.6,"carbs_100g":0,"fiber_100g":0,"sodium_100g":0.074}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients/resolve", bytes.NewBufferString(body))
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "Resolved Chicken", got["name"])
	assert.NotZero(t, got["id"])
	assert.Equal(t, "fdc", got["source"])
}

func TestResolve_InvalidBody(t *testing.T) {
	r := setupLookupRouter(t, nil, nil)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients/resolve", bytes.NewBufferString(`{bad`))
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestRefetch_UpdatesMacrosFromOFF(t *testing.T) {
	off := &stubBarcodeProvider{
		candidates: []ingredient.Candidate{{
			Name:        "Refreshed",
			Source:      "off",
			Barcode:     "7770",
			Kcal100g:    pf64(250),
			Protein100g: pf64(12),
			Fat100g:     pf64(8),
			Carbs100g:   pf64(30),
			Sugar100g:   pf64(20),
		}},
	}
	r := setupLookupRouter(t, off, nil)

	// Create with stale values + stored barcode.
	create := `{"name":"Local","source":"off","barcode":"7770","kcal_100g":100,"protein_100g":5,"fat_100g":2,"carbs_100g":10,"fiber_100g":0,"sodium_100g":0}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(create))
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	id := int64(created["id"].(float64))

	// Refetch.
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/ingredients/%d/refetch", id), nil)
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	// Core macros overwritten.
	assert.Equal(t, float64(250), got["kcal_100g"])
	assert.Equal(t, float64(12), got["protein_100g"])
	// Extended macro now present.
	assert.Equal(t, float64(20), got["sugar_100g"])
	// Name preserved.
	assert.Equal(t, "Local", got["name"])
}

func TestRefetch_MissingSourceIDs(t *testing.T) {
	r := setupLookupRouter(t, nil, nil)

	create := `{"name":"ManualOnly","source":"manual","kcal_100g":100,"protein_100g":5,"fat_100g":2,"carbs_100g":10,"fiber_100g":0,"sodium_100g":0}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(create))
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	id := int64(created["id"].(float64))

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/ingredients/%d/refetch", id), nil)
	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestResolve_EmptyName(t *testing.T) {
	r := setupLookupRouter(t, nil, nil)

	body := `{"name":"","source":"fdc","kcal_100g":100}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients/resolve", bytes.NewBufferString(body))
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func setupLookupRouterWithImageStore(t *testing.T, imgStore *imagestore.Store) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	svc := ingredient.NewService(repo)
	resolver := ingredient.NewResolver(repo, nil, nil, nil)
	lh := handlers.NewLookupHandler(resolver, imgStore, svc)

	r := chi.NewRouter()
	r.Route("/api/ingredients", func(r chi.Router) {
		r.Get("/", handlers.NewIngredientHandler(svc).List)
		r.Post("/", handlers.NewIngredientHandler(svc).Create)
		r.Get("/lookup", lh.Lookup)
		r.Post("/resolve", lh.Resolve)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", handlers.NewIngredientHandler(svc).Get)
		})
	})
	return r
}

func TestResolve_ImageDownloadFailure(t *testing.T) {
	// HTTP server that always returns 500.
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	imgStore, err := imagestore.New(t.TempDir(), &http.Client{})
	require.NoError(t, err)

	router := setupLookupRouterWithImageStore(t, imgStore)

	body := fmt.Sprintf(
		`{"name":"Fail Image Ingredient","source":"off","kcal_100g":100,"protein_100g":10,"fat_100g":5,"carbs_100g":20,"fiber_100g":2,"sodium_100g":0.1,"image_url":"%s/fail.jpg"}`,
		failSrv.URL,
	)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients/resolve", bytes.NewBufferString(body))
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "Fail Image Ingredient", got["name"])
	assert.NotZero(t, got["id"])
	// image_path should be absent (omitempty) because download failed.
	assert.Nil(t, got["image_path"], "image_path should be nil when image download fails")
}
