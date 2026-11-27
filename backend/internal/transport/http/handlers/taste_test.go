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
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
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
