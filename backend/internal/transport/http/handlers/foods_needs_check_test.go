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

func TestFoodHandler_NeedsCheck_ListsLegacyLinesAndClearsOnFix(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	h := handlers.NewFoodHandler(food.NewService(repo), food.NewNutritionResolver(repo), nil)
	r := chi.NewRouter()
	r.Route("/api/foods", func(r chi.Router) {
		r.Get("/needs-check", h.NeedsCheck)
		r.Post("/", h.Create)
		r.Put("/{id}", h.Update)
		r.Post("/{id}/fix-nutrition", h.FixNutrition)
	})

	mkLeaf := func(name string, kcal *float64) int64 {
		body := map[string]any{"name": name, "kind": "leaf"}
		if kcal != nil {
			body["kcal_100g"], body["protein_100g"], body["fat_100g"], body["carbs_100g"] = *kcal, 1.0, 1.0, 1.0
		}
		return create(t, r, body)
	}
	k := 100.0
	ok := mkLeaf("Reis", &k)
	bare := mkLeaf("Mystery", nil)
	salt := mkLeaf("Salz", nil)
	mkRecipe := func(name string, kids ...map[string]any) int64 {
		return create(t, r, map[string]any{"name": name, "kind": "composed", "role": "main", "reference_portions": 2, "children": kids})
	}
	line := func(id int64, toTaste bool) map[string]any {
		return map[string]any{"child_id": id, "amount": 100, "unit": "g", "grams": 100, "to_taste": toTaste}
	}
	legacy := mkRecipe("Legacy", line(ok, false), line(bare, false), line(salt, true))
	_ = mkRecipe("Fine", line(ok, false), line(salt, true))
	_ = mkRecipe("Also legacy", line(bare, false))

	get := func() map[string]any {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/foods/needs-check", nil))
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		var got map[string]any
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &got))
		return got
	}

	got := get()
	assert.EqualValues(t, 2, got["count"])
	recipes := got["recipes"].([]any)
	require.Len(t, recipes, 2)
	first := recipes[1].(map[string]any)
	assert.EqualValues(t, legacy, first["id"])
	lines := first["lines"].([]any)
	require.Len(t, lines, 1, "only the nutritionless, not-to-taste line is a problem")
	assert.Equal(t, "Mystery", lines[0].(map[string]any)["leaf_name"])
	leaves := got["leaves"].([]any)
	require.Len(t, leaves, 1)
	assert.Len(t, leaves[0].(map[string]any)["recipes"], 2)

	// Fixing the leaf's nutrition clears every recipe that used it.
	body, _ := json.Marshal(map[string]any{
		"name": "Mystery", "kind": "leaf",
		"kcal_100g": 50, "protein_100g": 1, "fat_100g": 1, "carbs_100g": 1,
	})
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%d", bare), bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	got = get()
	assert.EqualValues(t, 0, got["count"])
	assert.Empty(t, got["recipes"])
	assert.Empty(t, got["leaves"])
}

func create(t *testing.T, r http.Handler, body map[string]any) int64 {
	t.Helper()
	b, _ := json.Marshal(body)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewReader(b)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return int64(got["id"].(float64))
}

func TestFoodHandler_NeedsCheck_ArchivedRecipesAreNotListed(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	h := handlers.NewFoodHandler(food.NewService(repo), food.NewNutritionResolver(repo), nil)
	r := chi.NewRouter()
	r.Route("/api/foods", func(r chi.Router) {
		r.Get("/needs-check", h.NeedsCheck)
		r.Post("/", h.Create)
	})
	bare := create(t, r, map[string]any{"name": "Mystery", "kind": "leaf"})
	rec := create(t, r, map[string]any{
		"name": "Old", "kind": "composed", "role": "main", "reference_portions": 2,
		"children": []map[string]any{{"child_id": bare, "amount": 100, "unit": "g", "grams": 100}},
	})
	_, err := db.Exec("UPDATE foods SET archived = 1 WHERE id = ?", rec)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/foods/needs-check", nil))
	var got map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &got))
	assert.EqualValues(t, 0, got["count"])
	assert.Empty(t, got["leaves"], "a leaf used only by archived recipes is not listed")
}

func TestFoodHandler_FixNutrition_KeepsKnownValuesRoleAndRefusesPartialMacros(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	svc := food.NewService(repo)
	h := handlers.NewFoodHandler(svc, food.NewNutritionResolver(repo), nil)
	r := chi.NewRouter()
	r.Route("/api/foods", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Put("/{id}", h.Update)
		r.Post("/{id}/fix-nutrition", h.FixNutrition)
	})
	// A legacy leaf: fiber known, macros missing, role overridden by the user.
	id := create(t, r, map[string]any{"name": "Mystery", "kind": "leaf", "role": "sauce", "fiber_100g": 2.5})
	send := func(method, path string, body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(method, path, bytes.NewReader(b)))
		return resp
	}
	url := fmt.Sprintf("/api/foods/%d/fix-nutrition", id)

	partial := send(http.MethodPost, url, map[string]any{
		"kind": "leaf", "name": "Other", "source": "fdc", "fdc_id": "9",
		"kcal_100g": 80, "protein_100g": nil, "fat_100g": 1, "carbs_100g": 10,
	})
	assert.Equal(t, http.StatusUnprocessableEntity, partial.Code, partial.Body.String())
	f, err := svc.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Nil(t, f.Kcal100g, "a refused fix changes nothing")
	assert.Nil(t, f.FdcID)

	ok := send(http.MethodPost, url, map[string]any{
		"kind": "leaf", "name": "Other", "source": "fdc", "fdc_id": "9",
		"kcal_100g": 80, "protein_100g": 4, "fat_100g": 1, "carbs_100g": 10,
	})
	require.Equal(t, http.StatusNoContent, ok.Code, ok.Body.String())
	f, err = svc.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "Mystery", f.Name)
	require.NotNil(t, f.Role)
	assert.Equal(t, food.RoleSauce, *f.Role, "the user's role override survives")
	require.NotNil(t, f.Fiber100g)
	assert.InDelta(t, 2.5, *f.Fiber100g, 1e-9, "a nutrient the candidate lacks is kept")
	require.NotNil(t, f.FdcID)
	assert.Equal(t, "9", *f.FdcID)
	require.NotNil(t, f.Kcal100g)
	assert.InDelta(t, 80, *f.Kcal100g, 1e-9)
}

func TestFoodHandler_Update_KeepsRoleWhenRequestHasNone(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	svc := food.NewService(repo)
	h := handlers.NewFoodHandler(svc, food.NewNutritionResolver(repo), nil)
	r := chi.NewRouter()
	r.Route("/api/foods", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Put("/{id}", h.Update)
	})
	id := create(t, r, map[string]any{"name": "Mystery", "kind": "leaf", "role": "sauce"})
	body, _ := json.Marshal(map[string]any{"name": "Mystery", "kind": "leaf", "kcal_100g": 50, "protein_100g": 1, "fat_100g": 1, "carbs_100g": 1})
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%d", id), bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	f, err := svc.Get(t.Context(), id)
	require.NoError(t, err)
	require.NotNil(t, f.Role)
	assert.Equal(t, food.RoleSauce, *f.Role)
}
