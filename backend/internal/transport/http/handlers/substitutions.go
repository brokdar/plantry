package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/substitution"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// substitutionService is the subset of substitution.Service the endpoints use.
type substitutionService interface {
	Options(ctx context.Context, fromID int64, sessionIDs []int64) ([]substitution.Option, []int64, error)
	Apply(ctx context.Context, sessionID, fromID, toID int64) (foodevent.Undo, error)
	Remove(ctx context.Context, sessionID int64) (foodevent.Undo, error)
}

// SubstitutionHandler serves the rule-based "I don't have it" substitutions.
type SubstitutionHandler struct {
	svc substitutionService
}

// NewSubstitutionHandler creates a SubstitutionHandler.
func NewSubstitutionHandler(svc substitutionService) *SubstitutionHandler {
	return &SubstitutionHandler{svc: svc}
}

type substituteOption struct {
	FoodID int64           `json:"food_id"`
	Name   string          `json:"name"`
	Ratio  float64         `json:"ratio"`
	Amount *units.Quantity `json:"amount"`
	NoteEN string          `json:"note_en"`
	NoteDE string          `json:"note_de"`
}

// Options handles GET /api/substitutes?food_id=&session_ids=1,2: up to three
// substitutes for ingredient food_id with the amounts the listed batches need,
// and the listed batches the swap applies to (session_ids).
func (h *SubstitutionHandler) Options(w http.ResponseWriter, r *http.Request) {
	foodID, err := strconv.ParseInt(r.URL.Query().Get("food_id"), 10, 64)
	if err != nil || foodID <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	var ids []int64
	for _, part := range strings.Split(r.URL.Query().Get("session_ids"), ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		ids = append(ids, id)
	}
	opts, applicable, err := h.svc.Options(r.Context(), foodID, ids)
	if err != nil {
		writePlateError(w, err)
		return
	}
	out := make([]substituteOption, len(opts))
	for i, o := range opts {
		out[i] = substituteOption{FoodID: o.FoodID, Name: o.Name, Ratio: o.Ratio, NoteEN: o.NoteEN, NoteDE: o.NoteDE}
		if o.Amount.Amount > 0 {
			q := o.Amount
			out[i].Amount = &q
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": out, "session_ids": applicable})
}

type applyOverrideRequest struct {
	FromFoodID int64 `json:"from_food_id"`
	ToFoodID   int64 `json:"to_food_id"`
}

// Apply handles POST /api/cook-sessions/{id}/overrides {from_food_id,
// to_food_id}: it swaps the ingredient for this batch only and returns the
// undo token for POST /api/food-events/revert.
func (h *SubstitutionHandler) Apply(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req applyOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FromFoodID <= 0 || req.ToFoodID <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	undo, err := h.svc.Apply(r.Context(), id, req.FromFoodID, req.ToFoodID)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"undo": undo})
}

// Remove handles DELETE /api/cook-sessions/{id}/overrides: the batch cooks
// the recipe as written again.
func (h *SubstitutionHandler) Remove(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	undo, err := h.svc.Remove(r.Context(), id)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"undo": undo})
}
