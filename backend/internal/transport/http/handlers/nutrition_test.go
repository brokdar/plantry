package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// stubNutritionRangeSvc satisfies the unexported plateRangeService interface.
type stubNutritionRangeSvc struct {
	fn func(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

func (s *stubNutritionRangeSvc) Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	if s.fn != nil {
		return s.fn(ctx, from, to)
	}
	return nil, nil
}

// newValidationNutritionRouter builds a handler with a stub plate service and a
// nil resolver — suitable only for validation tests that return before resolver use.
func newValidationNutritionRouter() http.Handler {
	svc := &stubNutritionRangeSvc{}
	h := handlers.NewNutritionRangeHandlerFromService(svc, nil)
	r := chi.NewRouter()
	r.Get("/api/nutrition", h.List)
	return r
}

// newNutritionRouterWithDB wires real repos backed by testhelper.NewTestDB.
func newNutritionRouterWithDB(t *testing.T) (http.Handler, *sqlite.FoodRepo, *sqlite.PlateRepo, *sqlite.SlotRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	foodRepo := sqlite.NewFoodRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	resolver := food.NewNutritionResolver(foodRepo)
	plateSvc := plate.NewService(plateRepo, slotRepo, foodRepo)
	h := handlers.NewNutritionRangeHandler(plateSvc, resolver)
	r := chi.NewRouter()
	r.Get("/api/nutrition", h.List)
	return r, foodRepo, plateRepo, slotRepo
}

// ── validation tests ──────────────────────────────────────────────────────────

func TestNutritionRange_400_MissingFrom(t *testing.T) {
	r := newValidationNutritionRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nutrition?to=2026-04-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertNutritionMessageKey(t, w, "error.invalid_from")
}

func TestNutritionRange_400_MissingTo(t *testing.T) {
	r := newValidationNutritionRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nutrition?from=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertNutritionMessageKey(t, w, "error.invalid_to")
}

func TestNutritionRange_400_MalformedFrom(t *testing.T) {
	r := newValidationNutritionRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nutrition?from=not-a-date&to=2026-04-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertNutritionMessageKey(t, w, "error.invalid_from")
}

func TestNutritionRange_400_MalformedTo(t *testing.T) {
	r := newValidationNutritionRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nutrition?from=2026-04-26&to=bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertNutritionMessageKey(t, w, "error.invalid_to")
}

func TestNutritionRange_400_FromAfterTo(t *testing.T) {
	r := newValidationNutritionRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nutrition?from=2026-05-01&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertNutritionMessageKey(t, w, "error.invalid_date_range")
}

func TestNutritionRange_400_SpanTooLarge(t *testing.T) {
	r := newValidationNutritionRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nutrition?from=2024-01-01&to=2025-12-31", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertNutritionMessageKey(t, w, "error.date_range_too_large")
}

// ── happy path tests (real DB) ────────────────────────────────────────────────

func TestNutritionRange_200_EmptyRange(t *testing.T) {
	r, _, _, _ := newNutritionRouterWithDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/nutrition?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	days, ok := body["days"].([]any)
	require.True(t, ok)
	assert.Empty(t, days)
}

func TestNutritionRange_200_HappyPath(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newNutritionRouterWithDB(t)
	ctx := context.Background()

	// Seed a slot (FK required by plates table).
	s := &slot.TimeSlot{NameKey: "dinner", Icon: "🍽", SortOrder: 0, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	// Seed a leaf food with known macros. Leaf foods require Source != nil.
	src := food.SourceManual
	kcal := 400.0
	f := &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src, Kcal100g: &kcal}
	require.NoError(t, foodRepo.Create(ctx, f))

	// Seed plates on two different dates.
	d1, _ := time.Parse("2006-01-02", "2026-04-26")
	d2, _ := time.Parse("2006-01-02", "2026-04-27")
	p1 := &plate.Plate{
		Date: d1, SlotID: s.ID,
		Components: []plate.PlateComponent{{FoodID: f.ID, Portions: 1}},
	}
	p2 := &plate.Plate{
		Date: d2, SlotID: s.ID,
		Components: []plate.PlateComponent{{FoodID: f.ID, Portions: 2}},
	}
	require.NoError(t, plateRepo.Create(ctx, p1))
	require.NoError(t, plateRepo.Create(ctx, p2))

	req := httptest.NewRequest(http.MethodGet, "/api/nutrition?from=2026-04-26&to=2026-04-27", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	days, ok := body["days"].([]any)
	require.True(t, ok)
	require.Len(t, days, 2)

	// Days should be sorted by date string.
	day0 := days[0].(map[string]any)
	day1 := days[1].(map[string]any)
	assert.Equal(t, "2026-04-26", day0["date"])
	assert.Equal(t, "2026-04-27", day1["date"])

	// Verify macros key is present.
	_, hasMacros := day0["macros"]
	assert.True(t, hasMacros)
}

// ── helper ────────────────────────────────────────────────────────────────────

func assertNutritionMessageKey(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, want, body["message_key"])
}
