package handlers_test

import (
	"bytes"
	"encoding/json"
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

func setupRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	svc := ingredient.NewService(repo)
	h := handlers.NewIngredientHandler(svc)

	r := chi.NewRouter()
	r.Route("/api/ingredients", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
	return r
}

func TestCreateIngredient_201(t *testing.T) {
	r := setupRouter(t)
	body := `{"name":"Chicken Breast","kcal_100g":165,"protein_100g":31}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(body))
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "Chicken Breast", got["name"])
	assert.NotZero(t, got["id"])
}

func TestCreateIngredient_409_Duplicate(t *testing.T) {
	r := setupRouter(t)
	body := `{"name":"Tofu"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(body))
	resp1 := httptest.NewRecorder()
	r.ServeHTTP(resp1, req1)
	require.Equal(t, http.StatusCreated, resp1.Code)

	req2 := httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(body))
	resp2 := httptest.NewRecorder()
	r.ServeHTTP(resp2, req2)
	assert.Equal(t, http.StatusConflict, resp2.Code)
}

func TestCreateIngredient_400_InvalidJSON(t *testing.T) {
	r := setupRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{bad`))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateIngredient_400_EmptyName(t *testing.T) {
	r := setupRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":""}`))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestGetIngredient_200(t *testing.T) {
	r := setupRouter(t)
	// create first
	body := `{"name":"Rice"}`
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, createResp.Code)
	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))

	// get
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients/1", nil))
	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "Rice", got["name"])
}

func TestGetIngredient_404(t *testing.T) {
	r := setupRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients/999", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestGetIngredient_400_InvalidID(t *testing.T) {
	r := setupRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients/abc", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpdateIngredient_200(t *testing.T) {
	r := setupRouter(t)
	// create
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"Rice"}`)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	// update
	updateBody := `{"name":"Brown Rice","kcal_100g":112}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/ingredients/1", bytes.NewBufferString(updateBody)))
	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "Brown Rice", got["name"])
}

func TestUpdateIngredient_404(t *testing.T) {
	r := setupRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/ingredients/999", bytes.NewBufferString(`{"name":"X"}`)))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestDeleteIngredient_204(t *testing.T) {
	r := setupRouter(t)
	// create
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"Butter"}`)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	// delete
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/ingredients/1", nil))
	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestDeleteIngredient_404(t *testing.T) {
	r := setupRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/ingredients/999", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestListIngredients_200(t *testing.T) {
	r := setupRouter(t)
	for _, name := range []string{"Apple", "Banana", "Cherry"} {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"`+name+`"}`)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients", nil))
	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	items := got["items"].([]any)
	assert.Len(t, items, 3)
	assert.Equal(t, float64(3), got["total"])
}

func TestListIngredients_Search(t *testing.T) {
	r := setupRouter(t)
	for _, name := range []string{"Chicken Breast", "Chicken Thigh", "Tofu"} {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"`+name+`"}`)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients?search=chicken", nil))
	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, float64(2), got["total"])
}

func TestListIngredients_Pagination(t *testing.T) {
	r := setupRouter(t)
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"`+name+`"}`)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients?limit=2&offset=0", nil))
	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	items := got["items"].([]any)
	assert.Len(t, items, 2)
	assert.Equal(t, float64(5), got["total"])
}

func TestUpdateIngredient_400_InvalidJSON(t *testing.T) {
	r := setupRouter(t)
	// create first
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"Rice"}`)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/ingredients/1", bytes.NewBufferString(`{bad`)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpdateIngredient_400_EmptyName(t *testing.T) {
	r := setupRouter(t)
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"Rice"}`)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/ingredients/1", bytes.NewBufferString(`{"name":""}`)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpdateIngredient_400_InvalidID(t *testing.T) {
	r := setupRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/ingredients/abc", bytes.NewBufferString(`{"name":"X"}`)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpdateIngredient_409_DuplicateName(t *testing.T) {
	r := setupRouter(t)
	for _, name := range []string{"Alpha", "Beta"} {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"`+name+`"}`)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/ingredients/2", bytes.NewBufferString(`{"name":"Alpha"}`)))
	assert.Equal(t, http.StatusConflict, resp.Code)
}

func TestUpdateIngredient_200_RoundTrip(t *testing.T) {
	r := setupRouter(t)
	// create
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"Rice"}`)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	// update
	updateResp := httptest.NewRecorder()
	r.ServeHTTP(updateResp, httptest.NewRequest(http.MethodPut, "/api/ingredients/1", bytes.NewBufferString(`{"name":"Brown Rice","kcal_100g":112}`)))
	require.Equal(t, http.StatusOK, updateResp.Code)

	// verify via GET
	getResp := httptest.NewRecorder()
	r.ServeHTTP(getResp, httptest.NewRequest(http.MethodGet, "/api/ingredients/1", nil))
	assert.Equal(t, http.StatusOK, getResp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	assert.Equal(t, "Brown Rice", got["name"])
	assert.Equal(t, float64(112), got["kcal_100g"])
}

func TestDeleteIngredient_400_InvalidID(t *testing.T) {
	r := setupRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/ingredients/abc", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestListIngredients_NegativeLimit(t *testing.T) {
	r := setupRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients?limit=-1", nil))
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestListIngredients_InvalidSort(t *testing.T) {
	r := setupRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients?sort=nonexistent", nil))
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestListIngredients_FTSSpecialChars(t *testing.T) {
	r := setupRouter(t)
	// create some data
	for _, name := range []string{"Chicken Breast", "Tofu"} {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"`+name+`"}`)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients?search=chicken+AND+tofu", nil))
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestListIngredients_SortDesc(t *testing.T) {
	r := setupRouter(t)
	for _, name := range []string{"Apple", "Banana", "Cherry"} {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(`{"name":"`+name+`"}`)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/ingredients?sort=name&order=desc", nil))
	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	items := got["items"].([]any)
	require.Len(t, items, 3)
	first := items[0].(map[string]any)
	last := items[2].(map[string]any)
	assert.Equal(t, "Cherry", first["name"])
	assert.Equal(t, "Apple", last["name"])
}
