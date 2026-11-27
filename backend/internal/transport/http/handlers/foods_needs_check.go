package handlers

import (
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

type needsCheckLineDTO struct {
	LeafID   int64   `json:"leaf_id"`
	LeafName string  `json:"leaf_name"`
	Amount   float64 `json:"amount"`
	Unit     string  `json:"unit"`
}

type needsCheckRecipeDTO struct {
	ID    int64               `json:"id"`
	Name  string              `json:"name"`
	Lines []needsCheckLineDTO `json:"lines,omitempty"`
}

type needsCheckLeafDTO struct {
	ID      int64                 `json:"id"`
	Name    string                `json:"name"`
	Recipes []needsCheckRecipeDTO `json:"recipes"`
}

type needsCheckResponse struct {
	// Count is the number of recipes waiting; the Recipes screen shows it.
	Count   int                   `json:"count"`
	Leaves  []needsCheckLeafDTO   `json:"leaves"`
	Recipes []needsCheckRecipeDTO `json:"recipes"`
}

func needsCheckRecipes(in []food.NeedsCheckRecipe, withLines bool) []needsCheckRecipeDTO {
	out := make([]needsCheckRecipeDTO, len(in))
	for i, r := range in {
		out[i] = needsCheckRecipeDTO{ID: r.ID, Name: r.Name}
		if !withLines {
			continue
		}
		for _, l := range r.Lines {
			out[i].Lines = append(out[i].Lines, needsCheckLineDTO{
				LeafID: l.LeafID, LeafName: l.LeafName, Amount: l.Amount, Unit: l.Unit,
			})
		}
	}
	return out
}

// NeedsCheck handles GET /api/foods/needs-check: leaves without nutrition
// that recipes use, and the recipes with such a line (legacy data).
func (h *FoodHandler) NeedsCheck(w http.ResponseWriter, r *http.Request) {
	nc, err := h.svc.NeedsCheck(r.Context())
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	resp := needsCheckResponse{
		Count:   len(nc.Recipes),
		Leaves:  make([]needsCheckLeafDTO, len(nc.Leaves)),
		Recipes: needsCheckRecipes(nc.Recipes, true),
	}
	for i, l := range nc.Leaves {
		resp.Leaves[i] = needsCheckLeafDTO{ID: l.ID, Name: l.Name, Recipes: needsCheckRecipes(l.Recipes, false)}
	}
	writeJSON(w, http.StatusOK, resp)
}
