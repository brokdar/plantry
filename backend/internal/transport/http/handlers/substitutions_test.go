package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/substitution"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// subEnv is the shop environment plus the substitution endpoints, with two
// Chili batches on different days and a shallot leaf in the catalogue.
type subEnv struct {
	*shopEnv
	api      http.Handler
	shallot  *food.Food
	a, b     int64 // the two sessions
	sessions *sqlite.CookSessionRepo
}

func newSubEnv(t *testing.T) *subEnv {
	t.Helper()
	ctx := context.Background()
	e := newShopEnv(t)
	src := food.SourceManual
	kcal, p, f, c := 72.0, 2.5, 0.1, 16.8
	shallot := &food.Food{Name: "Schalotte", Kind: food.KindLeaf, Source: &src, Kcal100g: &kcal, Protein100g: &p, Fat100g: &f, Carbs100g: &c}
	require.NoError(t, e.foods.Create(ctx, shallot))

	ok, op, of, oc := 40.0, 1.1, 0.1, 9.3
	e.onion.Kcal100g, e.onion.Protein100g, e.onion.Fat100g, e.onion.Carbs100g = &ok, &op, &of, &oc
	require.NoError(t, e.foods.Update(ctx, e.onion))

	sessions := sqlite.NewCookSessionRepo(e.db)
	events := foodevent.NewService(sqlite.NewTxRunner(e.db), sqlite.NewFoodEventRepo(e.db), map[foodevent.Kind]foodevent.Restorer{}, nil)
	svc := substitution.NewService(e.foods, sessions, sqlite.NewSubstitutionRepo(e.db), events)
	restorers := map[foodevent.Kind]foodevent.Restorer{foodevent.KindSubstitution: svc}
	events = foodevent.NewService(sqlite.NewTxRunner(e.db), sqlite.NewFoodEventRepo(e.db), restorers, nil)
	svc = substitution.NewService(e.foods, sessions, sqlite.NewSubstitutionRepo(e.db), events)
	restorers[foodevent.KindSubstitution] = svc

	shop := shopping.NewService(shopping.Deps{
		Source: sqlite.NewShoppingRepo(e.db), Foods: e.foods, Settings: e.settings, Slots: sqlite.NewSlotRepo(e.db),
	})
	sh := handlers.NewSubstitutionHandler(svc)
	r := chi.NewRouter()
	r.Get("/api/shopping-list", handlers.NewShoppingHandler(shop).List)
	r.Get("/api/substitutes", sh.Options)
	r.Post("/api/cook-sessions/{id}/overrides", sh.Apply)
	r.Delete("/api/cook-sessions/{id}/overrides", sh.Remove)
	r.Post("/api/food-events/revert", handlers.NewFoodEventHandler(events).Revert)

	se := &subEnv{shopEnv: e, api: r, shallot: shallot, sessions: sessions}
	se.a = e.chiliBatch(t)
	one := 1.0
	p2, err := e.plates.Create(ctx, plate.PlateInput{
		Date: day(t, "2026-11-05"), SlotID: e.slotID, Eaters: &one,
		Components: []plate.PlateComponent{{FoodID: e.chili.ID, Portions: &one}},
	})
	require.NoError(t, err)
	se.b = *p2.Components[0].CookSessionID
	return se
}

func (e *subEnv) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rd bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&rd).Encode(body))
	}
	w := httptest.NewRecorder()
	e.api.ServeHTTP(w, httptest.NewRequest(method, path, &rd))
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (e *subEnv) swap(t *testing.T, session int64) map[string]any {
	t.Helper()
	code, out := e.do(t, http.MethodPost, "/api/cook-sessions/"+strconv.FormatInt(session, 10)+"/overrides",
		map[string]int64{"from_food_id": e.onion.ID, "to_food_id": e.shallot.ID})
	require.Equal(t, http.StatusOK, code, out)
	return out
}

// foodOf is the food the session and its plate components point at.
func (e *subEnv) foodOf(t *testing.T, session int64) int64 {
	t.Helper()
	s, err := e.sessions.GetSession(context.Background(), session)
	require.NoError(t, err)
	links, err := e.sessions.ListLinks(context.Background(), session)
	require.NoError(t, err)
	for _, l := range links {
		assert.Equal(t, s.FoodID, l.FoodID, "a component stayed on the old recipe")
	}
	return s.FoodID
}

func (e *subEnv) itemGrams(t *testing.T, foodID int64) (float64, bool) {
	t.Helper()
	code, b, raw := e.get(t, "?from=2026-11-01&to=2026-11-07")
	require.Equal(t, http.StatusOK, code, raw)
	for _, s := range b.Sections {
		for _, it := range s.Items {
			if it.FoodID == foodID {
				return it.TotalGrams, true
			}
		}
	}
	return 0, false
}

func undoBody(out map[string]any) map[string]any { return out["undo"].(map[string]any) }

