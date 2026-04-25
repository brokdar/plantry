package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// PlateHandler exposes plate + plate_component mutation endpoints.
type PlateHandler struct {
	svc *plate.Service
}

// NewPlateHandler creates a PlateHandler.
func NewPlateHandler(svc *plate.Service) *PlateHandler {
	return &PlateHandler{svc: svc}
}

type updatePlateRequest struct {
	Day     *int    `json:"day"`
	SlotID  *int64  `json:"slot_id"`
	Note    *string `json:"note"`
	NoteSet bool    `json:"-"`
}

func (r *updatePlateRequest) UnmarshalJSON(b []byte) error {
	type alias struct {
		Day    *int    `json:"day"`
		SlotID *int64  `json:"slot_id"`
		Note   *string `json:"note"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	r.Day = a.Day
	r.SlotID = a.SlotID
	r.Note = a.Note
	// Detect whether "note" key was present in the JSON.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	_, r.NoteSet = raw["note"]
	return nil
}

func plateError(err error) (int, string) {
	return toHTTPWithResource(err, "plate")
}

// Get handles GET /api/plates/{id}.
func (h *PlateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(p, nil))
}

// Update handles PUT /api/plates/{id}.
func (h *PlateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	var req updatePlateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.Day != nil {
		existing.Day = *req.Day
	}
	if req.SlotID != nil {
		existing.SlotID = *req.SlotID
	}
	if req.NoteSet {
		existing.Note = req.Note
	}
	if err := h.svc.Update(r.Context(), existing); err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(existing, nil))
}

// Delete handles DELETE /api/plates/{id}.
func (h *PlateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type addComponentRequest struct {
	FoodID   int64   `json:"food_id"`
	Portions float64 `json:"portions"`
}

// AddComponent handles POST /api/plates/{id}/components.
func (h *PlateHandler) AddComponent(w http.ResponseWriter, r *http.Request) {
	plateID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req addComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	pc, err := h.svc.AddComponent(r.Context(), plateID, req.FoodID, req.Portions)
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toPlateComponentResponse(pc))
}

type updateComponentRequest struct {
	FoodID   *int64   `json:"food_id"`
	Portions *float64 `json:"portions"`
}

// UpdateComponent handles PUT /api/plates/{id}/components/{pcId}.
// Supports two modes: swap (food_id provided) or rescale (portions only).
func (h *PlateHandler) UpdateComponent(w http.ResponseWriter, r *http.Request) {
	pcID, err := strconv.ParseInt(chi.URLParam(r, "pcId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req updateComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	var pc *plate.PlateComponent
	switch {
	case req.FoodID != nil:
		pc, err = h.svc.SwapComponent(r.Context(), pcID, *req.FoodID, req.Portions)
	case req.Portions != nil:
		pc, err = h.svc.UpdateComponentPortions(r.Context(), pcID, *req.Portions)
	default:
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPlateComponentResponse(pc))
}

type skipRequest struct {
	Skipped bool    `json:"skipped"`
	Note    *string `json:"note"`
}

// SetSkipped handles POST /api/plates/{id}/skip.
// Marks a slot as prospectively skipped (eating out / canteen) so the fill-empty
// kitchen agent leaves it alone. Enabling skip clears attached components.
func (h *PlateHandler) SetSkipped(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req skipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	p, err := h.svc.SetSkipped(r.Context(), id, req.Skipped, req.Note)
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(p, nil))
}

// DeleteComponent handles DELETE /api/plates/{id}/components/{pcId}.
func (h *PlateHandler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	pcID, err := strconv.ParseInt(chi.URLParam(r, "pcId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.RemoveComponent(r.Context(), pcID); err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
