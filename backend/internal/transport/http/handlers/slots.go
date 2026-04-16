package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// SlotHandler holds the HTTP handlers for time slots.
type SlotHandler struct {
	svc *slot.Service
}

// NewSlotHandler creates a SlotHandler.
func NewSlotHandler(svc *slot.Service) *SlotHandler {
	return &SlotHandler{svc: svc}
}

type slotRequest struct {
	NameKey   string `json:"name_key"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
}

type slotResponse struct {
	ID        int64  `json:"id"`
	NameKey   string `json:"name_key"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
}

func toSlotResponse(s *slot.TimeSlot) slotResponse {
	return slotResponse{
		ID: s.ID, NameKey: s.NameKey, Icon: s.Icon,
		SortOrder: s.SortOrder, Active: s.Active,
	}
}

func slotError(err error) (int, string) {
	return toHTTPWithResource(err, "slot")
}

// List handles GET /api/settings/slots.
func (h *SlotHandler) List(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "true"
	items, err := h.svc.List(r.Context(), activeOnly)
	if err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	out := make([]slotResponse, len(items))
	for i := range items {
		out[i] = toSlotResponse(&items[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Create handles POST /api/settings/slots.
func (h *SlotHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req slotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	s := &slot.TimeSlot{
		NameKey: req.NameKey, Icon: req.Icon,
		SortOrder: req.SortOrder, Active: req.Active,
	}
	if err := h.svc.Create(r.Context(), s); err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toSlotResponse(s))
}

// Update handles PUT /api/settings/slots/{id}.
func (h *SlotHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req slotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	s := &slot.TimeSlot{
		ID: id, NameKey: req.NameKey, Icon: req.Icon,
		SortOrder: req.SortOrder, Active: req.Active,
	}
	if err := h.svc.Update(r.Context(), s); err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toSlotResponse(s))
}

// Delete handles DELETE /api/settings/slots/{id}.
func (h *SlotHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
