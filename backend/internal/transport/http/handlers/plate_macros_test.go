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

func newPlateMacrosRouter(t *testing.T) (http.Handler, *sqlite.FoodRepo, *sqlite.PlateRepo, *sqlite.SlotRepo) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	foodRepo := sqlite.NewFoodRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	resolver := food.NewNutritionResolver(foodRepo)
	plateSvc := plate.NewService(plateRepo, slotRepo, foodRepo)
	h := handlers.NewPlateMacrosHandler(plateSvc, resolver)
	r := chi.NewRouter()
	r.Get("/api/plates/macros", h.List)
	return r, foodRepo, plateRepo, slotRepo
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }
func ptrStr(v string) *string       { return &v }

func TestPlateMacros_EmptyRange(t *testing.T) {
	r, _, _, _ := newPlateMacrosRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/plates/macros?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	plates, ok := body["plates"].([]any)
	require.True(t, ok)
	assert.Empty(t, plates)
}

func TestPlateMacros_ComposedComponent_KcalMatchesPerPortion(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newPlateMacrosRouter(t)
	ctx := context.Background()

	// Seed slot.
	s := &slot.TimeSlot{NameKey: "dinner", Icon: "🍽", SortOrder: 0, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	// Leaf: rice 130 kcal/100g.
	src := food.SourceManual
	rice := &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(130)}
	require.NoError(t, foodRepo.Create(ctx, rice))

	// Composed: 1 portion of curry uses 200g rice → 260 kcal/portion.
	role := food.RoleMain
	ref := float64(1)
	curry := &food.Food{
		Name:              "Curry",
		Kind:              food.KindComposed,
		Role:              &role,
		ReferencePortions: &ref,
		Children: []food.FoodComponent{{
			ChildID: rice.ID, Amount: 200, Unit: "g", Grams: 200,
		}},
	}
	require.NoError(t, foodRepo.Create(ctx, curry))

	d, _ := time.Parse("2006-01-02", "2026-04-26")
	p := &plate.Plate{
		Date: d, SlotID: s.ID,
		Components: []plate.PlateComponent{{FoodID: curry.ID, Portions: ptrInt(1)}},
	}
	require.NoError(t, plateRepo.Create(ctx, p))

	req := httptest.NewRequest(http.MethodGet, "/api/plates/macros?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	plates := body["plates"].([]any)
	require.Len(t, plates, 1)
	first := plates[0].(map[string]any)
	macros := first["macros"].(map[string]any)
	// 1 portion × (200g × 1.30 kcal/g) = 260 kcal.
	assert.InDelta(t, 260.0, macros["kcal"].(float64), 0.5)
	assert.Equal(t, false, first["skipped"].(bool))
}

func TestPlateMacros_LeafComponent_KcalScalesWithGrams(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newPlateMacrosRouter(t)
	ctx := context.Background()

	s := &slot.TimeSlot{NameKey: "dinner", Icon: "🍽", SortOrder: 0, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	src := food.SourceManual
	rice := &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(130)}
	require.NoError(t, foodRepo.Create(ctx, rice))

	d, _ := time.Parse("2006-01-02", "2026-04-26")
	p := &plate.Plate{
		Date: d, SlotID: s.ID,
		Components: []plate.PlateComponent{{
			FoodID:      rice.ID,
			Amount:      ptrFloat64(200),
			Unit:        ptrStr("g"),
			Grams:       ptrFloat64(200),
			GramsSource: ptrStr("direct"),
		}},
	}
	require.NoError(t, plateRepo.Create(ctx, p))

	req := httptest.NewRequest(http.MethodGet, "/api/plates/macros?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	plates := body["plates"].([]any)
	require.Len(t, plates, 1)
	macros := plates[0].(map[string]any)["macros"].(map[string]any)
	// 200 g × 1.30 kcal/g = 260 kcal.
	assert.InDelta(t, 260.0, macros["kcal"].(float64), 0.5)
}

func TestPlateMacros_SkippedPlateIncluded(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newPlateMacrosRouter(t)
	ctx := context.Background()

	s := &slot.TimeSlot{NameKey: "dinner", Icon: "🍽", SortOrder: 0, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	src := food.SourceManual
	rice := &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(130)}
	require.NoError(t, foodRepo.Create(ctx, rice))

	d, _ := time.Parse("2006-01-02", "2026-04-26")
	p := &plate.Plate{Date: d, SlotID: s.ID}
	require.NoError(t, plateRepo.Create(ctx, p))
	_, err := plateRepo.SetSkipped(ctx, p.ID, true, nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/plates/macros?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	plates := body["plates"].([]any)
	require.Len(t, plates, 1)
	first := plates[0].(map[string]any)
	assert.Equal(t, true, first["skipped"].(bool))
	macros := first["macros"].(map[string]any)
	assert.InDelta(t, 0.0, macros["kcal"].(float64), 0.01)
}

func TestPlateMacros_ComposedAndLeaf_Sum(t *testing.T) {
	r, foodRepo, plateRepo, slotRepo := newPlateMacrosRouter(t)
	ctx := context.Background()

	s := &slot.TimeSlot{NameKey: "dinner", Icon: "🍽", SortOrder: 0, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	src := food.SourceManual
	rice := &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrFloat64(130)}
	require.NoError(t, foodRepo.Create(ctx, rice))

	role := food.RoleMain
	ref := float64(1)
	curry := &food.Food{
		Name: "Curry", Kind: food.KindComposed, Role: &role, ReferencePortions: &ref,
		Children: []food.FoodComponent{{ChildID: rice.ID, Amount: 200, Unit: "g", Grams: 200}},
	}
	require.NoError(t, foodRepo.Create(ctx, curry))

	d, _ := time.Parse("2006-01-02", "2026-04-26")
	p := &plate.Plate{
		Date: d, SlotID: s.ID,
		Components: []plate.PlateComponent{
			{FoodID: curry.ID, Portions: ptrInt(1)}, // 260 kcal
			{
				FoodID: rice.ID, Amount: ptrFloat64(100), Unit: ptrStr("g"),
				Grams: ptrFloat64(100), GramsSource: ptrStr("direct"),
			}, // 130 kcal
		},
	}
	require.NoError(t, plateRepo.Create(ctx, p))

	req := httptest.NewRequest(http.MethodGet, "/api/plates/macros?from=2026-04-26&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	plates := body["plates"].([]any)
	require.Len(t, plates, 1)
	macros := plates[0].(map[string]any)["macros"].(map[string]any)
	assert.InDelta(t, 390.0, macros["kcal"].(float64), 0.5)
}
