package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/recipechange"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

type recipeToolEnv struct {
	db    *sql.DB
	ts    *agent.ToolSet
	foods *sqlite.FoodRepo
	curry *food.Food
	attrs *flavor.Service
}

func newRecipeToolEnv(t *testing.T) *recipeToolEnv {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	foods := sqlite.NewFoodRepo(db)
	leaf := func(name string, kcal float64) *food.Food {
		src := food.SourceManual
		p, f, c := 5.0, 5.0, 5.0
		lf := &food.Food{Name: name, Kind: food.KindLeaf, Source: &src, Kcal100g: &kcal, Protein100g: &p, Fat100g: &f, Carbs100g: &c}
		require.NoError(t, foods.Create(ctx, lf))
		return lf
	}
	chickpeas, tomato := leaf("Chickpeas", 120), leaf("Tomato", 18)
	leaf("Chili flakes", 280)
	ref := 4.0
	curry := &food.Food{Name: "Curry", Kind: food.KindComposed, ReferencePortions: &ref, Children: []food.FoodComponent{
		{ChildID: chickpeas.ID, Amount: 400, Unit: "g", Grams: 400},
		{ChildID: tomato.ID, Amount: 300, Unit: "g", Grams: 300, SortOrder: 1},
	}, Instructions: []food.Instruction{{StepNumber: 1, Text: "Simmer."}}}
	require.NoError(t, foods.Create(ctx, curry))

	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, nil)
	rc := recipechange.NewService(foods, sqlite.NewFoodOriginRepo(db), food.NewMatcher(foods, nil, nil),
		sqlite.NewRecipeChangeRepo(db), profile.NewService(sqlite.NewProfileRepo(db)), events)
	rc.Now = func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) }
	restorers[foodevent.KindRecipeContent] = rc
	attrs := flavor.NewService(sqlite.NewFoodAttributeRepo(db), foods, food.NewNutritionResolver(foods), events)
	ts, err := agent.NewToolSet(agent.Services{Recipes: agent.RecipeTools{Changes: rc, Attributes: attrs}})
	require.NoError(t, err)
	return &recipeToolEnv{db: db, ts: ts, foods: foods, curry: curry, attrs: attrs}
}

func (e *recipeToolEnv) call(t *testing.T, name string, input map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(input)
	require.NoError(t, err)
	out, _, _, err := e.ts.Execute(context.Background(), name, b)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	return got
}

func recipe(lines ...string) map[string]any {
	ings := make([]any, len(lines))
	for i, l := range lines {
		ings[i] = l
	}
	return map[string]any{"name": "Spicy curry", "ingredients": ings, "steps": []any{"Simmer."}, "changes": []any{"Added chili flakes for heat."}}
}

func TestRecipeTools_CreateVariantSavesAndBlockRefuses(t *testing.T) {
	t.Parallel()
	e := newRecipeToolEnv(t)
	got := e.call(t, "create_variant", map[string]any{"recipe_id": e.curry.ID, "recipe": recipe("400 g chickpeas", "300 g tomato", "1 tsp chili flakes")})
	require.Equal(t, true, got["saved"], got)
	v, err := e.foods.Get(context.Background(), int64(got["recipe_id"].(float64)))
	require.NoError(t, err)
	assert.Len(t, v.Children, 3)
	assert.NotNil(t, got["undo"])

	got = e.call(t, "create_variant", map[string]any{"recipe_id": e.curry.ID, "recipe": recipe("400 g chickpeas", "2 tbsp dragon salt")})
	assert.Equal(t, false, got["saved"], "an unmatched line blocks: %v", got)
	assert.NotEmpty(t, got["check"].(map[string]any)["blocks"])
}

func TestRecipeTools_EditAsksBeforeConfirmedPlates(t *testing.T) {
	t.Parallel()
	e := newRecipeToolEnv(t)
	_, err := e.db.Exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('dinner', 'moon', 0, 1)`)
	require.NoError(t, err)
	_, err = e.db.Exec(`INSERT INTO plates (date, slot_id, state) VALUES ('2026-11-03', (SELECT id FROM time_slots), 'confirmed')`)
	require.NoError(t, err)
	_, err = e.db.Exec(`INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions) VALUES (?, '2026-11-03', (SELECT id FROM time_slots), 1)`, e.curry.ID)
	require.NoError(t, err)
	_, err = e.db.Exec(`INSERT INTO plate_components (plate_id, food_id, portions, cook_session_id, sort_order) VALUES ((SELECT id FROM plates), ?, 1, (SELECT id FROM cook_sessions), 0)`, e.curry.ID)
	require.NoError(t, err)

	in := map[string]any{"recipe_id": e.curry.ID, "recipe": recipe("400 g chickpeas", "400 g tomato")}
	got := e.call(t, "edit_recipe", in)
	assert.Equal(t, false, got["saved"])
	assert.Equal(t, true, got["needs_confirmation"])
	assert.Equal(t, []any{"2026-11-03"}, got["confirmed_dates"])
	assert.NotNil(t, got["before"])

	in["confirmed"] = true
	got = e.call(t, "edit_recipe", in)
	require.Equal(t, true, got["saved"], got)
	c, err := e.foods.Get(context.Background(), e.curry.ID)
	require.NoError(t, err)
	assert.InDelta(t, 400, c.Children[1].Grams, 0.01)
}

func TestRecipeTools_SetAttributesKeepsUserValues(t *testing.T) {
	t.Parallel()
	e := newRecipeToolEnv(t)
	ctx := context.Background()
	_, _, err := e.attrs.Set(ctx, e.curry.ID, flavor.Patch{Set: flavor.Attributes{Cuisine: "indian"}, Fields: []flavor.Field{flavor.FieldCuisine}})
	require.NoError(t, err)

	got := e.call(t, "set_recipe_attributes", map[string]any{"recipe_id": e.curry.ID, "attributes": map[string]any{"cuisine": "thai", "saucy": true}})
	assert.Equal(t, []any{"cuisine"}, got["kept"])
	v, err := e.attrs.Get(ctx, e.curry.ID)
	require.NoError(t, err)
	b, _ := json.Marshal(v)
	assert.Contains(t, string(b), `"indian"`, "the user's cuisine stays")
	assert.NotContains(t, string(b), `"thai"`)
}

func TestRecipeTools_NotRegisteredWithoutServices(t *testing.T) {
	t.Parallel()
	ts, err := agent.NewToolSet(agent.Services{})
	require.NoError(t, err)
	for _, tl := range ts.Describe() {
		assert.NotContains(t, []string{"create_variant", "edit_recipe", "set_recipe_attributes"}, tl.Name)
	}
}

func TestPrompt_RecipeRuleFitsTheBudget(t *testing.T) {
	t.Parallel()
	prompt, _ := agent.ComposePrompt(nil, "", "", time.Time{})
	assert.Contains(t, prompt, "create_variant; edit_recipe only when the user explicitly asks")
	assert.Less(t, len(prompt), 24_000)
}
