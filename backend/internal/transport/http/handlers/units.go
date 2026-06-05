package handlers

import (
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// unitResponse is the wire shape for a single unit descriptor.
type unitResponse struct {
	ID          string  `json:"id"`
	Group       string  `json:"group"`
	Grams       float64 `json:"grams,omitempty"`
	Approximate bool    `json:"approximate,omitempty"`
}

// massOrder and volumeOrder preserve the canonical display order that the
// frontend uses for grouped pickers. countOrder mirrors COUNT_UNITS_ORDERED.
var (
	massOrder   = []string{"g", "kg", "mg", "oz", "lb"}
	volumeOrder = []string{"ml", "l", "cl", "dl", "tbsp", "tsp", "cup", "floz"}
)

// countOrder is the canonical display order for count units (mirrors the
// frontend COUNT_UNITS_ORDERED constant).
var countOrder = []string{
	"piece", "clove", "slice", "bunch", "pinch",
	"stick", "can", "jar", "packet", "serving",
	"stalk", "pod", "head", "leaf",
}

// Units handles GET /units and returns the canonical unit vocabulary.
func Units(w http.ResponseWriter, _ *http.Request) {
	resp := make([]unitResponse, 0, len(massOrder)+len(volumeOrder)+len(countOrder))

	for _, id := range massOrder {
		d := units.Defaults[id]
		resp = append(resp, unitResponse{
			ID:    id,
			Group: "mass",
			Grams: d.Grams,
		})
	}

	for _, id := range volumeOrder {
		d := units.Defaults[id]
		resp = append(resp, unitResponse{
			ID:          id,
			Group:       "volume",
			Grams:       d.Grams,
			Approximate: d.Approximate,
		})
	}

	for _, id := range countOrder {
		resp = append(resp, unitResponse{
			ID:    id,
			Group: "count",
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
