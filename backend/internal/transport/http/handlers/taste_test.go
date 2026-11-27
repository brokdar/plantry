package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// GET /api/taste lists the items with source, polarity and score; PUT flips
// one, DELETE removes one, and both Undo through the food event revert.
func TestTasteHandler(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, nil)
	svc := taste.NewService(taste.Deps{
		Store: sqlite.NewTasteRepo(db), Foods: sqlite.NewFoodRepo(db), Events: events,
		History: sqlite.NewFoodEventRepo(db), Attributes: noAttrs{},
	})
	restorers[foodevent.KindTasteItem] = svc
	thai, _, err := svc.State(ctx, taste.Ref{Level: taste.LevelCuisine, Key: "thai"}, taste.Love, taste.ScopeAlways, time.Time{})
	require.NoError(t, err)
	week := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	_, _, err = svc.State(ctx, taste.Ref{Level: taste.LevelHero, Key: "fish"}, taste.RatherNot, taste.ScopeWindow, week)
	require.NoError(t, err)

	h := handlers.NewTasteHandler(svc)
	r := chi.NewRouter()
	r.Get("/api/taste", h.Get)
	r.Put("/api/taste/items/{id}", h.Put)
	r.Delete("/api/taste/items/{id}", h.Delete)
	r.Post("/api/food-events/revert", handlers.NewFoodEventHandler(events).Revert)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	get := func(path string) taste.View {
		w := do(http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var v taste.View
		require.NoError(t, json.NewDecoder(w.Body).Decode(&v))
		return v
	}

	v := get("/api/taste")
	require.Len(t, v.Items, 1, "the window item only with its window")
	assert.Equal(t, taste.ItemView{ID: thai.ID, Level: taste.LevelCuisine, Key: "thai", Scope: taste.ScopeAlways, Source: taste.SourceStatement, Polarity: taste.Love, Score: 1}, v.Items[0])
	assert.Len(t, get("/api/taste?window=2026-03-02").Items, 2)
	assert.Equal(t, http.StatusBadRequest, do(http.MethodGet, "/api/taste?window=monday", "").Code)

	w := do(http.MethodPut, "/api/taste/items/"+itoa(thai.ID), `{"polarity":"rather_not"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var put struct {
		Item taste.ItemView `json:"item"`
		Undo foodevent.Undo `json:"undo"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&put))
	assert.Equal(t, taste.RatherNot, put.Item.Polarity)
	assert.Equal(t, taste.SourceUser, put.Item.Source)
	undo, _ := json.Marshal(put.Undo)
	require.Equal(t, http.StatusOK, do(http.MethodPost, "/api/food-events/revert", string(undo)).Code)
	assert.Equal(t, taste.Love, get("/api/taste").Items[0].Polarity, "undone")

	w = do(http.MethodDelete, "/api/taste/items/"+itoa(thai.ID), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, get("/api/taste").Items)
	var del struct {
		Undo foodevent.Undo `json:"undo"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&del))
	undo, _ = json.Marshal(del.Undo)
	require.Equal(t, http.StatusOK, do(http.MethodPost, "/api/food-events/revert", string(undo)).Code)
	assert.Len(t, get("/api/taste").Items, 1, "restored")

	assert.Equal(t, http.StatusBadRequest, do(http.MethodPut, "/api/taste/items/"+itoa(thai.ID), `{"polarity":"never"}`).Code, "never is the kitchen's, not a taste polarity")
	assert.Equal(t, http.StatusBadRequest, do(http.MethodPut, "/api/taste/items/x", `{"polarity":"love"}`).Code)
	assert.Equal(t, http.StatusNotFound, do(http.MethodPut, "/api/taste/items/999", `{"polarity":"love"}`).Code)
	assert.Equal(t, http.StatusNotFound, do(http.MethodDelete, "/api/taste/items/999", "").Code)
}

type noAttrs struct{}

func (noAttrs) Get(_ context.Context, id int64) (flavor.View, error) {
	return flavor.View{FoodID: id}, nil
}

// TASTE-8, TASTE-11: the Taste page lists the never items, a chip tap
// confirms a statement (taste: permanent; health: a never item, or a
// restriction chip without a food of that name), and "Never" on a hero item
// makes its food a never item.
func TestTasteHandler_NeverAndConfirm(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, nil)
	svc := taste.NewService(taste.Deps{
		Store: sqlite.NewTasteRepo(db), Foods: sqlite.NewFoodRepo(db), Events: events,
		History: sqlite.NewFoodEventRepo(db), Attributes: noAttrs{},
	})
	foods := food.NewService(sqlite.NewFoodRepo(db))
	peanuts := &food.Food{Name: "peanuts", Kind: food.KindLeaf}
	require.NoError(t, foods.Create(ctx, peanuts))
	h := handlers.NewTasteHandler(svc).WithKitchen(kitchen.NewService(sqlite.NewKitchenRepo(db)), foods, profile.NewService(sqlite.NewProfileRepo(db)))
	r := chi.NewRouter()
	r.Get("/api/taste", h.Get)
	r.Post("/api/taste/items/{id}/never", h.MakeNever)
	r.Post("/api/taste/statements/confirm", h.Confirm)
	r.Delete("/api/taste/never/{id}", h.RemoveNever)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	get := func() struct {
		Never []struct {
			Name string `json:"name"`
		} `json:"never"`
		Items []taste.ItemView `json:"items"`
	} {
		var v struct {
			Never []struct {
				Name string `json:"name"`
			} `json:"never"`
			Items []taste.ItemView `json:"items"`
		}
		require.NoError(t, json.NewDecoder(do(http.MethodGet, "/api/taste", "").Body).Decode(&v))
		return v
	}

	week := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	_, _, err := svc.State(ctx, taste.Ref{Level: taste.LevelHero, Key: "coriander"}, taste.RatherNot, taste.ScopeWindow, week)
	require.NoError(t, err)
	body := `{"kind":"taste","level":"hero","key":"coriander","polarity":"rather_not","scope":"always","window_start":"2026-03-02"}`
	require.Equal(t, http.StatusOK, do(http.MethodPost, "/api/taste/statements/confirm", body).Code)
	items := get().Items
	require.Len(t, items, 1)
	assert.Equal(t, taste.ScopeAlways, items[0].Scope, "permanent now")

	require.Equal(t, http.StatusNoContent, do(http.MethodPost, "/api/taste/statements/confirm", `{"kind":"health","word":"peanuts"}`).Code)
	require.Len(t, get().Never, 1)
	assert.Equal(t, "peanuts", get().Never[0].Name)
	require.Equal(t, http.StatusNoContent, do(http.MethodDelete, "/api/taste/never/"+itoa(peanuts.ID), "").Code)
	assert.Empty(t, get().Never)

	assert.Equal(t, http.StatusNoContent, do(http.MethodPost, "/api/taste/statements/confirm", `{"kind":"health","word":"shellfish"}`).Code, "no such food: a restriction chip")
	assert.Equal(t, http.StatusNotFound, do(http.MethodPost, "/api/taste/items/"+itoa(items[0].ID)+"/never", "").Code, "no food named coriander")
	assert.Equal(t, http.StatusBadRequest, do(http.MethodPost, "/api/taste/statements/confirm", `{"kind":"x"}`).Code)
}

// COLD-1: propose returns the statements for the chips and stores nothing.
func TestTasteHandler_Propose(t *testing.T) {
	h := handlers.NewTasteHandler(nil)
	w := httptest.NewRecorder()
	h.Propose(w, httptest.NewRequest(http.MethodPost, "/api/taste/propose", strings.NewReader(`{"text":"we love thai, no coriander"}`)))
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"statements":[{"kind":"taste","level":"cuisine","key":"thai","polarity":"love"},{"kind":"taste","level":"hero","key":"coriander","polarity":"rather_not"}]}`, w.Body.String())
}

