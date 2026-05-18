package handlers

import (
	"net/http"
	"sort"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// PlateMacrosHandler serves per-plate macro totals for a date range. Reuses
// the same per-plate aggregation as the day-totals endpoint.
type PlateMacrosHandler struct {
	plates   plateRangeService
	resolver *food.NutritionResolver
}

// NewPlateMacrosHandler wires a concrete plate.Service.
func NewPlateMacrosHandler(plates *plate.Service, resolver *food.NutritionResolver) *PlateMacrosHandler {
	return &PlateMacrosHandler{plates: plates, resolver: resolver}
}

// NewPlateMacrosHandlerFromService accepts any plateRangeService for tests.
func NewPlateMacrosHandlerFromService(plates plateRangeService, resolver *food.NutritionResolver) *PlateMacrosHandler {
	return &PlateMacrosHandler{plates: plates, resolver: resolver}
}

type plateMacrosEntry struct {
	PlateID int64          `json:"plate_id"`
	Date    string         `json:"date"`
	Skipped bool           `json:"skipped"`
	Macros  macrosResponse `json:"macros"`
}

type plateMacrosResponse struct {
	Plates []plateMacrosEntry `json:"plates"`
}

// List handles GET /api/plates/macros?from=YYYY-MM-DD&to=YYYY-MM-DD.
func (h *PlateMacrosHandler) List(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}
	plates, err := h.plates.Range(r.Context(), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	macros, err := computePlateMacros(r.Context(), h.resolver, plates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	out := make([]plateMacrosEntry, 0, len(plates))
	for _, pl := range plates {
		out = append(out, plateMacrosEntry{
			PlateID: pl.ID,
			Date:    pl.DateString(),
			Skipped: pl.Skipped,
			Macros:  toMacrosResponse(macros[pl.ID]),
		})
	}
	// Stable order: by date, then plate id.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].PlateID < out[j].PlateID
	})

	writeJSON(w, http.StatusOK, plateMacrosResponse{Plates: out})
}

// Compile-time assertion: plate.Plate must expose Skipped — guards against
// silent removal during refactors that would leave the JSON shape lying.
var _ = plate.Plate{}.Skipped
