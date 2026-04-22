package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type plannerRouterFixture struct {
	router    http.Handler
	slotID    int64
	compID    int64
	plateRepo *sqlite.PlateRepo
	weekRepo  *sqlite.WeekRepo
}

func setupPlannerRouter(t *testing.T) *plannerRouterFixture {
	t.Helper()
	db := testhelper.NewTestDB(t)

	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	weekRepo := sqlite.NewWeekRepo(db)
	tx := sqlite.NewTxRunner(db)

	plateSvc := plate.NewService(plateRepo, slotRepo, compRepo)
	plannerSvc := planner.NewService(weekRepo, plateRepo, tx)
	slotSvc := slot.NewService(slotRepo)
	componentSvc := component.NewService(compRepo, ingRepo, ingRepo)

	fbRepo := sqlite.NewFeedbackRepo(db)
	wh := handlers.NewWeekHandler(plannerSvc, plateSvc, componentSvc, ingRepo, fbRepo)
	ph := handlers.NewPlateHandler(plateSvc)
	sh := handlers.NewSlotHandler(slotSvc)

	r := chi.NewRouter()
	r.Route("/api/settings/slots", func(r chi.Router) {
		r.Get("/", sh.List)
		r.Post("/", sh.Create)
	})
	r.Route("/api/weeks", func(r chi.Router) {
		r.Get("/", wh.List)
		r.Get("/current", wh.Current)
		r.Get("/by-date", wh.ByDate)
		r.Get("/{id}", wh.Get)
		r.Post("/{id}/copy", wh.Copy)
		r.Post("/{id}/plates", wh.CreatePlate)
		r.Get("/{id}/shopping-list", wh.ShoppingList)
		r.Get("/{id}/nutrition", wh.Nutrition)
	})
	r.Route("/api/plates/{id}", func(r chi.Router) {
		r.Get("/", ph.Get)
		r.Put("/", ph.Update)
		r.Delete("/", ph.Delete)
		r.Post("/components", ph.AddComponent)
		r.Put("/components/{pcId}", ph.UpdateComponent)
		r.Delete("/components/{pcId}", ph.DeleteComponent)
	})

	ctx := context.Background()
	s := &slot.TimeSlot{NameKey: "slot.dinner", Icon: "Moon", SortOrder: 1, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))
	ing := &ingredient.Ingredient{Name: "Chicken", Source: "manual", Kcal100g: 100}
	require.NoError(t, ingRepo.Create(ctx, ing))
	c := &component.Component{
		Name: "Curry", Role: component.RoleMain, ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
	}
	require.NoError(t, compRepo.Create(ctx, c))

	return &plannerRouterFixture{
		router: r, slotID: s.ID, compID: c.ID,
		plateRepo: plateRepo, weekRepo: weekRepo,
	}
}

func TestWeeks_Current_CreatesOnFirstCall(t *testing.T) {
	f := setupPlannerRouter(t)

	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/current", nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var first map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&first))
	id := first["id"]

	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/current", nil))
	var second map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&second))
	assert.Equal(t, id, second["id"], "second call returns same week")
}

func TestWeeks_ByDate_BadParams(t *testing.T) {
	f := setupPlannerRouter(t)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/by-date?year=abc&week=1", nil))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestWeeks_Get_NotFound(t *testing.T) {
	f := setupPlannerRouter(t)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/9999", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestWeeks_CreatePlate_InvalidDay(t *testing.T) {
	f := setupPlannerRouter(t)
	week := getCurrentWeek(t, f.router)
	weekID := int64(week["id"].(float64))
	body := fmt.Sprintf(`{"day":7,"slot_id":%d}`, f.slotID)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/weeks/%d/plates", weekID), bytes.NewBufferString(body)))
	assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
}

func TestWeeks_CreatePlate_SlotUnknown(t *testing.T) {
	f := setupPlannerRouter(t)
	week := getCurrentWeek(t, f.router)
	weekID := int64(week["id"].(float64))
	body := `{"day":1,"slot_id":9999}`
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/weeks/%d/plates", weekID), bytes.NewBufferString(body)))
	require.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())
	var body2 map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body2))
	assert.Equal(t, "error.plate.slot_unknown", body2["message_key"])
}

