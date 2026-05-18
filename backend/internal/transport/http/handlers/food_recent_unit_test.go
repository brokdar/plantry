package handlers_test

import (
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
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// newRecentUnitRouter wires the recent-unit endpoint exactly the way the
// production router does (under /api/foods/{id}/recent-unit) so tests cover
// the URL the frontend actually calls.
func newRecentUnitRouter(t *testing.T) (http.Handler, *sqlite.FoodRepo, *sqlite.PlateRepo, *sqlite.SlotRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	foodRepo := sqlite.NewFoodRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateSvc := plate.NewService(plateRepo, slotRepo, foodRepo)
	h := handlers.NewPlateHandler(plateSvc)
	r := chi.NewRouter()
	r.Get("/api/foods/{id}/recent-unit", h.RecentUnit)
	return r, foodRepo, plateRepo, slotRepo
}

// seedFoods creates one leaf (apple) and one composed (curry) food and a
// time-slot, returning the ids needed to seed plates against them.
func seedFoodsForRecent(t *testing.T, fr *sqlite.FoodRepo, sr *sqlite.SlotRepo) (slotID, leafID, composedID int64) {
	t.Helper()
	ctx := context.Background()
	s := &slot.TimeSlot{NameKey: "dinner", Icon: "🍽", SortOrder: 0, Active: true}
	require.NoError(t, sr.Create(ctx, s))

	src := food.SourceManual
	apple := &food.Food{
		Name:     "Apple",
		Kind:     food.KindLeaf,
		Source:   &src,
		Portions: []food.Portion{{Unit: "apple", Grams: 180}},
	}
	require.NoError(t, fr.Create(ctx, apple))

	role := food.RoleMain
	ref := float64(1)
	curry := &food.Food{
		Name:              "Curry",
		Kind:              food.KindComposed,
		Role:              &role,
		ReferencePortions: &ref,
	}
	require.NoError(t, fr.Create(ctx, curry))

	return s.ID, apple.ID, curry.ID
}

func TestRecentUnit_NoPriorPlates(t *testing.T) {
	r, fr, _, sr := newRecentUnitRouter(t)
	_, leafID, _ := seedFoodsForRecent(t, fr, sr)

	req := httptest.NewRequest("GET", "/api/foods/"+strconv.FormatInt(leafID, 10)+"/recent-unit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	// JSON null decodes as Go nil; ensure the field is null (not "").
	assert.Nil(t, body["unit"])
}

func TestRecentUnit_ReturnsLastUnit(t *testing.T) {
	r, fr, pr, sr := newRecentUnitRouter(t)
	slotID, leafID, _ := seedFoodsForRecent(t, fr, sr)

	ctx := context.Background()
	require.NoError(t, pr.Create(ctx, &plate.Plate{
		Date:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SlotID: slotID,
	}))
	// Two leaf components — the most recent (max id) wins.
	plates, err := pr.ListByDateRange(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, plates, 1)
	plateID := plates[0].ID

	a := float64(200)
	gramsG := float64(200)
	srcDirect := "direct"
	unitG := "g"
	require.NoError(t, pr.CreateComponent(ctx, &plate.PlateComponent{
		PlateID: plateID, FoodID: leafID,
		Amount: &a, Unit: &unitG, Grams: &gramsG, GramsSource: &srcDirect,
	}))
	a2 := float64(1)
	gramsApple := float64(180)
	unitApple := "apple"
	srcPortion := "portion"
	require.NoError(t, pr.CreateComponent(ctx, &plate.PlateComponent{
		PlateID: plateID, FoodID: leafID,
		Amount: &a2, Unit: &unitApple, Grams: &gramsApple, GramsSource: &srcPortion,
	}))

	req := httptest.NewRequest("GET", "/api/foods/"+strconv.FormatInt(leafID, 10)+"/recent-unit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Unit *string `json:"unit"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotNil(t, body.Unit)
	assert.Equal(t, "apple", *body.Unit)
}

func TestRecentUnit_SkipsComposedRows(t *testing.T) {
	// A composed plate row sets unit IS NULL; the query's WHERE clause must
	// skip it and fall through to the next leaf usage.
	r, fr, pr, sr := newRecentUnitRouter(t)
	slotID, leafID, composedID := seedFoodsForRecent(t, fr, sr)

	ctx := context.Background()
	require.NoError(t, pr.Create(ctx, &plate.Plate{
		Date:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SlotID: slotID,
	}))
	plates, err := pr.ListByDateRange(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	plateID := plates[0].ID

	// First a leaf usage (unit = "g")
	a := float64(50)
	g := float64(50)
	uG := "g"
	srcD := "direct"
	require.NoError(t, pr.CreateComponent(ctx, &plate.PlateComponent{
		PlateID: plateID, FoodID: leafID,
		Amount: &a, Unit: &uG, Grams: &g, GramsSource: &srcD,
	}))
	// Then a composed usage on a *different* food (unit IS NULL); this must
	// not affect the leaf food's recent unit.
	pp := 2
	require.NoError(t, pr.CreateComponent(ctx, &plate.PlateComponent{
		PlateID: plateID, FoodID: composedID,
		Portions: &pp,
	}))

	req := httptest.NewRequest("GET", "/api/foods/"+strconv.FormatInt(leafID, 10)+"/recent-unit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Unit *string `json:"unit"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotNil(t, body.Unit)
	assert.Equal(t, "g", *body.Unit)
}

func TestRecentUnit_InvalidID(t *testing.T) {
	r, _, _, _ := newRecentUnitRouter(t)
	req := httptest.NewRequest("GET", "/api/foods/not-a-number/recent-unit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
