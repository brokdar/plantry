package handlers

import (
	"context"
	"fmt"
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
	plateMacros, err := computePlateMacros(r.Context(), h.resolver, plates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	dayMacros := map[string]nutrition.Macros{}
	for _, pl := range plates {
		dateStr := pl.DateString()
		m := plateMacros[pl.ID]
		d := dayMacros[dateStr]
		d.Kcal += m.Kcal
		d.Protein += m.Protein
		d.Fat += m.Fat
		d.Carbs += m.Carbs
		d.Fiber += m.Fiber
		d.Sodium += m.Sodium
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

// computePlateMacros aggregates per-plate macros for the given plates using
// the kind-aware quantity model:
//
//   - composed component: multiplier = portions, macros from PerPortion (which
//     returns per-portion macros for composed foods).
//   - leaf component: multiplier = grams / 100, macros from PerPortion (which
//     returns per-100g macros for leaf foods).
//
// PerPortion is called once per unique food_id within the request.
func computePlateMacros(ctx context.Context, resolver *food.NutritionResolver, plates []plate.Plate) (map[int64]nutrition.Macros, error) {
	perPortion := map[int64]nutrition.Macros{}
	for _, pl := range plates {
		for _, pc := range pl.Components {
			if _, ok := perPortion[pc.FoodID]; ok {
				continue
			}
			m, err := resolver.PerPortion(ctx, pc.FoodID)
			if err != nil {
				return nil, fmt.Errorf("resolve food %d: %w", pc.FoodID, err)
			}
			perPortion[pc.FoodID] = m
		}
	}

	out := make(map[int64]nutrition.Macros, len(plates))
	for _, pl := range plates {
		var total nutrition.Macros
		for _, pc := range pl.Components {
			m := perPortion[pc.FoodID]
			multiplier := pc.Multiplier()
			total.Kcal += m.Kcal * multiplier
			total.Protein += m.Protein * multiplier
			total.Fat += m.Fat * multiplier
			total.Carbs += m.Carbs * multiplier
			total.Fiber += m.Fiber * multiplier
			total.Sodium += m.Sodium * multiplier
		}
		out[pl.ID] = total
	}
	return out, nil
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
