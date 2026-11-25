package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
)

// Labelling is the assistant's attribute labelling as the handler uses it
// (flavor.Labelling).
type Labelling interface {
	Estimate(ctx context.Context) (flavor.Estimate, error)
	LabelLibrary(ctx context.Context) (int, error)
}

// ChefHandler serves /api/chef.
type ChefHandler struct {
	spend     func(ctx context.Context) (chef.MonthSpend, error)
	labelling Labelling
}

// NewChefHandler creates the handler; spend is this month's capped spend
// against the cap (chef.Guard.Spent).
func NewChefHandler(spend func(ctx context.Context) (chef.MonthSpend, error), labelling Labelling) *ChefHandler {
	return &ChefHandler{spend: spend, labelling: labelling}
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

// LabellingEstimate handles GET /api/chef/labelling: what the library pass
// would label, its rough cost and duration (ON-2).
func (h *ChefHandler) LabellingEstimate(w http.ResponseWriter, r *http.Request) {
	est, err := h.labelling.Estimate(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "chef.labelling.estimate", "error", err)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, est)
}

// LabelLibrary handles POST /api/chef/labelling: queues the one-time pass
// over the library's unlabelled recipes (ATTR-2). 409 while the Chef is off.
func (h *ChefHandler) LabelLibrary(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.labelling.LabelLibrary(r.Context())
	switch {
	case errors.Is(err, flavor.ErrChefOff):
		writeError(w, http.StatusConflict, "error.chef.off")
		return
	case err != nil:
		slog.ErrorContext(r.Context(), "chef.labelling.start", "error", err, "queued", jobs)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"jobs": jobs})
}
