package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// planWriteService is the subset of plate.Service the plan-wide write
// endpoints use.
type planWriteService interface {
	ClearRange(ctx context.Context, from, to time.Time, slotID int64) (plate.PlanSnapshot, error)
	Apply(ctx context.Context, snap plate.PlanSnapshot, opts plate.ApplyOptions) (*plate.ApplyResult, error)
	Accept(ctx context.Context, from, to time.Time) (plate.RowSet, error)
	Unaccept(ctx context.Context, rows plate.RowSet) (plate.RowSet, error)
	Discard(ctx context.Context, from, to time.Time) (plate.PlanSnapshot, error)
}

// PlanWriteHandler exposes the plan-wide writes: clear a range, restore a
// snapshot, and accept, unaccept or discard a window's drafts (D4).
type PlanWriteHandler struct {
	svc planWriteService
}

// NewPlanWriteHandler creates a PlanWriteHandler.
func NewPlanWriteHandler(svc planWriteService) *PlanWriteHandler {
	return &PlanWriteHandler{svc: svc}
}

// Clear handles DELETE /api/plan?from&to[&slot_id] and returns what restores it.
func (h *PlanWriteHandler) Clear(w http.ResponseWriter, r *http.Request) {
	from, err := time.Parse(time.DateOnly, r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_query")
		return
	}
	to, err := time.Parse(time.DateOnly, r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_query")
		return
	}
	var slotID int64
	if v := r.URL.Query().Get("slot_id"); v != "" {
		if slotID, err = strconv.ParseInt(v, 10, 64); err != nil || slotID <= 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_query")
			return
		}
	}
	snap, err := h.svc.ClearRange(r.Context(), from, to, slotID)
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

// decodeWindow reads the body {from, to} of accept and discard.
func decodeWindow(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return time.Time{}, time.Time{}, false
	}
	from, errFrom := time.Parse(time.DateOnly, req.From)
	to, errTo := time.Parse(time.DateOnly, req.To)
	if errFrom != nil || errTo != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

// Accept handles POST /api/plan/accept {from, to}: it confirms every draft
// row of the window closure and returns {plate_ids, session_ids}, the body
// of its undo (unaccept). No drafts: 400 error.plan.nothing_to_accept.
func (h *PlanWriteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	from, to, ok := decodeWindow(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.Accept(r.Context(), from, to)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRowSetResponse(rows))
}

// Unaccept handles POST /api/plan/unaccept {plate_ids, session_ids}: it
// returns exactly those rows to draft. A row that is not confirmed: 409
// error.plan.state_mismatch.
func (h *PlanWriteHandler) Unaccept(w http.ResponseWriter, r *http.Request) {
	var req rowSetResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	rows, err := h.svc.Unaccept(r.Context(), plate.RowSet{PlateIDs: req.PlateIDs, SessionIDs: req.SessionIDs})
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRowSetResponse(rows))
}

// Discard handles POST /api/plan/discard {from, to}: it deletes every draft
// row of the window closure and returns {removed}, which POST
// /api/plan/restore brings back as drafts. No drafts: 400
// error.plan.nothing_to_accept.
func (h *PlanWriteHandler) Discard(w http.ResponseWriter, r *http.Request) {
	from, to, ok := decodeWindow(w, r)
	if !ok {
		return
	}
	snap, err := h.svc.Discard(r.Context(), from, to)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, removedResponse{Removed: snap})
}
