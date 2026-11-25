package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
)

// FoodEventHandler serves the undo of food changes.
type FoodEventHandler struct {
	svc *foodevent.Service
}

// NewFoodEventHandler creates a FoodEventHandler.
func NewFoodEventHandler(svc *foodevent.Service) *FoodEventHandler {
	return &FoodEventHandler{svc: svc}
}

// Revert handles POST /api/food-events/revert with an undo token
// {first_event_id, last_event_id} and returns {subjects: [{kind,
// subject_id}]}. A stale token is 409 error.plan.change_stale.
func (h *FoodEventHandler) Revert(w http.ResponseWriter, r *http.Request) {
	var u foodevent.Undo
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil || u.FirstEventID <= 0 || u.LastEventID < u.FirstEventID {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	subjects, err := h.svc.Revert(r.Context(), u)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subjects": subjects})
}
