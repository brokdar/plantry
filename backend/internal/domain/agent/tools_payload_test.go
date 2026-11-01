package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// memFoodRepo is a minimal in-memory food.Repository covering the methods the
// food tools and the nutrition resolver call. Unused methods panic via the
// embedded nil interface.
type memFoodRepo struct {
	food.Repository
	foods    map[int64]*food.Food
	children map[int64][]food.FoodComponent
	history  map[int64]food.History
	// historyToday records the date History was last asked for.
	historyToday time.Time
}

func (r *memFoodRepo) History(_ context.Context, _ []int64, today time.Time) (map[int64]food.History, error) {
	r.historyToday = today
	return r.history, nil
}

func (r *memFoodRepo) UsedInCounts(_ context.Context, _ []int64) (map[int64]int, error) {
	return nil, nil
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

func (r *memFoodRepo) LookupByIDs(ctx context.Context, ids []int64) (map[int64]*food.Food, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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

func (r *memFoodRepo) ListChildrenByParents(_ context.Context, ids []int64) (map[int64][]food.FoodComponent, error) {
	out := map[int64][]food.FoodComponent{}
	for _, id := range ids {
		out[id] = r.children[id]
	}
	return out, nil
}

func ptr[T any](v T) *T { return &v }

// keepDaysSetting serves plan.default_keep_days = raw.
type keepDaysSetting string

func (k keepDaysSetting) Get(_ context.Context, key string) (settings.Value, error) {
	if key != settings.KeyPlanDefaultKeepDays {
		return settings.Value{}, fmt.Errorf("unexpected setting %s", key)
	}
	return settings.Value{Key: key, Raw: string(k)}, nil
}

// newPayloadToolSet seeds: rice (complete leaf), mystery sauce (leaf without
// nutrition), curry (favorite composed food of both, 2 portions) and a food
// whose nutrition lookup fails.
func newPayloadToolSet(t *testing.T) (*ToolSet, *memFoodRepo) {
	t.Helper()
	eaten := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	repo := &memFoodRepo{
		foods: map[int64]*food.Food{
			1: {
				ID: 1, Name: "Rice", Kind: food.KindLeaf,
				Kcal100g: ptr(130.04), Protein100g: ptr(2.73), Fat100g: ptr(0.3), Carbs100g: ptr(28.06),
			},
			2: {ID: 2, Name: "Mystery sauce", Kind: food.KindLeaf},
			3: {
				ID: 3, Name: "Curry", Kind: food.KindComposed, ReferencePortions: ptr(2.0),
				Favorite: true, KeepDays: ptr(3), Freezable: ptr(true),
			},
			4: {ID: 4, Name: "Broken", Kind: food.KindLeaf},
		},
		history: map[int64]food.History{3: {LastEaten: &eaten, TimesCooked: 3}},
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
		Settings:          keepDaysSetting("4"),
	})
	require.NoError(t, err)
	return ts, repo
}

// payloadToday is the user's date the payload tests run the tools under.
var payloadToday = time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)

func execJSON(t *testing.T, ts *ToolSet, name, input string) map[string]any {
	t.Helper()
	out, _, err := ts.Execute(withToday(context.Background(), payloadToday), name, json.RawMessage(input))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	return m
}

func assertCurryPayload(t *testing.T, m map[string]any) {
	t.Helper()
	assert.Equal(t, true, m["favorite"])
	assert.Equal(t, "unknown", m["diet_class"], "the mystery sauce makes the recipe unknown")
	assert.Equal(t, float64(3), m["times_cooked"])
	assert.Equal(t, "2026-10-20", m["last_eaten"])
	assert.Equal(t, float64(3), m["keep_days"])
	assert.Equal(t, true, m["freezable"])
	assert.Equal(t, "per_portion", m["basis"])
	assert.Equal(t, false, m["nutrition_complete"])
	assert.Equal(t, []any{"Mystery sauce"}, m["missing_nutrition"])
	// 200 g rice / 2 portions = 100 g rice per portion.
	assert.Equal(t, map[string]any{
		"kcal": 130.0, "protein": 2.7, "fat": 0.3, "carbs": 28.1,
	}, m["per_portion"])
}

func TestTool_GetFood_PayloadIncludesFavoriteHistoryAndMacros(t *testing.T) {
	ts, repo := newPayloadToolSet(t)
	assertCurryPayload(t, execJSON(t, ts, "get_food", `{"food_id":3}`))
	assert.Equal(t, payloadToday, repo.historyToday, "history is as of the user's today")
}

// B2-AC6 (D10): keep_days is the effective value, the plan default when the
// recipe sets none.
func TestTool_Foods_KeepDaysIsEffective(t *testing.T) {
	ts, repo := newPayloadToolSet(t)
	repo.foods[3].KeepDays = nil
	assert.Equal(t, float64(4), execJSON(t, ts, "get_food", `{"food_id":3}`)["keep_days"])
	items := execJSON(t, ts, "list_foods", `{}`)["items"].([]any)
	for _, it := range items {
		f := it.(map[string]any)
		if f["name"] == "Curry" {
			assert.Equal(t, float64(4), f["keep_days"])
		}
	}
}

