package handlers

import (
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
)

// SetupHandler exposes first-run setup progress.
type SetupHandler struct {
	svc *setup.Service
}

// NewSetupHandler creates a SetupHandler.
func NewSetupHandler(svc *setup.Service) *SetupHandler {
	return &SetupHandler{svc: svc}
}

type setupStatusJSON struct {
	Completed    bool `json:"completed"`
	RecipeCount  int  `json:"recipe_count"`
	RecipeTarget int  `json:"recipe_target"`
}

func writeSetupStatus(w http.ResponseWriter, st setup.Status, err error) {
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, setupStatusJSON{
		Completed: st.Completed, RecipeCount: st.RecipeCount, RecipeTarget: st.RecipeTarget,
	})
}

// Get handles GET /api/setup.
func (h *SetupHandler) Get(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Status(r.Context())
	writeSetupStatus(w, st, err)
}

// Complete handles POST /api/setup/complete. Idempotent.
func (h *SetupHandler) Complete(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Complete(r.Context())
	writeSetupStatus(w, st, err)
}
