package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func setupComponentRouter(t *testing.T) (http.Handler, *sqlite.IngredientRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	iRepo := sqlite.NewIngredientRepo(db)
	cRepo := sqlite.NewComponentRepo(db)
	cSvc := component.NewService(cRepo, iRepo, iRepo)
	h := handlers.NewComponentHandler(cSvc, nil)

	r := chi.NewRouter()
	r.Route("/api/components", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.Update)
			r.Delete("/", h.Delete)
			r.Get("/nutrition", h.Nutrition)
		})
	})
	return r, iRepo
}

func seedTestIngredient(t *testing.T, iRepo *sqlite.IngredientRepo, name string, kcal, protein float64) *ingredient.Ingredient {
	t.Helper()
	i := &ingredient.Ingredient{Name: name, Source: "manual", Kcal100g: kcal, Protein100g: protein}
	require.NoError(t, iRepo.Create(t.Context(), i))
	return i
}

func TestCreateComponent_201(t *testing.T) {
	router, iRepo := setupComponentRouter(t)
	ing := seedTestIngredient(t, iRepo, "Chicken", 165, 31)

	body := fmt.Sprintf(`{
		"name":"Chicken Curry",
		"role":"main",
		"reference_portions":2,
		"prep_minutes":10,
		"cook_minutes":30,
		"ingredients":[{"ingredient_id":%d,"amount":300,"unit":"g","grams":300,"sort_order":0}],
		"instructions":[{"step_number":1,"text":"Cook chicken"}],
		"tags":["spicy"]
	}`, ing.ID)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(body))
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "Chicken Curry", got["name"])
	assert.NotZero(t, got["id"])
	assert.Equal(t, "main", got["role"])

	ingredients, ok := got["ingredients"].([]any)
	require.True(t, ok)
	assert.Len(t, ingredients, 1)
}

func TestCreateComponent_400_InvalidRole(t *testing.T) {
	router, _ := setupComponentRouter(t)
	body := `{"name":"Test","role":"appetizer","reference_portions":1}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(body))
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateComponent_400_EmptyName(t *testing.T) {
	router, _ := setupComponentRouter(t)
	body := `{"name":"","role":"main","reference_portions":1}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(body))
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateComponent_400_MalformedJSON(t *testing.T) {
	router, _ := setupComponentRouter(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString("{bad"))
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestGetComponent_200(t *testing.T) {
	router, iRepo := setupComponentRouter(t)
	ing := seedTestIngredient(t, iRepo, "Tofu", 76, 8)

	createBody := fmt.Sprintf(`{
		"name":"Tofu Bowl",
		"role":"main",
		"reference_portions":1,
		"ingredients":[{"ingredient_id":%d,"amount":200,"unit":"g","grams":200}],
		"instructions":[{"step_number":1,"text":"Slice tofu"}],
		"tags":["vegan"]
	}`, ing.ID)

	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(createBody)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))

	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/components/%.0f", created["id"]), nil))

	assert.Equal(t, http.StatusOK, getResp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	assert.Equal(t, "Tofu Bowl", got["name"])

	ingredients, _ := got["ingredients"].([]any)
	assert.Len(t, ingredients, 1)
	instructions, _ := got["instructions"].([]any)
	assert.Len(t, instructions, 1)
	tags, _ := got["tags"].([]any)
	assert.Equal(t, []any{"vegan"}, tags)
}

func TestGetComponent_404(t *testing.T) {
	router, _ := setupComponentRouter(t)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/components/999", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestUpdateComponent_200(t *testing.T) {
	router, _ := setupComponentRouter(t)
	createBody := `{"name":"Original","role":"main","reference_portions":1}`
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(createBody)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))

	updateBody := `{"name":"Updated","role":"side_veg","reference_portions":2,"tags":["new"]}`
	updateResp := httptest.NewRecorder()
	router.ServeHTTP(updateResp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/components/%.0f", created["id"]), bytes.NewBufferString(updateBody)))

	assert.Equal(t, http.StatusOK, updateResp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(updateResp.Body).Decode(&got))
	assert.Equal(t, "Updated", got["name"])
	assert.Equal(t, "side_veg", got["role"])
}

func TestDeleteComponent_204(t *testing.T) {
	router, _ := setupComponentRouter(t)
	createBody := `{"name":"To Delete","role":"main","reference_portions":1}`
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(createBody)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))

	deleteResp := httptest.NewRecorder()
	router.ServeHTTP(deleteResp, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/components/%.0f", created["id"]), nil))
	assert.Equal(t, http.StatusNoContent, deleteResp.Code)
}

