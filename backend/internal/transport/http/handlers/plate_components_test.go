package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// newComponentRouterWithDB wires a real-DB plate handler so the JSON shapes
// round-trip through both the service and the repo.
func newComponentRouterWithDB(t *testing.T) (http.Handler, *sqlite.FoodRepo, *sqlite.PlateRepo, *sqlite.SlotRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	foodRepo := sqlite.NewFoodRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateSvc := plate.NewService(plateRepo, slotRepo, foodRepo)
	h := handlers.NewPlateHandler(plateSvc)
	r := chi.NewRouter()
	r.Route("/api/plates", func(r chi.Router) {
		r.Post("/{id}/components", h.AddComponent)
		r.Put("/{id}/components/{pcId}", h.UpdateComponent)
		r.Get("/", h.List)
	})
	return r, foodRepo, plateRepo, slotRepo
}

func seedComponentTestData(t *testing.T, foodRepo *sqlite.FoodRepo, plateRepo *sqlite.PlateRepo, slotRepo *sqlite.SlotRepo) (slotID, leafID, composedID, plateID int64) {
	t.Helper()
	ctx := context.Background()
	s := &slot.TimeSlot{NameKey: "dinner", Icon: "🍽", SortOrder: 0, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	src := food.SourceManual
	leaf := &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src}
	require.NoError(t, foodRepo.Create(ctx, leaf))

	role := food.RoleMain
	ref := float64(1)
	composed := &food.Food{
		Name: "Curry", Kind: food.KindComposed, Role: &role, ReferencePortions: &ref,
		Children: []food.FoodComponent{{ChildID: leaf.ID, Amount: 100, Unit: "g", Grams: 100}},
	}
	require.NoError(t, foodRepo.Create(ctx, composed))

	d, _ := time.Parse("2006-01-02", "2026-04-26")
	p := &plate.Plate{Date: d, SlotID: s.ID}
	require.NoError(t, plateRepo.Create(ctx, p))
	return s.ID, leaf.ID, composed.ID, p.ID
}

func postComponent(t *testing.T, r http.Handler, plateID int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/plates/%d/components", plateID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAddComponent_HTTP_Composed_201(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newComponentRouterWithDB(t)
	_, _, composedID, plateID := seedComponentTestData(t, foodRepo, plateRepo, slotRepo)

	w := postComponent(t, r, plateID,
		fmt.Sprintf(`{"food_id":%d,"portions":2}`, composedID))

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.EqualValues(t, 2, resp["portions"])
	_, hasAmount := resp["amount"]
	assert.False(t, hasAmount, "composed response must omit amount")
}

func TestAddComponent_HTTP_Leaf_201(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newComponentRouterWithDB(t)
	_, leafID, _, plateID := seedComponentTestData(t, foodRepo, plateRepo, slotRepo)

	w := postComponent(t, r, plateID,
		fmt.Sprintf(`{"food_id":%d,"amount":200,"unit":"g"}`, leafID))

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.InDelta(t, 200.0, resp["amount"].(float64), 0.01)
	assert.Equal(t, "g", resp["unit"])
	assert.InDelta(t, 200.0, resp["grams"].(float64), 0.01)
	assert.Equal(t, "direct", resp["grams_source"])
	_, hasPortions := resp["portions"]
	assert.False(t, hasPortions, "leaf response must omit portions")
}

func TestAddComponent_HTTP_BothShapes_400(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newComponentRouterWithDB(t)
	_, leafID, _, plateID := seedComponentTestData(t, foodRepo, plateRepo, slotRepo)

	w := postComponent(t, r, plateID,
		fmt.Sprintf(`{"food_id":%d,"portions":1,"amount":100,"unit":"g"}`, leafID))

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "error.plate.invalid_quantity_shape", resp["message_key"])
}

func TestAddComponent_HTTP_Composed_With_Amount_400(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newComponentRouterWithDB(t)
	_, _, composedID, plateID := seedComponentTestData(t, foodRepo, plateRepo, slotRepo)

	w := postComponent(t, r, plateID,
		fmt.Sprintf(`{"food_id":%d,"amount":100,"unit":"g"}`, composedID))

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "error.plate.invalid_quantity_for_composed", resp["message_key"])
}

func TestAddComponent_HTTP_Leaf_UnknownUnit_400(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newComponentRouterWithDB(t)
	_, leafID, _, plateID := seedComponentTestData(t, foodRepo, plateRepo, slotRepo)

	w := postComponent(t, r, plateID,
		fmt.Sprintf(`{"food_id":%d,"amount":1,"unit":"slice"}`, leafID))

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "error.plate.unit_requires_portion", resp["message_key"])
}

func TestList_HTTP_RoundTrips_BothShapes(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newComponentRouterWithDB(t)
	_, leafID, composedID, plateID := seedComponentTestData(t, foodRepo, plateRepo, slotRepo)

	require.Equal(t, http.StatusCreated,
		postComponent(t, r, plateID, fmt.Sprintf(`{"food_id":%d,"portions":2}`, composedID)).Code)
	require.Equal(t, http.StatusCreated,
		postComponent(t, r, plateID, fmt.Sprintf(`{"food_id":%d,"amount":150,"unit":"g"}`, leafID)).Code)

	req := httptest.NewRequest(http.MethodGet, "/api/plates?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	plates := body["plates"].([]any)
	require.Len(t, plates, 1)
	comps := plates[0].(map[string]any)["components"].([]any)
	require.Len(t, comps, 2)

	// First component (composed): portions present, no amount.
	c0 := comps[0].(map[string]any)
	assert.EqualValues(t, 2, c0["portions"])
	_, hasAmount := c0["amount"]
	assert.False(t, hasAmount)
	// Second component (leaf): amount/unit/grams present, no portions.
	c1 := comps[1].(map[string]any)
	_, hasPortions := c1["portions"]
	assert.False(t, hasPortions)
	assert.InDelta(t, 150.0, c1["amount"].(float64), 0.01)
	assert.Equal(t, "g", c1["unit"])
}
