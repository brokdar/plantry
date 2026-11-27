package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

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

// FixNutrition puts catalogue or hand-entered nutrition on an existing leaf
// (the "Needs your check" fix). Unlike Update it requires all four core
// macros (as MATCH-2 does for a match), never replaces a known nutrient
// with null, and keeps the leaf's name, role and other unsent fields. The
// source, barcode and catalogue ids follow the new nutrition together.
func (h *FoodHandler) FixNutrition(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req foodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	if existing.Kind != food.KindLeaf {
		writeError(w, http.StatusUnprocessableEntity, "error.kind_immutable")
		return
	}
	req.Kind = string(food.KindLeaf)
	req.Name = existing.Name
	f := requestToFood(&req)
	if f.Kcal100g == nil || f.Protein100g == nil || f.Fat100g == nil || f.Carbs100g == nil {
		writeError(w, http.StatusUnprocessableEntity, "error.invalid_body")
		return
	}
	f.ID = id
	f.ImagePath = existing.ImagePath
	f.SourceURL = existing.SourceURL
	f.Description = existing.Description
	f.Role = existing.Role
	f.Favorite = existing.Favorite
	for _, n := range [][2]**float64{
		{&f.Fiber100g, &existing.Fiber100g},
		{&f.Sodium100g, &existing.Sodium100g},
		{&f.SaturatedFat100g, &existing.SaturatedFat100g},
		{&f.TransFat100g, &existing.TransFat100g},
		{&f.Cholesterol100g, &existing.Cholesterol100g},
		{&f.Sugar100g, &existing.Sugar100g},
		{&f.Potassium100g, &existing.Potassium100g},
		{&f.Calcium100g, &existing.Calcium100g},
		{&f.Iron100g, &existing.Iron100g},
		{&f.Magnesium100g, &existing.Magnesium100g},
		{&f.Phosphorus100g, &existing.Phosphorus100g},
		{&f.Zinc100g, &existing.Zinc100g},
		{&f.VitaminA100g, &existing.VitaminA100g},
		{&f.VitaminC100g, &existing.VitaminC100g},
		{&f.VitaminD100g, &existing.VitaminD100g},
		{&f.VitaminB12100g, &existing.VitaminB12100g},
		{&f.VitaminB6100g, &existing.VitaminB6100g},
		{&f.Folate100g, &existing.Folate100g},
	} {
		if *n[0] == nil {
			*n[0] = *n[1]
		}
	}
	if err := h.svc.Update(r.Context(), f); err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
