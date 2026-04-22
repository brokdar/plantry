package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type feedbackRouterFixture struct {
	router  http.Handler
	plateID int64
}

func setupFeedbackRouter(t *testing.T) *feedbackRouterFixture {
	t.Helper()
	db := testhelper.NewTestDB(t)

	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	weekRepo := sqlite.NewWeekRepo(db)
	tx := sqlite.NewTxRunner(db)

	plateSvc := plate.NewService(plateRepo, slotRepo, compRepo)
	feedbackSvc := feedback.NewService(tx, plateRepo, compRepo)

	fh := handlers.NewFeedbackHandler(feedbackSvc)

	r := chi.NewRouter()
	r.Route("/api/plates/{id}", func(r chi.Router) {
		r.Put("/feedback", fh.Put)
		r.Delete("/feedback", fh.Delete)
	})

	ctx := context.Background()
	w := &planner.Week{Year: 2026, WeekNumber: 16}
	require.NoError(t, weekRepo.Create(ctx, w))
	s := &slot.TimeSlot{NameKey: "slot.dinner", Icon: "Moon", SortOrder: 1, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))
	ing := &ingredient.Ingredient{Name: "Chicken", Source: "manual", Kcal100g: 100}
	require.NoError(t, ingRepo.Create(ctx, ing))
	c := &component.Component{
		Name: "Curry", Role: component.RoleMain, ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
		Tags: []string{"spicy"},
	}
	require.NoError(t, compRepo.Create(ctx, c))
	pl := &plate.Plate{WeekID: w.ID, Day: 0, SlotID: s.ID}
	require.NoError(t, plateRepo.Create(ctx, pl))
	_, err := plateSvc.AddComponent(ctx, pl.ID, c.ID, 1)
	require.NoError(t, err)

	return &feedbackRouterFixture{router: r, plateID: pl.ID}
}

func putFeedback(t *testing.T, router http.Handler, plateID int64, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut,
		"/api/plates/"+itoa(plateID)+"/feedback",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestFeedback_Put_HappyPath(t *testing.T) {
	f := setupFeedbackRouter(t)

	resp := putFeedback(t, f.router, f.plateID, []byte(`{"status":"loved","note":"yum"}`))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, "loved", body["status"])
	assert.Equal(t, "yum", body["note"])
	assert.NotEmpty(t, body["rated_at"])
}

func TestFeedback_Put_ReplacesExisting(t *testing.T) {
	f := setupFeedbackRouter(t)

	resp := putFeedback(t, f.router, f.plateID, []byte(`{"status":"cooked"}`))
	require.Equal(t, http.StatusOK, resp.Code)

	resp = putFeedback(t, f.router, f.plateID, []byte(`{"status":"disliked"}`))
	require.Equal(t, http.StatusOK, resp.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, "disliked", body["status"])
}

func TestFeedback_Put_InvalidStatusReturns422(t *testing.T) {
	f := setupFeedbackRouter(t)

	resp := putFeedback(t, f.router, f.plateID, []byte(`{"status":"bogus"}`))
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, "error.plate.feedback_invalid_status", body["message_key"])
}

func TestFeedback_Put_MalformedBodyReturns400(t *testing.T) {
	f := setupFeedbackRouter(t)

	resp := putFeedback(t, f.router, f.plateID, []byte(`{`))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestFeedback_Put_UnknownPlateReturns404(t *testing.T) {
	f := setupFeedbackRouter(t)

	resp := putFeedback(t, f.router, 9999, []byte(`{"status":"cooked"}`))
	assert.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())
}

func TestFeedback_Put_NonNumericIDReturns400(t *testing.T) {
	f := setupFeedbackRouter(t)

	req := httptest.NewRequest(http.MethodPut, "/api/plates/abc/feedback", bytes.NewReader([]byte(`{"status":"cooked"}`)))
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestFeedback_Delete_HappyPath(t *testing.T) {
	f := setupFeedbackRouter(t)

	putResp := putFeedback(t, f.router, f.plateID, []byte(`{"status":"cooked"}`))
	require.Equal(t, http.StatusOK, putResp.Code)

	req := httptest.NewRequest(http.MethodDelete, "/api/plates/"+itoa(f.plateID)+"/feedback", nil)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestFeedback_Delete_MissingReturns404(t *testing.T) {
	f := setupFeedbackRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/plates/"+itoa(f.plateID)+"/feedback", nil)
	resp := httptest.NewRecorder()
	f.router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestWeekResponse_EmbedsFeedback(t *testing.T) {
	db := testhelper.NewTestDB(t)

	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	weekRepo := sqlite.NewWeekRepo(db)
	fbRepo := sqlite.NewFeedbackRepo(db)
	tx := sqlite.NewTxRunner(db)

	plateSvc := plate.NewService(plateRepo, slotRepo, compRepo)
	plannerSvc := planner.NewService(weekRepo, plateRepo, tx)
	componentSvc := component.NewService(compRepo, ingRepo, ingRepo)
	feedbackSvc := feedback.NewService(tx, plateRepo, compRepo)

	wh := handlers.NewWeekHandler(plannerSvc, plateSvc, componentSvc, ingRepo, fbRepo)
	fh := handlers.NewFeedbackHandler(feedbackSvc)

	r := chi.NewRouter()
	r.Route("/api/weeks", func(r chi.Router) {
		r.Get("/current", wh.Current)
		r.Get("/{id}", wh.Get)
		r.Post("/{id}/plates", wh.CreatePlate)
	})
	r.Route("/api/plates/{id}", func(r chi.Router) {
		r.Put("/feedback", fh.Put)
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

	// Create a week + plate.
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/current", nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var week map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&week))
	weekID := int64(week["id"].(float64))

	body := []byte(`{"day":0,"slot_id":` + itoa(s.ID) + `,"components":[{"component_id":` + itoa(c.ID) + `,"portions":1}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/weeks/"+itoa(weekID)+"/plates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	plateID := int64(created["id"].(float64))

	// Before feedback — the plate's `feedback` field is absent.
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/"+itoa(weekID), nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var w0 map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&w0))
	p0 := w0["plates"].([]any)[0].(map[string]any)
	_, hasFeedback := p0["feedback"]
	assert.False(t, hasFeedback)

	// Record feedback.
	req = httptest.NewRequest(http.MethodPut, "/api/plates/"+itoa(plateID)+"/feedback",
		bytes.NewReader([]byte(`{"status":"loved","note":"yum"}`)))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	// After feedback — the week GET embeds it on the plate.
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/weeks/"+itoa(weekID), nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var w1 map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&w1))
	p1 := w1["plates"].([]any)[0].(map[string]any)
	fbMap, ok := p1["feedback"].(map[string]any)
	require.True(t, ok, "plate should now embed feedback")
	assert.Equal(t, "loved", fbMap["status"])
	assert.Equal(t, "yum", fbMap["note"])
	assert.Equal(t, float64(plateID), fbMap["plate_id"])
}
