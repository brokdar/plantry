package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func TestKitchenHandler(t *testing.T) {
	db := testhelper.NewTestDB(t)
	_, err := db.Exec(`INSERT INTO foods (id, name, kind, source) VALUES (1, 'Zucker', 'leaf', 'manual')`)
	require.NoError(t, err)
	h := handlers.NewKitchenHandler(profile.NewService(sqlite.NewProfileRepo(db)), kitchen.NewService(sqlite.NewKitchenRepo(db)))
	r := chi.NewRouter()
	r.Get("/api/kitchen", h.Get)
	r.Put("/api/kitchen", h.Update)
	r.Put("/api/kitchen/basics/{id}", h.AddBasic)
	r.Delete("/api/kitchen/basics/{id}", h.RemoveBasic)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	get := func() map[string]any {
		w := do(http.MethodGet, "/api/kitchen", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var m map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&m))
		return m
	}

	m := get()
	assert.EqualValues(t, 30, m["minutes_weekday"])
	assert.EqualValues(t, 60, m["minutes_weekend"])
	assert.Equal(t, []any{"oven", "stovetop"}, m["equipment"])
	assert.Equal(t, []any{}, m["basics"])

	w := do(http.MethodPut, "/api/kitchen", `{"minutes_weekday":20,"minutes_weekend":75,"skill":"beginner","equipment":["oven"],"heat":"mild","leanings":["lemon"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	m = get()
	assert.EqualValues(t, 20, m["minutes_weekday"])
	assert.Equal(t, "beginner", m["skill"])
	assert.Equal(t, []any{"lemon"}, m["leanings"])

	assert.Equal(t, http.StatusBadRequest, do(http.MethodPut, "/api/kitchen", `{"minutes_weekday":1,"minutes_weekend":60}`).Code)

	require.Equal(t, http.StatusNoContent, do(http.MethodPut, "/api/kitchen/basics/1", "").Code)
	assert.Equal(t, []any{map[string]any{"id": float64(1), "name": "Zucker"}}, get()["basics"])
	assert.Equal(t, http.StatusNotFound, do(http.MethodPut, "/api/kitchen/basics/9", "").Code)
	require.Equal(t, http.StatusNoContent, do(http.MethodDelete, "/api/kitchen/basics/1", "").Code)
	assert.Equal(t, []any{}, get()["basics"])
}