func TestWeeks_FullPlateLifecycle(t *testing.T) {
	f := setupPlannerRouter(t)
	week := getCurrentWeek(t, f.router)
	weekID := int64(week["id"].(float64))

	// Create plate.
	createBody := fmt.Sprintf(`{"day":1,"slot_id":%d,"components":[{"component_id":%d,"portions":1}]}`, f.slotID, f.compID)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/weeks/%d/plates", weekID), bytes.NewBufferString(createBody)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var p map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	plateID := int64(p["id"].(float64))
	pcs := p["components"].([]any)
	require.Len(t, pcs, 1)
	pc := pcs[0].(map[string]any)
	pcID := int64(pc["id"].(float64))

	// Add a component.
	addBody := fmt.Sprintf(`{"component_id":%d,"portions":2}`, f.compID)
	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/plates/%d/components", plateID), bytes.NewBufferString(addBody)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var added map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&added))
	addedID := int64(added["id"].(float64))

	// Swap (component_id) the first component.
	swapBody := fmt.Sprintf(`{"component_id":%d}`, f.compID)
	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/plates/%d/components/%d", plateID, pcID), bytes.NewBufferString(swapBody)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	// Update portions.
	rescaleBody := `{"portions":3}`
	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/plates/%d/components/%d", plateID, addedID), bytes.NewBufferString(rescaleBody)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	// Remove the added component.
	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/plates/%d/components/%d", plateID, addedID), nil))
	require.Equal(t, http.StatusNoContent, resp.Code)

	// Delete plate.
	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/plates/%d", plateID), nil))
	require.Equal(t, http.StatusNoContent, resp.Code)
}

func TestWeeks_CopyWeek(t *testing.T) {
	f := setupPlannerRouter(t)
	week := getCurrentWeek(t, f.router)
	weekID := int64(week["id"].(float64))

	createBody := fmt.Sprintf(`{"day":2,"slot_id":%d,"components":[{"component_id":%d,"portions":1}]}`, f.slotID, f.compID)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/weeks/%d/plates", weekID), bytes.NewBufferString(createBody)))
	require.Equal(t, http.StatusCreated, resp.Code)

	// Pick a target year/week distinct from current.
	currentYear := int(week["year"].(float64))
	targetWeek := int(week["week_number"].(float64)) + 1
	if targetWeek > 52 {
		targetWeek = 1
	}
	copyBody := fmt.Sprintf(`{"target_year":%d,"target_week":%d}`, currentYear, targetWeek)
	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/weeks/%d/copy", weekID), bytes.NewBufferString(copyBody)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var target map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&target))
	plates := target["plates"].([]any)
	assert.Len(t, plates, 1, "copy preserves plate count")
}

func TestWeeks_ShoppingList_NotFound(t *testing.T) {
	f := setupPlannerRouter(t)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/9999/shopping-list", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestWeeks_Nutrition_NotFound(t *testing.T) {
	f := setupPlannerRouter(t)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/9999/nutrition", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestWeeks_ShoppingList_EmptyWeek(t *testing.T) {
	f := setupPlannerRouter(t)
	week := getCurrentWeek(t, f.router)
	weekID := int64(week["id"].(float64))

	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/weeks/%d/shopping-list", weekID), nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	items := got["items"].([]any)
	assert.Empty(t, items)
}

func TestWeeks_ShoppingList_WithPlate(t *testing.T) {
	f := setupPlannerRouter(t)
	week := getCurrentWeek(t, f.router)
	weekID := int64(week["id"].(float64))

	// Create a plate with the fixture component (Curry, 1 ref portion, 100g Chicken@100kcal/100g).
	body := fmt.Sprintf(`{"day":1,"slot_id":%d,"components":[{"component_id":%d,"portions":1}]}`, f.slotID, f.compID)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/weeks/%d/plates", weekID), bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/weeks/%d/shopping-list", weekID), nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	items := got["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, "Chicken", item["name"])
	assert.InDelta(t, 100.0, item["total_grams"], 0.01)
}

func TestWeeks_Nutrition_EmptyWeek(t *testing.T) {
	f := setupPlannerRouter(t)
	week := getCurrentWeek(t, f.router)
	weekID := int64(week["id"].(float64))

	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/weeks/%d/nutrition", weekID), nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	days := got["days"].([]any)
	assert.Empty(t, days)
	weekMacros := got["week"].(map[string]any)
	assert.InDelta(t, 0.0, weekMacros["kcal"], 0.01)
}

func TestWeeks_Nutrition_WithPlate(t *testing.T) {
	f := setupPlannerRouter(t)
	week := getCurrentWeek(t, f.router)
	weekID := int64(week["id"].(float64))

	// Component: Curry, 1 ref portion, 100g Chicken @ 100 kcal/100g → 100 kcal/portion.
	body := fmt.Sprintf(`{"day":1,"slot_id":%d,"components":[{"component_id":%d,"portions":1}]}`, f.slotID, f.compID)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/weeks/%d/plates", weekID), bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	resp = httptest.NewRecorder()
	f.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/weeks/%d/nutrition", weekID), nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	days := got["days"].([]any)
	require.Len(t, days, 1)
	day := days[0].(map[string]any)
	assert.InDelta(t, 1.0, day["day"], 0)
	macros := day["macros"].(map[string]any)
	assert.InDelta(t, 100.0, macros["kcal"], 0.01)
	weekMacros := got["week"].(map[string]any)
	assert.InDelta(t, 100.0, weekMacros["kcal"], 0.01)
}

func getCurrentWeek(t *testing.T, r http.Handler) map[string]any {
	t.Helper()
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/current", nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return got
}
