package handlers

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// NutritionRangeHandler serves the date-range nutrition endpoint.
type NutritionRangeHandler struct {
	plates   plateRangeService
	resolver *food.NutritionResolver
}

// NewNutritionRangeHandler wires a concrete plate.Service.
func NewNutritionRangeHandler(plates *plate.Service, resolver *food.NutritionResolver) *NutritionRangeHandler {
	return &NutritionRangeHandler{plates: plates, resolver: resolver}
}

// NewNutritionRangeHandlerFromService accepts any plateRangeService; intended
// for tests that inject a stub.
func NewNutritionRangeHandlerFromService(plates plateRangeService, resolver *food.NutritionResolver) *NutritionRangeHandler {
	return &NutritionRangeHandler{plates: plates, resolver: resolver}
}

type nutritionDateDayResponse struct {
	Date   string         `json:"date"`
	Macros macrosResponse `json:"macros"`
}

type rangeNutritionResponse struct {
	Days []nutritionDateDayResponse `json:"days"`
}

// List handles GET /api/nutrition?from=YYYY-MM-DD&to=YYYY-MM-DD.
func (h *NutritionRangeHandler) List(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}
	plates, err := h.plates.Range(r.Context(), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	// Resolve per-portion macros once per unique food_id.
	perPortion := map[int64]nutrition.Macros{}
	for _, pl := range plates {
		for _, pc := range pl.Components {
			if _, ok := perPortion[pc.FoodID]; ok {
				continue
			}
			m, err := h.resolver.PerPortion(r.Context(), pc.FoodID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "error.server")
				return
			}
			perPortion[pc.FoodID] = m
		}
	}

	// Aggregate macros per date string.
	dayMacros := map[string]nutrition.Macros{}
	for _, pl := range plates {
		dateStr := pl.DateString()
		var plateMacros nutrition.Macros
		for _, pc := range pl.Components {
			m := perPortion[pc.FoodID]
			plateMacros.Kcal += m.Kcal * pc.Portions
			plateMacros.Protein += m.Protein * pc.Portions
			plateMacros.Fat += m.Fat * pc.Portions
			plateMacros.Carbs += m.Carbs * pc.Portions
			plateMacros.Fiber += m.Fiber * pc.Portions
			plateMacros.Sodium += m.Sodium * pc.Portions
		}
		d := dayMacros[dateStr]
		d.Kcal += plateMacros.Kcal
		d.Protein += plateMacros.Protein
		d.Fat += plateMacros.Fat
		d.Carbs += plateMacros.Carbs
		d.Fiber += plateMacros.Fiber
		d.Sodium += plateMacros.Sodium
		dayMacros[dateStr] = d
	}

	days := make([]nutritionDateDayResponse, 0, len(dayMacros))
	for dateStr, m := range dayMacros {
		days = append(days, nutritionDateDayResponse{
			Date:   dateStr,
			Macros: toMacrosResponse(m),
		})
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })

	writeJSON(w, http.StatusOK, rangeNutritionResponse{Days: days})
}

// plateRangeServiceFromFunc wraps a function as a plateRangeService.
// Used by tests to inject stubs without a real DB.
type plateRangeServiceFromFunc struct {
	fn func(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

func (s *plateRangeServiceFromFunc) Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	return s.fn(ctx, from, to)
}

// NewPlateRangeServiceFromFunc wraps a function as a plateRangeService.
// Exported so tests in handlers_test package can use it.
func NewPlateRangeServiceFromFunc(fn func(ctx context.Context, from, to time.Time) ([]plate.Plate, error)) plateRangeService {
	return &plateRangeServiceFromFunc{fn: fn}
}
