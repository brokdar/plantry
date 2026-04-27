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
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// ── stub for plateRangeService ────────────────────────────────────────────────

type stubShoppingRangeSvc struct {
	fn func(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

func (s *stubShoppingRangeSvc) Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	if s.fn != nil {
		return s.fn(ctx, from, to)
	}
	return nil, nil
}

// ── router helpers ────────────────────────────────────────────────────────────

// newValidationShoppingRouter builds a handler with a stub plate service and a
// nil shopping resolver. Suitable only for validation tests where parseDateRange
// returns before the resolver is ever called.
func newValidationShoppingRouter() http.Handler {
	svc := &stubShoppingRangeSvc{}
	h := handlers.NewShoppingRangeHandlerFromService(svc, nil)
	r := chi.NewRouter()
	r.Get("/api/shopping-list", h.List)
	return r
}

// newShoppingRouterWithDB wires real repos backed by testhelper.NewTestDB.
// Returns the handler, food repo, plate repo, and slot repo so tests can seed data.
func newShoppingRouterWithDB(t *testing.T) (http.Handler, *sqlite.FoodRepo, *sqlite.PlateRepo, *sqlite.SlotRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	foodRepo := sqlite.NewFoodRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	resolver := shopping.NewResolver(foodRepo)
	plateSvc := plate.NewService(plateRepo, slotRepo, foodRepo)
	h := handlers.NewShoppingRangeHandler(plateSvc, resolver)
	r := chi.NewRouter()
	r.Get("/api/shopping-list", h.List)
	return r, foodRepo, plateRepo, slotRepo
}

// ── validation tests ──────────────────────────────────────────────────────────

func TestShoppingRange_400_MissingFrom(t *testing.T) {
	r := newValidationShoppingRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?to=2026-04-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertShoppingMessageKey(t, w, "error.invalid_from")
}

func TestShoppingRange_400_MissingTo(t *testing.T) {
	r := newValidationShoppingRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?from=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertShoppingMessageKey(t, w, "error.invalid_to")
}

func TestShoppingRange_400_MalformedFrom(t *testing.T) {
	r := newValidationShoppingRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?from=not-a-date&to=2026-04-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertShoppingMessageKey(t, w, "error.invalid_from")
}

func TestShoppingRange_400_MalformedTo(t *testing.T) {
	r := newValidationShoppingRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?from=2026-04-26&to=bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertShoppingMessageKey(t, w, "error.invalid_to")
}

func TestShoppingRange_400_FromAfterTo(t *testing.T) {
	r := newValidationShoppingRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?from=2026-05-01&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertShoppingMessageKey(t, w, "error.invalid_date_range")
}

func TestShoppingRange_400_SpanTooLarge(t *testing.T) {
	r := newValidationShoppingRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?from=2024-01-01&to=2025-12-31", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertShoppingMessageKey(t, w, "error.date_range_too_large")
}

// ── happy path tests (real DB) ────────────────────────────────────────────────

func TestShoppingRange_200_EmptyRange(t *testing.T) {
	r, _, _, _ := newShoppingRouterWithDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	items, ok := body["items"].([]any)
	require.True(t, ok)
	assert.Empty(t, items)
}

func TestShoppingRange_200_HappyPath(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newShoppingRouterWithDB(t)
	ctx := context.Background()

	// Seed a slot (FK required by plates table).
	s := &slot.TimeSlot{NameKey: "dinner", Icon: "🍽", SortOrder: 0, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	// Seed two leaf foods. Leaf foods require Source != nil per DB constraint.
	src := food.SourceManual
	f1 := &food.Food{Name: "Apple", Kind: food.KindLeaf, Source: &src}
	f2 := &food.Food{Name: "Zebra Fruit", Kind: food.KindLeaf, Source: &src}
	require.NoError(t, foodRepo.Create(ctx, f1))
	require.NoError(t, foodRepo.Create(ctx, f2))

	// Seed a plate.
	d, _ := time.Parse("2006-01-02", "2026-04-26")
	p := &plate.Plate{
		Date:   d,
		SlotID: s.ID,
		Components: []plate.PlateComponent{
			{FoodID: f1.ID, Portions: 1},
			{FoodID: f2.ID, Portions: 2},
		},
	}
	require.NoError(t, plateRepo.Create(ctx, p))

	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	items, ok := body["items"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestShoppingRange_200_ItemsAlphabetical(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newShoppingRouterWithDB(t)
	ctx := context.Background()

	s := &slot.TimeSlot{NameKey: "lunch", Icon: "🥗", SortOrder: 0, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	// Seed foods in reverse alphabetical order to verify sorting.
	src := food.SourceManual
	zebra := &food.Food{Name: "Zebra Fruit", Kind: food.KindLeaf, Source: &src}
	apple := &food.Food{Name: "Apple", Kind: food.KindLeaf, Source: &src}
	require.NoError(t, foodRepo.Create(ctx, zebra))
	require.NoError(t, foodRepo.Create(ctx, apple))

	d, _ := time.Parse("2006-01-02", "2026-04-26")
	p := &plate.Plate{
		Date:   d,
		SlotID: s.ID,
		Components: []plate.PlateComponent{
			{FoodID: zebra.ID, Portions: 1},
			{FoodID: apple.ID, Portions: 1},
		},
	}
	require.NoError(t, plateRepo.Create(ctx, p))

	req := httptest.NewRequest(http.MethodGet, "/api/shopping-list?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	items := body["items"].([]any)
	require.Len(t, items, 2)
	first := items[0].(map[string]any)["name"].(string)
	second := items[1].(map[string]any)["name"].(string)
	assert.Less(t, first, second, "items should be alphabetically ordered")
}

// ── helper ────────────────────────────────────────────────────────────────────

func assertShoppingMessageKey(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, want, body["message_key"])
}
