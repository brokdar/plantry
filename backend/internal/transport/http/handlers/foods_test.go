package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func setupFoodRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	svc := food.NewService(repo)
	resolver := food.NewNutritionResolver(repo)
	h := handlers.NewFoodHandler(svc, resolver, nil)
	r := chi.NewRouter()
	r.Route("/api/foods", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/insights", h.Insights)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.Update)
			r.Delete("/", h.Delete)
			r.Post("/favorite", h.SetFavorite)
			r.Get("/portions", h.ListPortions)
			r.Post("/portions", h.UpsertPortion)
			r.Delete("/portions/{unit}", h.DeletePortion)
		})
	})
	return r
}

// createLeaf creates a minimal leaf food via the API and returns its ID.
func createLeaf(t *testing.T, r http.Handler, name string) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "kind": "leaf"})
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewReader(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return int64(got["id"].(float64))
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestFoodHandler_Create_Leaf(t *testing.T) {
	r := setupFoodRouter(t)
	body := `{"name":"Chicken Breast","kind":"leaf","kcal_100g":165}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotZero(t, got["id"])
	assert.Equal(t, "Chicken Breast", got["name"])
	assert.Equal(t, "leaf", got["kind"])
}

func TestFoodHandler_Create_MissingName(t *testing.T) {
	r := setupFoodRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods",
		bytes.NewBufferString(`{"name":"","kind":"leaf"}`)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestFoodHandler_Create_MalformedJSON(t *testing.T) {
	r := setupFoodRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods",
		bytes.NewBufferString(`{bad json`)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestFoodHandler_Create_DuplicateName(t *testing.T) {
	r := setupFoodRouter(t)
	createLeaf(t, r, "Rice")

	body := `{"name":"Rice","kind":"leaf"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	assert.Equal(t, http.StatusConflict, resp.Code)
}

func TestFoodHandler_Create_InvalidKind(t *testing.T) {
	r := setupFoodRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods",
		bytes.NewBufferString(`{"name":"Weird","kind":"snack"}`)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestFoodHandler_Get(t *testing.T) {
	r := setupFoodRouter(t)
	id := createLeaf(t, r, "Olive Oil")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/foods/%d", id), nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "Olive Oil", got["name"])
}

func TestFoodHandler_Get_NotFound(t *testing.T) {
	r := setupFoodRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/foods/99999", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestFoodHandler_Get_NonNumericID(t *testing.T) {
	r := setupFoodRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/foods/abc", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestFoodHandler_Update(t *testing.T) {
	r := setupFoodRouter(t)
	id := createLeaf(t, r, "Butter")

	body := `{"name":"Unsalted Butter","kind":"leaf"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%d", id),
		bytes.NewBufferString(body)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "Unsalted Butter", got["name"])
}

func TestFoodHandler_Update_NotFound(t *testing.T) {
	r := setupFoodRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/foods/99999",
		bytes.NewBufferString(`{"name":"Ghost","kind":"leaf"}`)))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestFoodHandler_Delete(t *testing.T) {
	r := setupFoodRouter(t)
	id := createLeaf(t, r, "Temp Food")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/foods/%d", id), nil))
	assert.Equal(t, http.StatusNoContent, resp.Code)

	// Confirm gone.
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/foods/%d", id), nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestFoodHandler_Delete_NotFound(t *testing.T) {
	r := setupFoodRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/foods/99999", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestFoodHandler_List(t *testing.T) {
	r := setupFoodRouter(t)
	createLeaf(t, r, "Apple")
	createLeaf(t, r, "Banana")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/foods/?limit=10", nil))
	require.Equal(t, http.StatusOK, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.GreaterOrEqual(t, int(got["total"].(float64)), 2)
}

func TestFoodHandler_List_InvalidKind(t *testing.T) {
	r := setupFoodRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/foods/?kind=snack", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

// ── SetFavorite ───────────────────────────────────────────────────────────────

func TestFoodHandler_SetFavorite(t *testing.T) {
	r := setupFoodRouter(t)
	id := createLeaf(t, r, "Avocado")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/foods/%d/favorite", id),
		bytes.NewBufferString(`{"favorite":true}`)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, true, got["favorite"])
}

// ── Portions ──────────────────────────────────────────────────────────────────

func TestFoodHandler_Portions_UpsertAndList(t *testing.T) {
	r := setupFoodRouter(t)
	id := createLeaf(t, r, "Oats")

	// Upsert a portion.
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/foods/%d/portions", id),
		bytes.NewBufferString(`{"unit":"cup","grams":90}`)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	// List portions.
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/foods/%d/portions", id), nil))
	require.Equal(t, http.StatusOK, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	items, ok := got["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Equal(t, "cup", items[0].(map[string]any)["unit"])
}
