package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/recipechange"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// rcEnv is the shop environment plus the recipe change endpoints: a Curry
// recipe of matched leaves and a fake provider answering the ask tool.
type rcEnv struct {
	*subEnv
	curry *food.Food
	svc   *recipechange.Service
	prof  *profile.Service
}

func askTurn(input string) fake.Turn {
	return fake.Turn{
		ForTool: recipechange.AskTool, StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{
			Type: "tool_use", ToolUseID: "toolu_1", ToolName: recipechange.AskTool, ToolInput: json.RawMessage(input),
		}}},
	}
}

func newRCEnv(t *testing.T, turns ...fake.Turn) *rcEnv {
	t.Helper()
	ctx := context.Background()
	se := newSubEnv(t)
	leaf := func(name string, kcal float64) *food.Food {
		src := food.SourceManual
		p, f, c := 5.0, 5.0, 5.0
		lf := &food.Food{Name: name, Kind: food.KindLeaf, Source: &src, Kcal100g: &kcal, Protein100g: &p, Fat100g: &f, Carbs100g: &c}
		require.NoError(t, se.foods.Create(ctx, lf))
		return lf
	}
	chickpeas, tomato := leaf("Chickpeas", 120), leaf("Tomato", 18)
	leaf("Coconut yoghurt", 110)
	leaf("Chili flakes", 280)
	leaf("Walnuts", 650)
	ref := 4.0
	curry := &food.Food{Name: "Curry", Kind: food.KindComposed, ReferencePortions: &ref, Children: []food.FoodComponent{
		{ChildID: chickpeas.ID, Amount: 400, Unit: "g", Grams: 400},
		{ChildID: tomato.ID, Amount: 300, Unit: "g", Grams: 300, SortOrder: 1},
	}, Instructions: []food.Instruction{{StepNumber: 1, Text: "Simmer everything for 20 minutes."}}}
	require.NoError(t, se.foods.Create(ctx, curry))

	runs := sqlite.NewAIRunRepo(se.db)
	rec := airun.NewRecorder(runs, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	client := fake.NewFromScript(&fake.Script{Turns: turns})
	prof := profile.NewService(sqlite.NewProfileRepo(se.db))
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(se.db), sqlite.NewFoodEventRepo(se.db), restorers, nil)
	svc := recipechange.NewService(se.foods, sqlite.NewFoodOriginRepo(se.db), food.NewMatcher(se.foods, nil, nil),
		sqlite.NewRecipeChangeRepo(se.db), prof, events).WithModel(llm.StaticResolver(rec.Meter(client, "fake"), "fake-model"), rec)
	restorers[foodevent.KindRecipeContent] = svc
	now := day(t, "2026-11-01")
	svc.Now = func() time.Time { return now }

	h := handlers.NewRecipeChangeHandler(svc)
	r := chi.NewRouter()
	r.Post("/api/foods/{id}/ask", h.Ask)
	r.Post("/api/foods/{id}/edit", h.Edit)
	r.Get("/api/foods/{id}/check", h.Check)
	r.Get("/api/foods/{id}/changes", h.Changes)
	r.Post("/api/food-events/revert", handlers.NewFoodEventHandler(events).Revert)
	se.api = r
	return &rcEnv{subEnv: se, curry: curry, svc: svc, prof: prof}
}

func (e *rcEnv) path(id int64, rest string) string {
	return "/api/foods/" + strconv.FormatInt(id, 10) + rest
}

const spicyVariant = `{"kind":"variant","recipe":{"name":"Spicy curry with coconut yoghurt",
 "ingredients":["400 g chickpeas","300 g tomato","150 g coconut yoghurt","1 tsp chili flakes"],
 "steps":["Simmer everything for 20 minutes.","Stir in the coconut yoghurt."],
 "changes":["Added chili flakes for heat.","Coconut yoghurt cools and thickens."]}}`

func TestRecipeChange_VariantHasChangesListOnlyMatchedLinesAndUndo(t *testing.T) {
	t.Parallel()
	e := newRCEnv(t, askTurn(spicyVariant))
	ctx := context.Background()

	code, out := e.do(t, http.MethodPost, e.path(e.curry.ID, "/ask"), map[string]string{"request": "spicier, with coconut yoghurt"})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, "variant", out["kind"])
	v := out["variant"].(map[string]any)
	id := int64(v["food_id"].(float64))
	require.NotZero(t, id, "the variant is saved: %v", v)
	assert.Len(t, v["changes"], 2)

	got, err := e.foods.Get(ctx, id)
	require.NoError(t, err)
	base, err := e.foods.Get(ctx, e.curry.ID)
	require.NoError(t, err)
	require.NotNil(t, base.VariantGroupID)
	assert.Equal(t, base.VariantGroupID, got.VariantGroupID, "the variant is in the recipe's group")
	assert.False(t, got.Archived, "a filled variant is visible")
	require.Len(t, got.Children, 4)
	for _, c := range got.Children {
		leaf, err := e.foods.Get(ctx, c.ChildID)
		require.NoError(t, err)
		assert.NotNil(t, leaf.Kcal100g, "every line is a food with nutrition: %s", c.ChildName)
	}
	assert.Len(t, got.Instructions, 2)

	code, ch := e.do(t, http.MethodGet, e.path(id, "/changes"), nil)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []any{"Added chili flakes for heat.", "Coconut yoghurt cools and thickens."}, ch["items"])

	code, _ = e.do(t, http.MethodPost, "/api/food-events/revert", v["undo"])
	require.Equal(t, http.StatusOK, code)
	_, err = e.foods.Get(ctx, id)
	assert.ErrorIs(t, err, domain.ErrNotFound, "Undo removes the variant")
}