func TestTool_GetFood_NeverEatenRecipeHasNullHistory(t *testing.T) {
	ts, repo := newPayloadToolSet(t)
	repo.history = nil
	m := execJSON(t, ts, "get_food", `{"food_id":3}`)
	assert.Contains(t, m, "last_eaten")
	assert.Nil(t, m["last_eaten"])
	assert.Equal(t, float64(0), m["times_cooked"])
}

func TestTool_ListFoods_PayloadIncludesFavoriteHistoryAndMacros(t *testing.T) {
	ts, repo := newPayloadToolSet(t)
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
	assert.Contains(t, rice, "diet_class")
	assert.NotContains(t, rice, "last_eaten", "history is for recipes only")
	assert.NotContains(t, rice, "times_cooked")
	assert.Equal(t, payloadToday, repo.historyToday)
	assert.Equal(t, "per_100g", rice["basis"])
	assert.Equal(t, true, rice["nutrition_complete"])
	assert.NotContains(t, rice, "missing_nutrition")
	assert.Equal(t, map[string]any{
		"kcal": 130.0, "protein": 2.7, "fat": 0.3, "carbs": 28.1,
	}, rice["per_portion"])

	// A failing nutrition lookup drops the macro fields for that item only.
	broken := byName["Broken"]
	assert.NotContains(t, broken, "per_portion")
	assert.NotContains(t, broken, "nutrition_complete")
	assert.NotContains(t, broken, "basis")
	assert.Equal(t, false, broken["favorite"])
}

func TestPlateSummary_HasNoSkipped(t *testing.T) {
	p := &plate.Plate{ID: 7, Date: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), SlotID: 1}
	var m map[string]any
	require.NoError(t, json.Unmarshal(mustJSON(plateSummary(p)), &m))
	assert.NotContains(t, m, "skipped")
	assert.Equal(t, "2026-10-26", m["date"])
}

func TestErrorMessageKey_CellErrors(t *testing.T) {
	assert.Equal(t, "error.plate.cell_unavailable", errorMessageKey(fmt.Errorf("x: %w", domain.ErrCellUnavailable)))
	assert.Equal(t, "error.plate.cell_occupied", errorMessageKey(fmt.Errorf("x: %w", domain.ErrCellOccupied)))
}

func TestErrorMessageKey_UsesDomainKeys(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{domain.ErrSessionBeforeCook, "error.session.before_cook"},
		{domain.ErrSessionPastKeepDays, "error.session.past_keep_days"},
		{domain.ErrSessionPastFreezerDays, "error.session.past_freezer_days"},
		{domain.ErrSessionFoodMismatch, "error.session.food_mismatch"},
		{domain.ErrSessionFreezingDisabled, "error.session.freezing_disabled"},
		{domain.ErrSessionNotFreezable, "error.session.not_freezable"},
		{domain.ErrSessionNotFound, "error.session.not_found"},
		{domain.ErrInvalidPlanStart, "error.plan.invalid_start"},
		{fmt.Errorf("%w: %w", domain.ErrInvalidInput, plate.ErrInvalidQuantityShape), "error.plate.invalid_quantity_shape"},
		{ErrToolNotFound, "error.ai.tool_not_found"},
		{errors.New("boom"), "error.server"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, errorMessageKey(fmt.Errorf("tool: %w", tt.err)))
		})
	}
}

func TestPlateTools_DescriptionsDropSkipped(t *testing.T) {
	for _, tool := range defaultTools(Services{}) {
		assert.NotContains(t, strings.ToLower(tool.Description), "skipped=", tool.Name)
		assert.NotContains(t, strings.ToLower(tool.Description), "carries skipped", tool.Name)
	}
	assert.Contains(t, toolsByName()["get_plates_range"].Description, "availability")
}

// The Anthropic Messages API rejects an input_schema with a top-level
// anyOf, oneOf or allOf, and every registered tool is sent on every turn.
func TestTools_SchemasHaveNoTopLevelCombinators(t *testing.T) {
	for _, tool := range defaultTools(Services{Presets: &stubPresetSvc{}}) {
		var schema map[string]any
		require.NoError(t, json.Unmarshal(tool.Schema, &schema), tool.Name)
		for _, key := range []string{"anyOf", "oneOf", "allOf"} {
			assert.NotContains(t, schema, key, "%s: top-level %s", tool.Name, key)
		}
	}
}

func TestTool_ListFoods_CancelledContextAborts(t *testing.T) {
	ts, _ := newPayloadToolSet(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := ts.Execute(ctx, "list_foods", json.RawMessage(`{}`))
	require.ErrorIs(t, err, context.Canceled)
}
