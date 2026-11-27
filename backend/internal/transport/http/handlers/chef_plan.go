package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/tips"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// ChefPlanDeps wire the Chef's plan page endpoints (C3).
type ChefPlanDeps struct {
	Finds       *finds.Service
	ChefOn      func(ctx context.Context) (bool, error)
	ShoppingDay func(ctx context.Context) (int, error)
	Plates      interface {
		Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
	}
	// LiveTips counts the tips shown on the window starting on start: they
	// share the PRO-4 noise budget with the assistant's list.
	LiveTips func(ctx context.Context, start time.Time) (int, error)
	// Suggested reports an open theme suggestion on the window: it counts
	// in the same budget (THEME-4); nil counts none.
	Suggested func(ctx context.Context, start time.Time) (bool, error)
	Now       func() time.Time
}

// ChefPlanHandler serves the plan page's planning session and the "What the
// assistant did" list (PRO-5).
type ChefPlanHandler struct{ d ChefPlanDeps }

// NewChefPlanHandler creates a ChefPlanHandler.
func NewChefPlanHandler(d ChefPlanDeps) *ChefPlanHandler {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &ChefPlanHandler{d: d}
}

// Opened handles POST /api/plan/opened {start, today}: the plan page shows
// the window at start. A window with drafts or the next window opens a
// planning session; the next window prefetches its new dishes (FIND-1a).
func (h *ChefPlanHandler) Opened(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Start string `json:"start"`
		Today string `json:"today"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	start, err := time.Parse(time.DateOnly, req.Start)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	today, ok := parseToday(w, req.Today, h.d.Now)
	if !ok {
		return
	}
	ctx := r.Context()
	if on, err := h.d.ChefOn(ctx); err != nil || !on {
		writeErrorOrNoContent(w, err)
		return
	}
	sd, err := h.d.ShoppingDay(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	next := window.Containing(today, sd).Start.AddDate(0, 0, 7).Equal(start)
	plates, err := h.d.Plates.Range(ctx, start, start.AddDate(0, 0, 6))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	drafts := false
	for _, p := range plates {
		drafts = drafts || p.State == plate.StateDraft
	}
	if err := h.d.Finds.Opened(ctx, start, drafts, next); err != nil {
		status, key := toHTTPWithResource(err, "find")
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeErrorOrNoContent(w http.ResponseWriter, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Assistant handles GET /api/plan/assistant?start=: {items, more}, the
// window's "What the assistant did" list within the noise budget left
// after its tips and theme suggestion (PRO-4); empty with the Chef off.
func (h *ChefPlanHandler) Assistant(w http.ResponseWriter, r *http.Request) {
	start, err := time.Parse(time.DateOnly, r.URL.Query().Get("start"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	ctx := r.Context()
	resp := struct {
		Items []finds.Activity `json:"items"`
		More  int              `json:"more"`
	}{Items: []finds.Activity{}}
	on, err := h.d.ChefOn(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	if !on {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	shown, err := h.d.LiveTips(ctx, start)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	if h.d.Suggested != nil {
		sg, err := h.d.Suggested(ctx, start)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error.server")
			return
		}
		if sg {
			shown++
		}
	}
	resp.Items, resp.More, err = h.d.Finds.Activities(ctx, start, tips.NoiseBudget-shown)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// UndoPlaced handles POST /api/plan/assistant/finds/{id}/undo: the placed
// find leaves the window's drafts and the library and is ready again.
func (h *ChefPlanHandler) UndoPlaced(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.d.Finds.UndoPlaced(r.Context(), id); err != nil {
		status, key := toHTTPWithResource(err, "find")
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
