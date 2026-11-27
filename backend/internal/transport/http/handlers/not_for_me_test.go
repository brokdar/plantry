package handlers_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// POST /api/foods/{id}/not-for-me archives the recipe and teaches the taste
// model by the reason; a bad reason changes nothing.
func TestNotForMeHandler(t *testing.T) {
	db := testhelper.NewTestDB(t)
	foods := sqlite.NewFoodRepo(db)
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, nil)
	foodSvc := food.NewService(foods).WithEvents(events)
	restorers[foodevent.KindRecipe] = foodSvc
	tasteSvc := taste.NewService(taste.Deps{
		Store: sqlite.NewTasteRepo(db), Foods: foods, Events: events,
		History: sqlite.NewFoodEventRepo(db), Attributes: noAttrs{},
	})
	router := transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Foods:    handlers.NewFoodHandler(foodSvc, food.NewNutritionResolver(foods), nil),
		NotForMe: handlers.NewNotForMeHandler(foodSvc, tasteSvc),
	})
	post := func(id int64, body string) int {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/foods/"+itoa(id)+"/not-for-me", strings.NewReader(body)))
		return rec.Code
	}

	src, role, portions := food.SourceManual, food.RoleMain, 2.0
	leaf := &food.Food{Name: "Reis", Kind: food.KindLeaf, Source: &src}
	require.NoError(t, foods.Create(t.Context(), leaf))
	recipe := &food.Food{
		Name: "Reispfanne", Kind: food.KindComposed, Role: &role, ReferencePortions: &portions,
		Children: []food.FoodComponent{{ChildID: leaf.ID, Amount: 100, Unit: "g", Grams: 100}},
	}
	require.NoError(t, foods.Create(t.Context(), recipe))

	assert.Equal(t, http.StatusBadRequest, post(recipe.ID, `{"reason":"nope"}`))
	got, err := foods.Get(t.Context(), recipe.ID)
	require.NoError(t, err)
	assert.False(t, got.Archived)

	assert.Equal(t, http.StatusOK, post(recipe.ID, `{"reason":"taste"}`))
	got, err = foods.Get(t.Context(), recipe.ID)
	require.NoError(t, err)
	assert.True(t, got.Archived)
	assert.Equal(t, http.StatusNotFound, post(9999, `{"reason":"taste"}`))
}