// TASTE-11 (CP2 judges): GET /api/taste leaves out the items that say
// nothing about a household (a plated dish, boiled, simmered); they still
// exist and score. Other format and technique items stay.
func TestTasteHandler_HidesTrivialItems(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	events := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), map[foodevent.Kind]foodevent.Restorer{}, nil)
	svc := taste.NewService(taste.Deps{
		Store: sqlite.NewTasteRepo(db), Foods: sqlite.NewFoodRepo(db), Events: events,
		History: sqlite.NewFoodEventRepo(db), Attributes: noAttrs{},
	})
	for _, ref := range []taste.Ref{
		{Level: taste.LevelFormat, Key: "plate"},
		{Level: taste.LevelTechnique, Key: "boil"},
		{Level: taste.LevelTechnique, Key: "simmer"},
		{Level: taste.LevelFormat, Key: "one_pot"},
		{Level: taste.LevelTechnique, Key: "raw"},
	} {
		_, _, err := svc.State(ctx, ref, taste.Love, taste.ScopeAlways, time.Time{})
		require.NoError(t, err)
	}
	all, err := svc.View(ctx, time.Time{})
	require.NoError(t, err)
	require.Len(t, all.Items, 5, "the domain keeps them")

	w := httptest.NewRecorder()
	handlers.NewTasteHandler(svc).Get(w, httptest.NewRequest(http.MethodGet, "/api/taste", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var v taste.View
	require.NoError(t, json.NewDecoder(w.Body).Decode(&v))
	var keys []string
	for _, it := range v.Items {
		keys = append(keys, it.Key)
	}
	assert.ElementsMatch(t, []string{"one_pot", "raw"}, keys)
	for _, r := range v.Recent {
		assert.False(t, taste.Trivial(r.Item.Level, r.Item.Key), r.Item.Key)
	}
}
