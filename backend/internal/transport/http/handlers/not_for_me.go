package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// NotForMeHandler serves "Not for me" on a library recipe (NEW-4): the
// recipe is archived and the reason teaches the taste model.
type NotForMeHandler struct {
	foods *food.Service
	taste *taste.Service
}

// NewNotForMeHandler creates a NotForMeHandler.
func NewNotForMeHandler(foods *food.Service, t *taste.Service) *NotForMeHandler {
	return &NotForMeHandler{foods: foods, taste: t}
}

// Post handles POST /api/foods/{id}/not-for-me {reason}: {undo}, the Undo
// token of the archive (reverted by POST /api/food-events/revert).
func (h *NotForMeHandler) Post(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	switch req.Reason {
	case taste.ReasonTaste, taste.ReasonIngredients, taste.ReasonTooLong, "looks_wrong":
	default:
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	_, undo, err := h.foods.Archive(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	if err := h.taste.NotForMe(r.Context(), id, req.Reason, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, map[string]foodevent.Undo{"undo": undo})
}

// TrialSeen handles POST /api/foods/{id}/trial-seen: opening a trial recipe
// clears its To try badge, not the trial (NEW-3). 204.
func (h *NotForMeHandler) TrialSeen(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.foods.MarkTrialSeen(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
