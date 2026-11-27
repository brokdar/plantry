package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// FindsHandler serves the finds API (spec §8): list, read, request,
// keep and discard ("Not for me", NEW-4).
type FindsHandler struct {
	svc    *finds.Service
	chefOn func(ctx context.Context) (bool, error)
}

// NewFindsHandler creates a FindsHandler.
func NewFindsHandler(svc *finds.Service, chefOn func(ctx context.Context) (bool, error)) *FindsHandler {
	return &FindsHandler{svc: svc, chefOn: chefOn}
}

// List handles GET /api/finds[?state=]: {items}.
func (h *FindsHandler) List(w http.ResponseWriter, r *http.Request) {
	var state *finds.State
	if v := r.URL.Query().Get("state"); v != "" {
		s := finds.State(v)
		state = &s
	}
	items, err := h.svc.List(r.Context(), state)
	if err != nil {
		status, key := toHTTPWithResource(err, "find")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Get handles GET /api/finds/{id}.
func (h *FindsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	f, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := toHTTPWithResource(err, "find")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// Create handles POST /api/finds {dish, from: chat|discover}: a requested
// find (FIND-1c), only with the Chef on. 202 with the searching find.
func (h *FindsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dish string `json:"dish"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	on, err := h.chefOn(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	if !on {
		writeError(w, http.StatusConflict, "error.chef.off")
		return
	}
	f, err := h.svc.Request(r.Context(), req.Dish, finds.TriggerDiscover)
	if errors.Is(err, llm.ErrProviderMissing) {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	if err != nil {
		status, key := toHTTPWithResource(err, "find")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusAccepted, f)
}

// Keep handles POST /api/finds/{id}/keep {food_id}: the review saved the
// find as a library recipe.
func (h *FindsHandler) Keep(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		FoodID int64 `json:"food_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FoodID <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	f, err := h.svc.Keep(r.Context(), id, req.FoodID)
	if err != nil {
		status, key := toHTTPWithResource(err, "find")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// Discard handles POST /api/finds/{id}/discard {reason?, ingredients?}.
func (h *FindsHandler) Discard(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Reason      string   `json:"reason"`
		Ingredients []string `json:"ingredients"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
	}
	f, err := h.svc.Discard(r.Context(), id, req.Reason, req.Ingredients)
	if err != nil {
		status, key := toHTTPWithResource(err, "find")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// FillLibrary handles POST /api/chef/fill-library (COLD-3): ten requested
// finds that enter the library on trial. 202 with the finds.
func (h *FindsHandler) FillLibrary(w http.ResponseWriter, r *http.Request) {
	on, err := h.chefOn(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	if !on {
		writeError(w, http.StatusConflict, "error.chef.off")
		return
	}
	items, err := h.svc.FillLibrary(r.Context())
	if errors.Is(err, llm.ErrProviderMissing) {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	if err != nil {
		status, key := toHTTPWithResource(err, "find")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"items": items})
}
