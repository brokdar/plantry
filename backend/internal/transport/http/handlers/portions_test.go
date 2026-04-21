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

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func setupPortionRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	svc := ingredient.NewService(repo)
	h := handlers.NewIngredientHandler(svc)

	r := chi.NewRouter()
	r.Route("/api/ingredients", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.Update)
			r.Delete("/", h.Delete)
			r.Get("/portions", h.ListPortions)
			r.Post("/portions", h.UpsertPortion)
			r.Delete("/portions/{unit}", h.DeletePortion)
		})
	})
	return r
}

func createIngredientViaAPI(t *testing.T, router http.Handler, name string) float64 {
	t.Helper()
	body := `{"name":"` + name + `"}`
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return got["id"].(float64)
}

func TestListPortions_OK(t *testing.T) {
	r := setupPortionRouter(t)
	createIngredientViaAPI(t, r, "Rice")

	// Upsert two portions
	for _, body := range []string{
		`{"unit":"cup","grams":185}`,
		`{"unit":"tbsp","grams":15}`,
	} {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients/1/portions", bytes.NewBufferString(body)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	// List
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients/1/portions", nil))
	assert.Equal(t, http.StatusOK, resp.Code)

	var got []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Len(t, got, 2)
}

func TestListPortions_InvalidID(t *testing.T) {
	r := setupPortionRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients/abc/portions", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestListPortions_IngredientNotFound(t *testing.T) {
	r := setupPortionRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients/999/portions", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestUpsertPortion_Created(t *testing.T) {
	r := setupPortionRouter(t)
	createIngredientViaAPI(t, r, "Rice")

	body := `{"unit":"cup","grams":185}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients/1/portions", bytes.NewBufferString(body)))
	assert.Equal(t, http.StatusCreated, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "cup", got["unit"])
	assert.Equal(t, 185.0, got["grams"])
	assert.Equal(t, 1.0, got["ingredient_id"])
}

func TestUpsertPortion_InvalidBody(t *testing.T) {
	r := setupPortionRouter(t)
	createIngredientViaAPI(t, r, "Rice")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients/1/portions", bytes.NewBufferString(`{bad`)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpsertPortion_EmptyUnit(t *testing.T) {
	r := setupPortionRouter(t)
	createIngredientViaAPI(t, r, "Rice")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients/1/portions", bytes.NewBufferString(`{"unit":"","grams":185}`)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpsertPortion_IngredientNotFound(t *testing.T) {
	r := setupPortionRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients/999/portions", bytes.NewBufferString(`{"unit":"cup","grams":185}`)))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestDeletePortion_NoContent(t *testing.T) {
	r := setupPortionRouter(t)
	createIngredientViaAPI(t, r, "Rice")

	// Create portion first
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients/1/portions", bytes.NewBufferString(`{"unit":"cup","grams":185}`)))
	require.Equal(t, http.StatusCreated, resp.Code)

	// Delete it
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/ingredients/1/portions/cup", nil))
	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestDeletePortion_NotFound(t *testing.T) {
	r := setupPortionRouter(t)
	createIngredientViaAPI(t, r, "Rice")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/ingredients/1/portions/nonexistent", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

// --- sync-portions tests ---

type stubPortionProvider struct {
	portions []ingredient.FoodPortion
	err      error
}

func (s *stubPortionProvider) GetFoodPortions(_ context.Context, _ int) ([]ingredient.FoodPortion, error) {
	return s.portions, s.err
}

func setupSyncRouter(t *testing.T, provider *stubPortionProvider) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	svc := ingredient.NewService(repo).WithPortionProvider(provider)
	h := handlers.NewIngredientHandler(svc)

	r := chi.NewRouter()
	r.Route("/api/ingredients", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/portions", h.ListPortions)
			r.Post("/sync-portions", h.SyncPortions)
		})
	})
	return r
}

func TestSyncPortions_OK(t *testing.T) {
	provider := &stubPortionProvider{
		portions: []ingredient.FoodPortion{
			{RawUnit: "cup", GramWeight: 339},
			{RawUnit: "undetermined", Modifier: "tbsp", GramWeight: 21},
		},
	}
	r := setupSyncRouter(t, provider)
	body := `{"name":"Honey","source":"fdc","fdc_id":"169640"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	id := created["id"].(float64)

	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/ingredients/%d/sync-portions", int(id)), nil))
	require.Equal(t, http.StatusOK, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, 2.0, got["added"])
	portions, _ := got["portions"].([]any)
	assert.Len(t, portions, 2)
}

func TestSyncPortions_NoFdcID(t *testing.T) {
	r := setupSyncRouter(t, &stubPortionProvider{})
	body := `{"name":"Manual"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code)

	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients/1/sync-portions", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestSyncPortions_InvalidID(t *testing.T) {
	r := setupSyncRouter(t, &stubPortionProvider{})
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients/abc/sync-portions", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestDeletePortion_InvalidID(t *testing.T) {
	r := setupPortionRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/ingredients/abc/portions/cup", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}
