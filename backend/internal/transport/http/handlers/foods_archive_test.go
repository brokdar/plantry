package handlers_test

import (
	"encoding/json"
	"fmt"
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
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// Archive and restore (LIB-4): the recipe leaves the default list and shows
// under ?archived=1, stays readable by id, both log a recipe food event, and
// Revert of the archive event un-hides it.
func TestFoodsArchiveHTTP(t *testing.T) {
	db := testhelper.NewTestDB(t)
	foods := sqlite.NewFoodRepo(db)
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, nil)
	svc := food.NewService(foods).WithEvents(events)
	restorers[foodevent.KindRecipe] = svc
	router := transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Foods:      handlers.NewFoodHandler(svc, food.NewNutritionResolver(foods), nil),
		FoodEvents: handlers.NewFoodEventHandler(events),
	})
	do := func(method, path, body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	listed := func(query string) []string {
		code, body := do(http.MethodGet, "/api/foods?kind=composed"+query, "")
		require.Equal(t, http.StatusOK, code, body)
		out := []string{}
		for _, it := range body["items"].([]any) {
			out = append(out, it.(map[string]any)["name"].(string))
		}
		return out
	}

	src, role, portions := food.SourceManual, food.RoleMain, 2.0
	leaf := &food.Food{Name: "Reis", Kind: food.KindLeaf, Source: &src}
	require.NoError(t, foods.Create(t.Context(), leaf))
	recipe := &food.Food{
		Name: "Reispfanne", Kind: food.KindComposed, Role: &role, ReferencePortions: &portions,
		Children: []food.FoodComponent{{ChildID: leaf.ID, Amount: 100, Unit: "g", Grams: 100}},
	}
	require.NoError(t, foods.Create(t.Context(), recipe))
	path := fmt.Sprintf("/api/foods/%d", recipe.ID)

	code, body := do(http.MethodPost, path+"/archive", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, true, body["archived"])
	assert.Empty(t, listed(""))
	assert.Equal(t, []string{"Reispfanne"}, listed("&archived=1"))
	code, body = do(http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, body["archived"], "readable by id")

	code, body = do(http.MethodPost, path+"/restore", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, false, body["archived"])
	assert.Equal(t, []string{"Reispfanne"}, listed(""))
	assert.Empty(t, listed("&archived=1"))

	last, err := sqlite.NewFoodEventRepo(db).LastID(t.Context())
	require.NoError(t, err)
	evs, err := sqlite.NewFoodEventRepo(db).Range(t.Context(), 1, last)
	require.NoError(t, err)
	require.Len(t, evs, 2, "archive and restore each log one event")
	for _, e := range evs {
		assert.Equal(t, foodevent.KindRecipe, e.Kind)
		assert.Equal(t, recipe.ID, e.SubjectID)
	}

	// Undo of the restore hides it again; a repeat archive logs nothing new.
	code, _ = do(http.MethodPost, "/api/food-events/revert", fmt.Sprintf(`{"first_event_id":%d,"last_event_id":%d}`, evs[1].ID, evs[1].ID))
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, listed(""))
	do(http.MethodPost, path+"/archive", "")
	l2, _ := sqlite.NewFoodEventRepo(db).LastID(t.Context())
	assert.Equal(t, last+1, l2, "only the undo event was added")

	// A leaf cannot be archived, a missing food is not found.
	code, _ = do(http.MethodPost, fmt.Sprintf("/api/foods/%d/archive", leaf.ID), "")
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = do(http.MethodPost, "/api/foods/9999/archive", "")
	assert.Equal(t, http.StatusNotFound, code)
}
