package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/recipechange"
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
	now      *time.Time // the service's clock
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
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(e.db), sqlite.NewFoodEventRepo(e.db), restorers, nil)
	recipes := recipechange.NewService(e.foods, nil, nil, sqlite.NewRecipeChangeRepo(e.db), nil, events)
	svc := substitution.NewService(e.foods, sessions, sqlite.NewSubstitutionRepo(e.db), recipes, events)
	restorers[foodevent.KindSubstitution] = svc
	restorers[foodevent.KindRecipeContent] = recipes
	now := day(t, "2026-11-01")
	svc.Now = func() time.Time { return now }

	shop := shopping.NewService(shopping.Deps{
		Source: sqlite.NewShoppingRepo(e.db), Foods: e.foods, Settings: e.settings, Slots: sqlite.NewSlotRepo(e.db),
	})
	sh := handlers.NewSubstitutionHandler(svc)
	r := chi.NewRouter()
	r.Get("/api/shopping-list", handlers.NewShoppingHandler(shop).List)
	r.Get("/api/substitutes", sh.Options)
	r.Post("/api/cook-sessions/{id}/overrides", sh.Apply)
	r.Delete("/api/cook-sessions/{id}/overrides", sh.Remove)
	r.Post("/api/cook-sessions/{id}/overrides/keep", sh.Keep)
	r.Post("/api/food-events/revert", handlers.NewFoodEventHandler(events).Revert)

	se := &subEnv{shopEnv: e, api: r, shallot: shallot, sessions: sessions, now: &now}
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

func (e *subEnv) hiddenCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM hidden_variants`).Scan(&n))
	return n
}

func (e *subEnv) eventCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM food_events`).Scan(&n))
	return n
}

// A session whose recipe has no line of the ingredient is refused before any
// write: no variant, no re-pointing, no event.
func TestSubstitution_MissingLineWritesNothing(t *testing.T) {
	e := newSubEnv(t)
	events := e.eventCount(t)
	var before int
	require.NoError(t, e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM foods`).Scan(&before))
	code, out := e.do(t, http.MethodPost, "/api/cook-sessions/"+strconv.FormatInt(e.b, 10)+"/overrides",
		map[string]int64{"from_food_id": e.shallot.ID, "to_food_id": e.onion.ID})
	assert.Equal(t, http.StatusBadRequest, code, out)
	assert.Equal(t, e.chili.ID, e.foodOf(t, e.b))
	var after int
	require.NoError(t, e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM foods`).Scan(&after))
	assert.Equal(t, before, after, "no clone")
	assert.Zero(t, e.hiddenCount(t))
	assert.Equal(t, events, e.eventCount(t), "no event")
}

// The shop dialog offers only the batches whose recipe has the item.
func TestSubstitution_OptionsListApplicableSessions(t *testing.T) {
	e := newSubEnv(t)
	e.swap(t, e.b) // b now has shallot, a still onion
	ids := strconv.FormatInt(e.a, 10) + "," + strconv.FormatInt(e.b, 10)
	code, out := e.do(t, http.MethodGet, "/api/substitutes?food_id="+strconv.FormatInt(e.shallot.ID, 10)+"&session_ids="+ids, nil)
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, []any{float64(e.b)}, out["session_ids"])

	code, out = e.do(t, http.MethodGet, "/api/substitutes?food_id="+strconv.FormatInt(e.apple.ID, 10)+"&session_ids="+ids, nil)
	assert.Equal(t, http.StatusBadRequest, code, out)
}

