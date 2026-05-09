package handlers_test

import (
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
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// setupFoodMacrosRouter wires the BatchMacros handler with a real food repo
// so we exercise the per-portion / per-100g resolver via HTTP.
func setupFoodMacrosRouter(t *testing.T) (http.Handler, *sqlite.FoodRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	svc := food.NewService(repo)
	resolver := food.NewNutritionResolver(repo)
	h := handlers.NewFoodHandler(svc, resolver, nil)
	r := chi.NewRouter()
	// Register `/macros` ahead of `/{id}` to mirror production wiring.
	r.Get("/api/foods/macros", h.BatchMacros)
	return r, repo
}

func TestFoodMacros_LeafPer100g(t *testing.T) {
	r, repo := setupFoodMacrosRouter(t)
	ctx := context.Background()

	src := food.SourceManual
	rice := &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(130)}
	require.NoError(t, repo.Create(ctx, rice))

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/foods/macros?ids=%d", rice.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	foods := body["foods"].([]any)
	require.Len(t, foods, 1)
	first := foods[0].(map[string]any)
	assert.Equal(t, float64(rice.ID), first["food_id"].(float64))
	macros := first["macros"].(map[string]any)
	// Leaf per-100g: kcal stays 130.
	assert.InDelta(t, 130.0, macros["kcal"].(float64), 0.5)
}

func TestFoodMacros_ComposedPerPortion(t *testing.T) {
	r, repo := setupFoodMacrosRouter(t)
	ctx := context.Background()

	src := food.SourceManual
	rice := &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(130)}
	require.NoError(t, repo.Create(ctx, rice))

	role := food.RoleMain
	ref := float64(2) // 2 portions => per-portion macros = total / 2
	curry := &food.Food{
		Name: "Curry", Kind: food.KindComposed, Role: &role, ReferencePortions: &ref,
		Children: []food.FoodComponent{{ChildID: rice.ID, Amount: 200, Unit: "g", Grams: 200}},
	}
	require.NoError(t, repo.Create(ctx, curry))

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/foods/macros?ids=%d", curry.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	foods := body["foods"].([]any)
	require.Len(t, foods, 1)
	macros := foods[0].(map[string]any)["macros"].(map[string]any)
	// 200 g rice × 1.30 kcal/g = 260 kcal total / 2 portions = 130 kcal/portion.
	assert.InDelta(t, 130.0, macros["kcal"].(float64), 0.5)
}

func TestFoodMacros_OrderPreserved(t *testing.T) {
	r, repo := setupFoodMacrosRouter(t)
	ctx := context.Background()

	src := food.SourceManual
	a := &food.Food{Name: "A", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(100)}
	b := &food.Food{Name: "B", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(200)}
	c := &food.Food{Name: "C", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(300)}
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))
	require.NoError(t, repo.Create(ctx, c))

	url := fmt.Sprintf("/api/foods/macros?ids=%d,%d,%d", c.ID, a.ID, b.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	foods := body["foods"].([]any)
	require.Len(t, foods, 3)
	assert.Equal(t, float64(c.ID), foods[0].(map[string]any)["food_id"].(float64))
	assert.Equal(t, float64(a.ID), foods[1].(map[string]any)["food_id"].(float64))
	assert.Equal(t, float64(b.ID), foods[2].(map[string]any)["food_id"].(float64))
}

func TestFoodMacros_UnknownIDOmitted(t *testing.T) {
	r, repo := setupFoodMacrosRouter(t)
	ctx := context.Background()

	src := food.SourceManual
	a := &food.Food{Name: "A", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(100)}
	require.NoError(t, repo.Create(ctx, a))

	// 999999 doesn't exist; should be silently dropped.
	url := fmt.Sprintf("/api/foods/macros?ids=999999,%d", a.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	foods := body["foods"].([]any)
	require.Len(t, foods, 1)
	assert.Equal(t, float64(a.ID), foods[0].(map[string]any)["food_id"].(float64))
}

func TestFoodMacros_DeduplicatesIDs(t *testing.T) {
	r, repo := setupFoodMacrosRouter(t)
	ctx := context.Background()

	src := food.SourceManual
	a := &food.Food{Name: "A", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(100)}
	require.NoError(t, repo.Create(ctx, a))

	url := fmt.Sprintf("/api/foods/macros?ids=%d,%d,%d", a.ID, a.ID, a.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	foods := body["foods"].([]any)
	require.Len(t, foods, 1)
}

func TestFoodMacros_EmptyQuery_400(t *testing.T) {
	r, _ := setupFoodMacrosRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/foods/macros?ids=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestFoodMacros_MissingQuery_400(t *testing.T) {
	r, _ := setupFoodMacrosRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/foods/macros", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestFoodMacros_NonNumeric_400(t *testing.T) {
	r, _ := setupFoodMacrosRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/foods/macros?ids=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestFoodMacros_AllUnknown_EmptyArray(t *testing.T) {
	r, _ := setupFoodMacrosRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/foods/macros?ids=999998,999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	foods := body["foods"].([]any)
	assert.Empty(t, foods)
}
