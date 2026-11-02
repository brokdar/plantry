package handlers_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// Z4-AC2 (AC-7.2): the Phase 2 fixture, migrated to head, answers the plan,
// today, shopping-list and foods endpoints with its seeded data.
func TestPhase2FixtureServesAfterMigration(t *testing.T) {
	db := testhelper.CopyDB(t, "../../../adapters/sqlite/testdata/phase2.db")
	testhelper.MigrateDB(t, db)

	foods := sqlite.NewFoodRepo(db)
	router := transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Foods:    handlers.NewFoodHandler(food.NewService(foods), food.NewNutritionResolver(foods), nil),
		PlanRead: handlers.NewPlanReadHandler(testsvc.PlanView(db)),
		Today:    handlers.NewTodayHandler(testsvc.PlanView(db)),
		Shopping: handlers.NewShoppingHandler(shopping.NewService(shopping.Deps{
			Source: sqlite.NewShoppingRepo(db), Foods: foods, Settings: testsvc.Settings(db), Slots: sqlite.NewSlotRepo(db),
		})),
	})
	get := func(path string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code, path+" "+w.Body.String())
		out := map[string]any{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
		return out
	}

	// The fixture's plates span 2026-10-26 .. 2026-11-08 (Monday weeks).
	plan := get("/api/plan?start=2026-11-02&today=2026-11-04")
	assert.Contains(t, mustJSON(t, plan), "Chicken", "this week's plan names seeded recipes")
	assert.NotEmpty(t, plan["days"])
	today := get("/api/today?date=2026-11-04")
	assert.NotEmpty(t, today["date"], mustJSON(t, today))
	shop := get("/api/shopping-list?from=2026-11-02&to=2026-11-08")
	assert.EqualValues(t, 7, shop["count"], mustJSON(t, shop))
	list := get("/api/foods?limit=100")
	assert.EqualValues(t, 20, list["total"])
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
