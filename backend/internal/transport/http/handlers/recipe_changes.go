package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/recipechange"
)

// RecipeChangeHandler serves "Ask about this recipe" (HELP-4): the ask run,
// the confirmed in-place edit, the rule check and the changes list.
type RecipeChangeHandler struct {
	svc *recipechange.Service
}

// NewRecipeChangeHandler creates a RecipeChangeHandler.
func NewRecipeChangeHandler(svc *recipechange.Service) *RecipeChangeHandler {
	return &RecipeChangeHandler{svc: svc}
}

type askRequest struct {
	Request string `json:"request"`
}

// Ask handles POST /api/foods/{id}/ask: a variant is saved at once, an
// in-place edit comes back as a preview, a question gets its answer.
func (h *RecipeChangeHandler) Ask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Request) == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	res, err := h.svc.Ask(r.Context(), id, req.Request)
	if err != nil {
		writeRecipeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Edit handles POST /api/foods/{id}/edit: saves the in-place edit the user
// confirmed. A blocked check saves nothing (food_id 0, check says why).
func (h *RecipeChangeHandler) Edit(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var p recipechange.Proposal
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	res, err := h.svc.Edit(r.Context(), id, p)
	if err != nil {
		writeRecipeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Check handles GET /api/foods/{id}/check ("Check this recipe"): the gates
// and the match status of every line, no model call.
func (h *RecipeChangeHandler) Check(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	res, err := h.svc.CheckRecipe(r.Context(), id)
	if err != nil {
		writeRecipeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Changes handles GET /api/foods/{id}/changes: the recipe's changes list.
func (h *RecipeChangeHandler) Changes(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	items, err := h.svc.Changes(r.Context(), id)
	if err != nil {
		writeRecipeChangeError(w, err)
		return
	}
	if items == nil {
		items = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func writeRecipeChangeError(w http.ResponseWriter, err error) {
	if errors.Is(err, llm.ErrProviderMissing) {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	status, key := toHTTPWithResource(err, "food")
	writeError(w, status, key)
}
