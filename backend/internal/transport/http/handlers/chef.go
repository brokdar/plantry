package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
)

// ChefHandler serves /api/chef.
type ChefHandler struct {
	spend func(ctx context.Context) (chef.MonthSpend, error)
}

// NewChefHandler creates the handler; spend is this month's capped spend
// against the cap (chef.Guard.Spent).
func NewChefHandler(spend func(ctx context.Context) (chef.MonthSpend, error)) *ChefHandler {
	return &ChefHandler{spend: spend}
}

// Spend handles GET /api/chef/spend (COST-4).
func (h *ChefHandler) Spend(w http.ResponseWriter, r *http.Request) {
	s, err := h.spend(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "chef.spend", "error", err)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, s)
}
