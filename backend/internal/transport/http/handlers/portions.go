package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

type portionRequest struct {
	Unit  string  `json:"unit"`
	Grams float64 `json:"grams"`
}

type portionResponse struct {
	IngredientID int64   `json:"ingredient_id"`
	Unit         string  `json:"unit"`
	Grams        float64 `json:"grams"`
}

// ListPortions handles GET /api/ingredients/{id}/portions
func (h *IngredientHandler) ListPortions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	portions, err := h.svc.ListPortions(r.Context(), id)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	resp := make([]portionResponse, len(portions))
	for i, p := range portions {
		resp[i] = portionResponse{
			IngredientID: p.IngredientID,
			Unit:         p.Unit,
			Grams:        p.Grams,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// UpsertPortion handles POST /api/ingredients/{id}/portions
func (h *IngredientHandler) UpsertPortion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	var req portionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	p := &ingredient.Portion{
		IngredientID: id,
		Unit:         req.Unit,
		Grams:        req.Grams,
	}

	if err := h.svc.UpsertPortion(r.Context(), p); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusCreated, portionResponse{
		IngredientID: p.IngredientID,
		Unit:         p.Unit,
		Grams:        p.Grams,
	})
}

// DeletePortion handles DELETE /api/ingredients/{id}/portions/{unit}
func (h *IngredientHandler) DeletePortion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	unit := chi.URLParam(r, "unit")

	if err := h.svc.DeletePortion(r.Context(), id, unit); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
