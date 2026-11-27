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
	Apply(ctx context.Context, sessionID, fromID, toID int64) (substitution.Applied, error)
	Always(ctx context.Context, sessionID, fromID, toID int64) (foodevent.Undo, error)
	SaveVariant(ctx context.Context, sessionID, fromID, toID int64) (substitution.Saved, error)
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
	res, err := h.svc.Apply(r.Context(), id, req.FromFoodID, req.ToFoodID)
	if err != nil {
		writePlateError(w, err)
		return
	}
	dates := res.ConfirmedDates
	if dates == nil {
		dates = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"undo": res.Undo, "offer_permanent": res.OfferPermanent, "confirmed_dates": dates})
}

type keepOverrideRequest struct {
	applyOverrideRequest
	// Mode is "recipe" (Always in this recipe) or "variant" (Save as variant).
	Mode string `json:"mode"`
}

// Keep handles POST /api/cook-sessions/{id}/overrides/keep {from_food_id,
// to_food_id, mode}: the swap just applied becomes permanent, either in the
// recipe itself or as a visible variant. It returns the undo token and, for a
// variant, its id and name.
func (h *SubstitutionHandler) Keep(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req keepOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FromFoodID <= 0 || req.ToFoodID <= 0 ||
		(req.Mode != "recipe" && req.Mode != "variant") {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.Mode == "recipe" {
		undo, err := h.svc.Always(r.Context(), id, req.FromFoodID, req.ToFoodID)
		if err != nil {
			writePlateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"undo": undo})
		return
	}
	saved, err := h.svc.SaveVariant(r.Context(), id, req.FromFoodID, req.ToFoodID)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"undo": saved.Undo, "food_id": saved.FoodID, "name": saved.Name})
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
