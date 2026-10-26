package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// memFoodRepo is a minimal in-memory food.Repository covering the methods the
// food tools and the nutrition resolver call. Unused methods panic via the
// embedded nil interface.
type memFoodRepo struct {
	food.Repository
	foods    map[int64]*food.Food
	children map[int64][]food.FoodComponent
}

func (r *memFoodRepo) Get(_ context.Context, id int64) (*food.Food, error) {
	f, ok := r.foods[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *f
	cp.Children = r.children[id]
	return &cp, nil
}

func (r *memFoodRepo) List(_ context.Context, _ food.ListQuery) (*food.ListResult, error) {
	items := make([]food.Food, 0, len(r.foods))
	for _, id := range []int64{1, 2, 3, 4} {
		if f, ok := r.foods[id]; ok {
			items = append(items, *f)
		}
	}
	return &food.ListResult{Items: items, Total: len(items)}, nil
}

func (r *memFoodRepo) LookupByIDs(_ context.Context, ids []int64) (map[int64]*food.Food, error) {
	out := map[int64]*food.Food{}
	for _, id := range ids {
		if id == 4 {
			return nil, errors.New("boom")
		}
		if f, ok := r.foods[id]; ok {
			cp := *f
			out[id] = &cp
		}
	}
	return out, nil
}

func (r *memFoodRepo) ListChildren(_ context.Context, parentID int64) ([]food.FoodComponent, error) {
	return r.children[parentID], nil
}

func ptr[T any](v T) *T { return &v }

// newPayloadToolSet seeds: rice (complete leaf), mystery sauce (leaf without
// nutrition), curry (favorite composed food of both, 2 portions) and a food
// whose nutrition lookup fails.
func newPayloadToolSet(t *testing.T) *ToolSet {
	t.Helper()
	cooked := time.Date(2026, 10, 20, 18, 30, 0, 0, time.UTC)
	repo := &memFoodRepo{
		foods: map[int64]*food.Food{
			1: {
				ID: 1, Name: "Rice", Kind: food.KindLeaf,
				Kcal100g: ptr(130.04), Protein100g: ptr(2.73), Fat100g: ptr(0.3), Carbs100g: ptr(28.06),
			},
			2: {ID: 2, Name: "Mystery sauce", Kind: food.KindLeaf},
			3: {
				ID: 3, Name: "Curry", Kind: food.KindComposed, ReferencePortions: ptr(2.0),
				Favorite: true, CookCount: 3, LastCookedAt: &cooked,
			},
			4: {ID: 4, Name: "Broken", Kind: food.KindLeaf},
		},
		children: map[int64][]food.FoodComponent{
			3: {
				{ParentID: 3, ChildID: 1, ChildName: "Rice", ChildKind: food.KindLeaf, Amount: 200, Unit: "g", Grams: 200},
				{ParentID: 3, ChildID: 2, ChildName: "Mystery sauce", ChildKind: food.KindLeaf, Amount: 100, Unit: "g", Grams: 100},
			},
		},
	}
	ts, err := NewToolSet(Services{
		Foods:             food.NewService(repo),
		NutritionResolver: food.NewNutritionResolver(repo),
	})
	require.NoError(t, err)
	return ts
}

func execJSON(t *testing.T, ts *ToolSet, name, input string) map[string]any {
	t.Helper()
	out, _, err := ts.Execute(context.Background(), name, json.RawMessage(input))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	return m
}

func assertCurryPayload(t *testing.T, m map[string]any) {
	t.Helper()
	assert.Equal(t, true, m["favorite"])
	assert.Equal(t, float64(3), m["cook_count"])
	assert.Equal(t, "2026-10-20", m["last_cooked_at"])
	assert.Equal(t, false, m["nutrition_complete"])
	assert.Equal(t, []any{"Mystery sauce"}, m["missing_nutrition"])
	// 200 g rice / 2 portions = 100 g rice per portion.
	assert.Equal(t, map[string]any{
		"kcal": 130.0, "protein": 2.7, "fat": 0.3, "carbs": 28.1,
	}, m["per_portion"])
}

func TestTool_GetFood_PayloadIncludesFavoriteCookDataAndMacros(t *testing.T) {
	ts := newPayloadToolSet(t)
	assertCurryPayload(t, execJSON(t, ts, "get_food", `{"food_id":3}`))
}

func TestTool_ListFoods_PayloadIncludesFavoriteCookDataAndMacros(t *testing.T) {
	ts := newPayloadToolSet(t)
	m := execJSON(t, ts, "list_foods", `{}`)
	items, ok := m["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 4)
	byName := map[string]map[string]any{}
	for _, it := range items {
		f := it.(map[string]any)
		byName[f["name"].(string)] = f
	}

	assertCurryPayload(t, byName["Curry"])

	rice := byName["Rice"]
	assert.Equal(t, false, rice["favorite"])
	assert.Equal(t, float64(0), rice["cook_count"])
	assert.Contains(t, rice, "last_cooked_at")
	assert.Nil(t, rice["last_cooked_at"])
	assert.Equal(t, true, rice["nutrition_complete"])
	assert.NotContains(t, rice, "missing_nutrition")
	assert.Equal(t, map[string]any{
		"kcal": 130.0, "protein": 2.7, "fat": 0.3, "carbs": 28.1,
	}, rice["per_portion"])

	// A failing nutrition lookup drops the macro fields for that item only.
	broken := byName["Broken"]
	assert.NotContains(t, broken, "per_portion")
	assert.NotContains(t, broken, "nutrition_complete")
	assert.Equal(t, false, broken["favorite"])
}

func TestPlateSummary_IncludesSkipped(t *testing.T) {
	p := &plate.Plate{ID: 7, Date: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), SlotID: 1, Skipped: true}
	var m map[string]any
	require.NoError(t, json.Unmarshal(mustJSON(plateSummary(p)), &m))
	assert.Equal(t, true, m["skipped"])
}