func TestRecipeChange_GateBlockSavesNothing(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		turn    string
		allergy bool
		code    string
	}{
		"unmatched line": {turn: `{"kind":"variant","recipe":{"name":"Curry X","ingredients":["400 g chickpeas","20 g unicornium powder"],"steps":["Simmer."],"changes":["Added unicornium."]}}`},
		"allergen":       {turn: `{"kind":"variant","recipe":{"name":"Nutty curry","ingredients":["400 g chickpeas","300 g tomato","50 g walnuts"],"steps":["Simmer."],"changes":["Added walnuts for crunch."]}}`, allergy: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRCEnv(t, askTurn(tc.turn))
			if tc.allergy {
				p, err := e.prof.Get(context.Background())
				require.NoError(t, err)
				p.DietaryRestrictions = []string{"nuts allergy"}
				_, err = e.prof.Update(context.Background(), p)
				require.NoError(t, err)
			}
			before, err := e.foods.List(context.Background(), food.ListQuery{Kind: food.KindComposed, Limit: 100})
			require.NoError(t, err)
			code, out := e.do(t, http.MethodPost, e.path(e.curry.ID, "/ask"), map[string]string{"request": "change it"})
			require.Equal(t, http.StatusOK, code, out)
			v := out["variant"].(map[string]any)
			assert.Zero(t, v["food_id"], "a block saves nothing")
			check := v["check"].(map[string]any)
			assert.NotEmpty(t, check["blocks"], "the check says why")
			after, err := e.foods.List(context.Background(), food.ListQuery{Kind: food.KindComposed, Limit: 100})
			require.NoError(t, err)
			assert.Equal(t, before.Total, after.Total)
		})
	}
}

func TestRecipeChange_EditPreviewDiffConfirmedPlatesAndUndo(t *testing.T) {
	t.Parallel()
	e := newRCEnv(t, askTurn(`{"kind":"edit","recipe":{"name":"Curry","ingredients":["400 g chickpeas","400 g tomato"],"steps":["Simmer everything for 25 minutes."],"changes":["More tomato for a saucier curry."]}}`))
	ctx := context.Background()
	one := 1.0
	p, err := e.plates.Create(ctx, plate.PlateInput{
		Date: day(t, "2026-11-04"), SlotID: e.slotID, Eaters: &one,
		Components: []plate.PlateComponent{{FoodID: e.curry.ID, Portions: &one}},
	})
	require.NoError(t, err)
	_, err = e.db.Exec(`UPDATE plates SET state = 'confirmed' WHERE id = ?`, p.ID)
	require.NoError(t, err)

	code, out := e.do(t, http.MethodPost, e.path(e.curry.ID, "/ask"), map[string]string{"request": "change the recipe itself: more tomato"})
	require.Equal(t, http.StatusOK, code, out)
	require.Equal(t, "edit", out["kind"])
	pv := out["preview"].(map[string]any)
	assert.Equal(t, []any{"2026-11-04"}, pv["confirmed_dates"], "the confirmed plate is named before saving")
	assert.Equal(t, []any{"400 g Chickpeas", "300 g Tomato"}, pv["before"].(map[string]any)["ingredients"])
	assert.Equal(t, []any{"400 g chickpeas", "400 g tomato"}, pv["after"].(map[string]any)["ingredients"])
	got, err := e.foods.Get(ctx, e.curry.ID)
	require.NoError(t, err)
	assert.InDelta(t, 300, got.Children[1].Grams, 0.01, "the preview writes nothing")

	code, res := e.do(t, http.MethodPost, e.path(e.curry.ID, "/edit"), out["proposal"])
	require.Equal(t, http.StatusOK, code, res)
	got, err = e.foods.Get(ctx, e.curry.ID)
	require.NoError(t, err)
	assert.InDelta(t, 400, got.Children[1].Grams, 0.01)
	assert.Equal(t, "Simmer everything for 25 minutes.", got.Instructions[0].Text)

	code, _ = e.do(t, http.MethodPost, "/api/food-events/revert", res["undo"])
	require.Equal(t, http.StatusOK, code)
	got, err = e.foods.Get(ctx, e.curry.ID)
	require.NoError(t, err)
	assert.InDelta(t, 300, got.Children[1].Grams, 0.01, "Undo restores the lines")
	assert.Equal(t, "Simmer everything for 20 minutes.", got.Instructions[0].Text)
	code, ch := e.do(t, http.MethodGet, e.path(e.curry.ID, "/changes"), nil)
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, ch["items"], "Undo restores the changes list")
}

func TestRecipeChange_CheckNeedsNoModel(t *testing.T) {
	t.Parallel()
	e := newRCEnv(t) // no scripted turn: a model call would fail
	code, out := e.do(t, http.MethodGet, e.path(e.curry.ID, "/check"), nil)
	require.Equal(t, http.StatusOK, code, out)
	assert.Len(t, out["lines"], 2)
	for _, l := range out["lines"].([]any) {
		assert.Equal(t, true, l.(map[string]any)["matched"])
	}
	assert.Empty(t, out["blocks"])
}
