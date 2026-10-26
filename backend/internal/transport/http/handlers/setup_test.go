package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func newSetupRouter(t *testing.T) (http.Handler, *sqlite.FoodRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	foods := sqlite.NewFoodRepo(db)
	settingsSvc := settings.NewService(sqlite.NewAppSettingsRepo(db), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	h := handlers.NewSetupHandler(setup.NewService(settingsSvc, foods))
	r := chi.NewRouter()
	r.Get("/api/setup", h.Get)
	r.Post("/api/setup/complete", h.Complete)
	return r, foods
}

func serveSetup(t *testing.T, r http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestSetupHandler_GetFresh(t *testing.T) {
	r, _ := newSetupRouter(t)
	rec := serveSetup(t, r, http.MethodGet, "/api/setup")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"completed":false,"recipe_count":0,"recipe_target":10}`, rec.Body.String())
}

func TestSetupHandler_GetCountsComposedOnly(t *testing.T) {
	r, foods := newSetupRouter(t)
	ctx := context.Background()
	src := food.SourceManual
	portions := 1.0
	leaf := &food.Food{Name: "Oats", Kind: food.KindLeaf, Source: &src}
	require.NoError(t, foods.Create(ctx, leaf))
	require.NoError(t, foods.Create(ctx, &food.Food{
		Name: "Porridge", Kind: food.KindComposed, ReferencePortions: &portions,
		Children: []food.FoodComponent{{ChildID: leaf.ID, Amount: 50, Unit: "g", Grams: 50}},
	}))

	rec := serveSetup(t, r, http.MethodGet, "/api/setup")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"completed":false,"recipe_count":1,"recipe_target":10}`, rec.Body.String())
}

func TestSetupHandler_CompleteIsIdempotent(t *testing.T) {
	r, _ := newSetupRouter(t)
	want := `{"completed":true,"recipe_count":0,"recipe_target":10}`
	for range 2 {
		rec := serveSetup(t, r, http.MethodPost, "/api/setup/complete")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, want, rec.Body.String())
	}
	rec := serveSetup(t, r, http.MethodGet, "/api/setup")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, want, rec.Body.String())
}
