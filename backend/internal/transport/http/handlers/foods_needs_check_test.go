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