func TestDeleteComponent_404(t *testing.T) {
	router, _ := setupComponentRouter(t)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/components/999", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestListComponents_200(t *testing.T) {
	router, _ := setupComponentRouter(t)
	for _, name := range []string{"Pasta", "Salad", "Soup"} {
		body := fmt.Sprintf(`{"name":"%s","role":"main","reference_portions":1}`, name)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(body)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/components", nil))
	assert.Equal(t, http.StatusOK, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	items, _ := got["items"].([]any)
	assert.Len(t, items, 3)
	assert.Equal(t, float64(3), got["total"])
}

func TestListComponents_SearchFilter(t *testing.T) {
	router, _ := setupComponentRouter(t)
	for _, name := range []string{"Chicken Curry", "Tofu Stir Fry"} {
		body := fmt.Sprintf(`{"name":"%s","role":"main","reference_portions":1}`, name)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(body)))
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/components?search=chicken", nil))
	assert.Equal(t, http.StatusOK, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, float64(1), got["total"])
}

func TestListComponents_RoleFilter(t *testing.T) {
	router, _ := setupComponentRouter(t)
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(`{"name":"Main","role":"main","reference_portions":1}`)))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(`{"name":"Side","role":"side_veg","reference_portions":1}`)))

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/components?role=main", nil))
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, float64(1), got["total"])
}

func TestNutrition_200(t *testing.T) {
	router, iRepo := setupComponentRouter(t)
	// Chicken: 165 kcal, 31g protein per 100g
	ing := seedTestIngredient(t, iRepo, "Chicken", 165, 31)

	body := fmt.Sprintf(`{
		"name":"Simple Chicken",
		"role":"main",
		"reference_portions":2,
		"ingredients":[{"ingredient_id":%d,"amount":200,"unit":"g","grams":200}]
	}`, ing.ID)

	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))

	nutResp := httptest.NewRecorder()
	router.ServeHTTP(nutResp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/components/%.0f/nutrition", created["id"]), nil))
	assert.Equal(t, http.StatusOK, nutResp.Code)

	var nut map[string]any
	require.NoError(t, json.NewDecoder(nutResp.Body).Decode(&nut))
	// 200g chicken: 330 kcal total, per 2 portions = 165
	assert.Equal(t, 165.0, nut["kcal"])
	// 200g: 62g protein total, per 2 portions = 31
	assert.Equal(t, 31.0, nut["protein"])
}

func TestNutrition_404(t *testing.T) {
	router, _ := setupComponentRouter(t)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/components/999/nutrition", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func setupComponentRouterWithImages(t *testing.T) (http.Handler, *sqlite.IngredientRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	iRepo := sqlite.NewIngredientRepo(db)
	cRepo := sqlite.NewComponentRepo(db)
	cSvc := component.NewService(cRepo, iRepo, iRepo)

	store, err := imagestore.New(t.TempDir(), nil)
	require.NoError(t, err)

	h := handlers.NewComponentHandler(cSvc, store)
	r := chi.NewRouter()
	r.Route("/api/components", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Post("/image", h.Upload)
			r.Delete("/image", h.DeleteImage)
		})
	})
	return r, iRepo
}

func TestComponentImageUpload(t *testing.T) {
	router, _ := setupComponentRouterWithImages(t)

	// Create a component first.
	createBody := `{"name":"Image Test","role":"main","reference_portions":1}`
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(createBody)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))

	// Build multipart request.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("image", "test.jpg")
	require.NoError(t, err)
	_, _ = part.Write(testImagePNG(t))
	require.NoError(t, w.Close())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/components/%.0f/image", created["id"]), &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Contains(t, got["image_path"], "components/")
}

func TestComponentImageDelete(t *testing.T) {
	router, _ := setupComponentRouterWithImages(t)

	// Create and upload.
	createBody := `{"name":"Delete Image Test","role":"main","reference_portions":1}`
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/components", bytes.NewBufferString(createBody)))
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	url := fmt.Sprintf("/api/components/%.0f/image", created["id"])

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("image", "test.jpg")
	require.NoError(t, err)
	_, _ = part.Write(testImagePNG(t))
	require.NoError(t, w.Close())

	uploadResp := httptest.NewRecorder()
	uploadReq := httptest.NewRequest(http.MethodPost, url, &buf)
	uploadReq.Header.Set("Content-Type", w.FormDataContentType())
	router.ServeHTTP(uploadResp, uploadReq)
	require.Equal(t, http.StatusOK, uploadResp.Code)

	// Delete the image.
	deleteResp := httptest.NewRecorder()
	router.ServeHTTP(deleteResp, httptest.NewRequest(http.MethodDelete, url, nil))
	assert.Equal(t, http.StatusNoContent, deleteResp.Code)

	// Verify image_path is nil.
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/components/%.0f", created["id"]), nil))
	var got map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	assert.Nil(t, got["image_path"])
}
