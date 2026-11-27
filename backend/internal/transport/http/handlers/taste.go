package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// TasteHandler serves the taste model's read API (TASTE-11's data; the
// Settings page comes with D2). The Undo tokens it returns are reverted by
// POST /api/food-events/revert.
type TasteHandler struct {
	svc *taste.Service
}

// NewTasteHandler creates a TasteHandler.
func NewTasteHandler(svc *taste.Service) *TasteHandler { return &TasteHandler{svc: svc} }

// Get handles GET /api/taste[?window=YYYY-MM-DD]: {items, recent}. window
// is the plan window whose window-scoped items are included.
func (h *TasteHandler) Get(w http.ResponseWriter, r *http.Request) {
	var window time.Time
	if v := r.URL.Query().Get("window"); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_query")
			return
		}
		window = d
	}
	v, err := h.svc.View(r.Context(), window)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// Delete handles DELETE /api/taste/items/{id}: {undo}.
func (h *TasteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	undo, err := h.svc.Remove(r.Context(), id)
	if err != nil {
		status, key := toHTTPWithResource(err, "taste_item")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"undo": undo})
}

// Put handles PUT /api/taste/items/{id} with {polarity: love|rather_not}:
// the user's own polarity, which outranks learned evidence (flip).
// Returns {item, undo}.
func (h *TasteHandler) Put(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Polarity taste.Polarity `json:"polarity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.Polarity.Valid() {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	it, undo, err := h.svc.SetPolarity(r.Context(), id, req.Polarity)
	if err != nil {
		status, key := toHTTPWithResource(err, "taste_item")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": it, "undo": undo})
}