// Confirmed sessions of today and later stay open to a swap; cooked ones and
// ones with a plate before today are refused.
func TestSubstitution_PastSessionsAreRefused(t *testing.T) {
	e := newSubEnv(t)
	swap := func() (int, map[string]any) {
		return e.do(t, http.MethodPost, "/api/cook-sessions/"+strconv.FormatInt(e.b, 10)+"/overrides",
			map[string]int64{"from_food_id": e.onion.ID, "to_food_id": e.shallot.ID})
	}
	sess, err := e.sessions.GetSession(context.Background(), e.b)
	require.NoError(t, err)
	cooked := day(t, "2026-11-05")
	sess.CookedAt = &cooked
	require.NoError(t, e.sessions.UpdateSession(context.Background(), sess))
	code, out := swap()
	assert.Equal(t, http.StatusBadRequest, code, "cooked: %v", out)
	sess.CookedAt = nil
	require.NoError(t, e.sessions.UpdateSession(context.Background(), sess))

	*e.now = day(t, "2026-11-06")
	code, out = swap()
	assert.Equal(t, http.StatusBadRequest, code, "plate in the past: %v", out)
	*e.now = day(t, "2026-11-05")
	code, out = swap()
	assert.Equal(t, http.StatusOK, code, "plate today: %v", out)
}

// Monday's batch substituted and eaten: the recipe is the one eaten (last
// eaten, times cooked), not the hidden variant.
func TestSubstitution_HistoryFollowsTheRecipe(t *testing.T) {
	e := newSubEnv(t)
	ctx := context.Background()
	e.swap(t, e.b)
	variant := e.foodOf(t, e.b)
	h, err := e.foods.History(ctx, []int64{e.chili.ID, variant}, day(t, "2026-11-10"))
	require.NoError(t, err)
	require.NotNil(t, h[e.chili.ID].LastEaten, "the recipe is eaten through its variant")
	assert.Equal(t, day(t, "2026-11-05"), *h[e.chili.ID].LastEaten)
	assert.Equal(t, 2, h[e.chili.ID].TimesCooked, "both batches count")
	base, err := e.foods.LookupByIDs(ctx, []int64{variant, e.chili.ID})
	require.NoError(t, err)
	assert.Equal(t, e.chili.ID, base[variant].HiddenBaseID)
	assert.Zero(t, base[e.chili.ID].HiddenBaseID)

	uneaten, err := e.foods.UneatenRecipes(ctx, day(t, "2026-11-03"), day(t, "2026-11-10"))
	require.NoError(t, err)
	for _, r := range uneaten {
		assert.NotEqual(t, e.chili.ID, r.ID, "eaten through the variant, so not uneaten")
	}
}

func (e *subEnv) keep(t *testing.T, session int64, mode string) (int, map[string]any) {
	t.Helper()
	return e.do(t, http.MethodPost, "/api/cook-sessions/"+strconv.FormatInt(session, 10)+"/overrides/keep",
		map[string]any{"from_food_id": e.onion.ID, "to_food_id": e.shallot.ID, "mode": mode})
}

// HELP-7: "Always in this recipe" edits the recipe, points the batch back at
// it, lists the change and one Undo restores both.
func TestSubstitution_AlwaysEditsTheRecipeAndUndo(t *testing.T) {
	e := newSubEnv(t)
	ctx := context.Background()
	e.swap(t, e.b)
	variant := e.foodOf(t, e.b)

	code, out := e.keep(t, e.b, "recipe")
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, e.chili.ID, e.foodOf(t, e.b), "the batch points at the recipe again")
	base, err := e.foods.Get(ctx, e.chili.ID)
	require.NoError(t, err)
	ids := []int64{}
	for _, c := range base.Children {
		ids = append(ids, c.ChildID)
	}
	assert.Contains(t, ids, e.shallot.ID)
	assert.NotContains(t, ids, e.onion.ID)
	changes, err := sqlite.NewRecipeChangeRepo(e.db).Changes(ctx, e.chili.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Zwiebel \u2192 Schalotte"}, changes)
	_, onionLeft := e.itemGrams(t, e.onion.ID)
	assert.False(t, onionLeft, "the recipe has shallot now")

	code, body := e.do(t, http.MethodPost, "/api/food-events/revert", undoBody(out))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, variant, e.foodOf(t, e.b), "Undo brings the batch's swap back")
	base, err = e.foods.Get(ctx, e.chili.ID)
	require.NoError(t, err)
	ids = ids[:0]
	for _, c := range base.Children {
		ids = append(ids, c.ChildID)
	}
	assert.Contains(t, ids, e.onion.ID, "Undo puts the recipe back")
	changes, err = sqlite.NewRecipeChangeRepo(e.db).Changes(ctx, e.chili.ID)
	require.NoError(t, err)
	assert.Empty(t, changes)
}

