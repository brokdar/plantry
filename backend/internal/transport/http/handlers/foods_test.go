package handlers_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func setupFoodRouter(t *testing.T) http.Handler {
	t.Helper()
	return setupFoodRouterWithStore(t, nil)
}

func setupFoodRouterWithStore(t *testing.T, store *imagestore.Store) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	svc := food.NewService(repo)
	resolver := food.NewNutritionResolver(repo)
	h := handlers.NewFoodHandler(svc, resolver, store)
	r := chi.NewRouter()
	r.Route("/api/foods", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Get("/scaled", h.Scaled)
			r.Put("/", h.Update)
			r.Delete("/", h.Delete)
			r.Post("/favorite", h.SetFavorite)
			r.Put("/shop-section", h.SetShopSection)
			r.Put("/diet-class", h.SetDietClass)
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
	existingID := createLeaf(t, r, "Rice")

	body := `{"name":"Rice","kind":"leaf"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusConflict, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "error.food.duplicate_name", got["message_key"])
	assert.Equal(t, float64(existingID), got["existing_id"])
}

func TestFoodHandler_Create_SourceURLAndDescription(t *testing.T) {
	r := setupFoodRouter(t)
	basilID := createLeaf(t, r, "Basil")
	body := fmt.Sprintf(`{"name":"Pesto","kind":"composed","role":"sauce","reference_portions":4,`+
		`"children":[{"child_id":%d,"amount":50,"unit":"g"}],`+
		`"source_url":"https://example.com/pesto","description":"Basil sauce."}`, basilID)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "https://example.com/pesto", got["source_url"])
	assert.Equal(t, "Basil sauce.", got["description"])
	assert.NotContains(t, got, "warnings")
}

func TestFoodHandler_Update_PreservesSourceURLWhenOmitted(t *testing.T) {
	r := setupFoodRouter(t)
	body := `{"name":"Apple","kind":"leaf","source_url":"https://example.com/apple","description":"Crisp."}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	id := int64(created["id"].(float64))

	// An editor that doesn't know the new fields must not wipe them.
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%d", id),
		bytes.NewBufferString(`{"name":"Apple","kind":"leaf","kcal_100g":52}`)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "https://example.com/apple", got["source_url"])
	assert.Equal(t, "Crisp.", got["description"])

	// An explicit value replaces it.
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%d", id),
		bytes.NewBufferString(`{"name":"Apple","kind":"leaf","description":"Sweet."}`)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var updated map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	assert.Equal(t, "Sweet.", updated["description"])
	assert.Equal(t, "https://example.com/apple", updated["source_url"])
}

func TestFoodHandler_Update_EmptyStringClearsToNull(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	h := handlers.NewFoodHandler(food.NewService(repo), food.NewNutritionResolver(repo), nil)
	r := chi.NewRouter()
	r.Post("/api/foods", h.Create)
	r.Put("/api/foods/{id}", h.Update)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods",
		bytes.NewBufferString(`{"name":"Apple","kind":"leaf","source_url":"https://example.com/apple","description":"Crisp."}`)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	id := int64(created["id"].(float64))

	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%d", id),
		bytes.NewBufferString(`{"name":"Apple","kind":"leaf","description":""}`)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotContains(t, got, "description")
	assert.Equal(t, "https://example.com/apple", got["source_url"])

	var desc sql.NullString
	require.NoError(t, db.QueryRow(`SELECT description FROM foods WHERE id = ?`, id).Scan(&desc))
	assert.False(t, desc.Valid, "empty description must be stored as NULL")
}

func smallPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := range 4 {
		for y := range 4 {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestFoodHandler_Create_ImageURL_Downloads(t *testing.T) {
	data := smallPNG(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	store, err := imagestore.New(t.TempDir(), srv.Client())
	require.NoError(t, err)
	r := setupFoodRouterWithStore(t, store)

	body := fmt.Sprintf(`{"name":"Tomato","kind":"leaf","image_url":%q}`, srv.URL+"/tomato.png")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	id := int64(got["id"].(float64))
	assert.Equal(t, fmt.Sprintf("foods/%d.jpg", id), got["image_path"])
	assert.NotContains(t, got, "warnings")

	// Persisted, not just echoed.
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/foods/%d", id), nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var reloaded map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&reloaded))
	assert.Equal(t, got["image_path"], reloaded["image_path"])
}

func TestFoodHandler_Create_ImageURL_DownloadFailsStillCreates(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	store, err := imagestore.New(t.TempDir(), srv.Client())
	require.NoError(t, err)
	r := setupFoodRouterWithStore(t, store)

	body := fmt.Sprintf(`{"name":"Tomato","kind":"leaf","image_url":%q}`, srv.URL+"/missing.png")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotZero(t, got["id"])
	assert.NotContains(t, got, "image_path")
	assert.Equal(t, []any{"image_download_failed"}, got["warnings"])
}

func TestFoodHandler_Create_ImageURL_NoStoreWarns(t *testing.T) {
	r := setupFoodRouter(t) // nil image store

	body := `{"name":"Tomato","kind":"leaf","image_url":"http://127.0.0.1:1/tomato.png"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotContains(t, got, "image_path")
	assert.Equal(t, []any{"image_download_failed"}, got["warnings"])
}

func TestFoodHandler_Create_EmptyImageURLIgnored(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	store, err := imagestore.New(t.TempDir(), srv.Client())
	require.NoError(t, err)
	r := setupFoodRouterWithStore(t, store)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods",
		bytes.NewBufferString(`{"name":"Tomato","kind":"leaf","image_url":""}`)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotContains(t, got, "warnings")
	assert.NotContains(t, got, "image_path")
	assert.Zero(t, hits.Load(), "no fetch for an empty image_url")
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

// ── Nutrition ─────────────────────────────────────────────────────────────────

func TestFoodHandler_Nutrition_CompletenessFields(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	h := handlers.NewFoodHandler(food.NewService(repo), food.NewNutritionResolver(repo), nil)
	r := chi.NewRouter()
	r.Get("/api/foods/{id}/nutrition", h.Nutrition)
	ctx := t.Context()

	src := food.SourceManual
	rice := &food.Food{
		Name: "Rice", Kind: food.KindLeaf, Source: &src,
		Kcal100g: ptrFloat64(130), Protein100g: ptrFloat64(2.7), Fat100g: ptrFloat64(0.3), Carbs100g: ptrFloat64(28),
	}
	require.NoError(t, repo.Create(ctx, rice))
	spice := &food.Food{Name: "Mystery Spice", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(300)}
	require.NoError(t, repo.Create(ctx, spice))

	role := food.RoleMain
	complete := &food.Food{
		Name: "Plain Rice", Kind: food.KindComposed, Role: &role, ReferencePortions: ptrFloat64(1),
		Children: []food.FoodComponent{{ChildID: rice.ID, Amount: 100, Unit: "g", Grams: 100}},
	}
	require.NoError(t, repo.Create(ctx, complete))
	incomplete := &food.Food{
		Name: "Spiced Rice", Kind: food.KindComposed, Role: &role, ReferencePortions: ptrFloat64(1),
		Children: []food.FoodComponent{
			{ChildID: rice.ID, Amount: 100, Unit: "g", Grams: 100},
			{ChildID: spice.ID, Amount: 5, Unit: "g", Grams: 5, SortOrder: 1},
		},
	}
	require.NoError(t, repo.Create(ctx, incomplete))

	get := func(t *testing.T, id int64) string {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/foods/%d/nutrition", id), nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return w.Body.String()
	}

	t.Run("complete recipe serializes empty missing list", func(t *testing.T) {
		body := get(t, complete.ID)
		assert.Contains(t, body, `"complete":true`)
		assert.Contains(t, body, `"missing":[]`)
	})

	t.Run("incomplete recipe lists the leaf id and name", func(t *testing.T) {
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(get(t, incomplete.ID)), &got))
		assert.Equal(t, false, got["complete"])
		assert.Equal(t, []any{map[string]any{"id": float64(spice.ID), "name": "Mystery Spice"}}, got["missing"])
		assert.InDelta(t, 145.0, got["kcal"].(float64), 0.01)
	})
}