// AC-16: a substitution in cook mode changes only that batch's shopping list,
// recipe lines (so nutrition and cook-mode amounts) and Undo restores.
func TestSubstitution_AC16_OnlyThatBatchAndUndo(t *testing.T) {
	e := newSubEnv(t)
	ctx := context.Background()
	// Batch A cooks 6 portions of a 4-portion recipe (360 g onion), batch B 1.
	onion, _ := e.itemGrams(t, e.onion.ID)
	require.InDelta(t, 420.0, onion, 0.01)

	out := e.swap(t, e.b)
	variant := e.foodOf(t, e.b)
	require.NotEqual(t, e.chili.ID, variant)
	assert.Equal(t, e.chili.ID, e.foodOf(t, e.a), "the other batch keeps the recipe")

	onion, _ = e.itemGrams(t, e.onion.ID)
	shallot, ok := e.itemGrams(t, e.shallot.ID)
	require.True(t, ok)
	assert.InDelta(t, 360.0, onion, 0.01)
	assert.InDelta(t, 60.0, shallot, 0.01)

	v, err := e.foods.Get(ctx, variant)
	require.NoError(t, err)
	assert.True(t, v.Archived)
	require.NotNil(t, v.VariantGroupID)
	base, err := e.foods.Get(ctx, e.chili.ID)
	require.NoError(t, err)
	require.NotNil(t, base.VariantGroupID, "the recipe joins a variant group")
	assert.Equal(t, *base.VariantGroupID, *v.VariantGroupID)
	assert.Len(t, base.Children, 2, "the recipe itself is untouched")
	scaled, err := food.Scale(v, 1)
	require.NoError(t, err)
	names := []string{}
	for _, i := range scaled.Ingredients {
		names = append(names, i.Name)
	}
	assert.ElementsMatch(t, []string{"Kidneybohnen (Dose)", "Schalotte"}, names)

	hidden, err := e.foods.List(ctx, food.ListQuery{Archive: food.ArchiveOnly, Limit: 50})
	require.NoError(t, err)
	assert.Empty(t, hidden.Items, "the hidden variant is not in the archive list")
	siblings, err := e.foods.Siblings(ctx, *base.VariantGroupID, e.chili.ID)
	require.NoError(t, err)
	assert.Empty(t, siblings, "nor among the recipe's variants")

	code, body := e.do(t, http.MethodPost, "/api/food-events/revert", undoBody(out))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, e.chili.ID, e.foodOf(t, e.b))
	onion, _ = e.itemGrams(t, e.onion.ID)
	assert.InDelta(t, 420.0, onion, 0.01)
	_, ok = e.itemGrams(t, e.shallot.ID)
	assert.False(t, ok)
	_, err = e.foods.Get(ctx, variant)
	assert.ErrorIs(t, err, domain.ErrNotFound, "Undo removes the unreferenced variant")
}

func TestSubstitution_ReapplyReplacesAndUndoGoesBackOneStep(t *testing.T) {
	e := newSubEnv(t)
	ctx := context.Background()
	first := e.swap(t, e.b)
	v1 := e.foodOf(t, e.b)
	// The shallot is now the line; swap it back for onion (a table row too).
	code, second := e.do(t, http.MethodPost, "/api/cook-sessions/"+strconv.FormatInt(e.b, 10)+"/overrides",
		map[string]int64{"from_food_id": e.shallot.ID, "to_food_id": e.onion.ID})
	require.Equal(t, http.StatusOK, code, second)
	v2 := e.foodOf(t, e.b)
	require.NotEqual(t, v1, v2)
	_, err := e.foods.Get(ctx, v1)
	require.NoError(t, err, "the replaced variant stays until its undo window ends")

	code, body := e.do(t, http.MethodPost, "/api/food-events/revert", undoBody(second))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, v1, e.foodOf(t, e.b), "Undo goes back one step")
	_, err = e.foods.Get(ctx, v2)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	_ = first
}

func TestSubstitution_RemoveAndItsUndo(t *testing.T) {
	e := newSubEnv(t)
	e.swap(t, e.b)
	variant := e.foodOf(t, e.b)

	code, out := e.do(t, http.MethodDelete, "/api/cook-sessions/"+strconv.FormatInt(e.b, 10)+"/overrides", nil)
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, e.chili.ID, e.foodOf(t, e.b))
	onion, _ := e.itemGrams(t, e.onion.ID)
	assert.InDelta(t, 420.0, onion, 0.01)

	code, body := e.do(t, http.MethodPost, "/api/food-events/revert", undoBody(out))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, variant, e.foodOf(t, e.b), "Undo of the removal brings the swap back")

	code, out = e.do(t, http.MethodDelete, "/api/cook-sessions/"+strconv.FormatInt(e.a, 10)+"/overrides", nil)
	assert.Equal(t, http.StatusNotFound, code, out)
}

func TestSubstitution_OnlyTableSwapsAreAccepted(t *testing.T) {
	e := newSubEnv(t)
	code, out := e.do(t, http.MethodPost, "/api/cook-sessions/"+strconv.FormatInt(e.b, 10)+"/overrides",
		map[string]int64{"from_food_id": e.onion.ID, "to_food_id": e.apple.ID})
	assert.Equal(t, http.StatusBadRequest, code, out)
	assert.Equal(t, e.chili.ID, e.foodOf(t, e.b))
}

func TestSubstitution_Options(t *testing.T) {
	e := newSubEnv(t)
	code, out := e.do(t, http.MethodGet, "/api/substitutes?food_id="+strconv.FormatInt(e.onion.ID, 10)+"&session_ids="+strconv.FormatInt(e.b, 10), nil)
	require.Equal(t, http.StatusOK, code, out)
	opts := out["options"].([]any)
	require.Len(t, opts, 1, "only catalogue foods with nutrition are offered")
	o := opts[0].(map[string]any)
	assert.Equal(t, "Schalotte", o["name"])
	assert.InDelta(t, 1.0, o["ratio"], 0)
	assert.Equal(t, map[string]any{"amount": 60.0, "unit": "g"}, o["amount"]) // 240 g for 4 portions, batch of 1
}
