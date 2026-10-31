package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// planWriteService is the subset of plate.Service the plan-wide write
// endpoints use.
type planWriteService interface {
	ClearRange(ctx context.Context, from, to time.Time) (plate.PlanSnapshot, error)
	Apply(ctx context.Context, snap plate.PlanSnapshot, opts plate.ApplyOptions) (*plate.ApplyResult, error)
}

// PlanWriteHandler exposes the plan-wide writes: clear a range and restore a
// snapshot.
type PlanWriteHandler struct {
	svc planWriteService
}

// NewPlanWriteHandler creates a PlanWriteHandler.
func NewPlanWriteHandler(svc planWriteService) *PlanWriteHandler {
	return &PlanWriteHandler{svc: svc}
}

// Clear handles DELETE /api/plan?from&to and returns what restores it.
func (h *PlanWriteHandler) Clear(w http.ResponseWriter, r *http.Request) {
	from, err := time.Parse(time.DateOnly, r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	to, err := time.Parse(time.DateOnly, r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	snap, err := h.svc.ClearRange(r.Context(), from, to)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, removedResponse{Removed: snap})
}

// Restore handles POST /api/plan/restore with a PlanSnapshot. It writes all
// of it or nothing: an occupied cell is 409 error.plan.cell_occupied.
func (h *PlanWriteHandler) Restore(w http.ResponseWriter, r *http.Request) {
	var snap plate.PlanSnapshot
	if err := json.NewDecoder(r.Body).Decode(&snap); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	res, err := h.svc.Apply(r.Context(), snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	if errors.Is(err, domain.ErrCellOccupied) {
		writeError(w, http.StatusConflict, "error.plan.cell_occupied")
		return
	}
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plate_ids": res.PlateIDs})
}