// HELP-7: "Save as variant" makes the batch's hidden variant a visible one.
func TestSubstitution_SaveAsVariantAndUndo(t *testing.T) {
	e := newSubEnv(t)
	ctx := context.Background()
	e.swap(t, e.b)
	variant := e.foodOf(t, e.b)

	code, out := e.keep(t, e.b, "variant")
	require.Equal(t, http.StatusOK, code, out)
	assert.EqualValues(t, variant, out["food_id"])
	assert.Equal(t, e.chili.Name+" with Schalotte", out["name"])
	assert.Equal(t, variant, e.foodOf(t, e.b), "the batch keeps cooking it")
	v, err := e.foods.Get(ctx, variant)
	require.NoError(t, err)
	assert.False(t, v.Archived)
	assert.Equal(t, e.chili.Name+" with Schalotte", v.Name)
	base, err := e.foods.Get(ctx, e.chili.ID)
	require.NoError(t, err)
	siblings, err := e.foods.Siblings(ctx, *base.VariantGroupID, e.chili.ID)
	require.NoError(t, err)
	require.Len(t, siblings, 1, "it is a visible variant in the group")
	changes, err := sqlite.NewRecipeChangeRepo(e.db).Changes(ctx, variant)
	require.NoError(t, err)
	assert.Equal(t, []string{"Zwiebel \u2192 Schalotte"}, changes)
	// A later swap in this batch builds on it and leaves it alone.
	e.swap(t, e.a)
	_, err = e.foods.Get(ctx, variant)
	require.NoError(t, err)

	code, body := e.do(t, http.MethodPost, "/api/food-events/revert", undoBody(out))
	require.Equal(t, http.StatusOK, code, body)
	v, err = e.foods.Get(ctx, variant)
	require.NoError(t, err)
	assert.True(t, v.Archived, "Undo hides it again")
	assert.NotEqual(t, e.chili.Name+" with Schalotte", v.Name)
	siblings, err = e.foods.Siblings(ctx, *base.VariantGroupID, e.chili.ID)
	require.NoError(t, err)
	assert.Empty(t, siblings)
	changes, err = sqlite.NewRecipeChangeRepo(e.db).Changes(ctx, variant)
	require.NoError(t, err)
	assert.Empty(t, changes)
}

func TestSubstitution_KeepNeedsASwap(t *testing.T) {
	e := newSubEnv(t)
	for _, mode := range []string{"recipe", "variant"} {
		code, out := e.keep(t, e.b, mode)
		assert.Equal(t, http.StatusNotFound, code, out)
	}
	code, out := e.keep(t, e.b, "nope")
	assert.Equal(t, http.StatusBadRequest, code, out)
}

// HELP-7: the second time the same swap is applied in a recipe the response
// offers to make it permanent, once.
func TestSubstitution_SecondSameSwapOffersPermanentOnce(t *testing.T) {
	e := newSubEnv(t)
	assert.Equal(t, false, e.swap(t, e.b)["offer_permanent"])
	assert.Equal(t, true, e.swap(t, e.a)["offer_permanent"], "second swap, other batch")
	code, out := e.do(t, http.MethodDelete, "/api/cook-sessions/"+strconv.FormatInt(e.b, 10)+"/overrides", nil)
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, false, e.swap(t, e.b)["offer_permanent"], "offered once only")
}

func TestSubstitution_ApplyReportsConfirmedDates(t *testing.T) {
	e := newSubEnv(t)
	out := e.swap(t, e.b)
	assert.NotNil(t, out["confirmed_dates"])
}
