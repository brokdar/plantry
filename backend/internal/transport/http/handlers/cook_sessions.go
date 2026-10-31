package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// cookSessionsService is the subset of plate.Service the session endpoints
// use.
type cookSessionsService interface {
	Session(ctx context.Context, id int64) (*plate.SessionView, error)
	SetSessionPortions(ctx context.Context, id int64, in plate.SessionPortions) (*plate.SessionView, error)
	SetCooked(ctx context.Context, id int64, cooked bool) (*plate.SessionView, error)
	LeftoverCells(ctx context.Context, id int64, count int) ([]plate.CellKey, error)
	AddLeftovers(ctx context.Context, id int64, cells []plate.CellKey) (*plate.SessionView, error)
	DeleteSession(ctx context.Context, id int64) (plate.PlanSnapshot, error)
}

// CookSessionHandler exposes /api/cook-sessions.
type CookSessionHandler struct {
	svc cookSessionsService
}

// NewCookSessionHandler creates a CookSessionHandler.
func NewCookSessionHandler(svc cookSessionsService) *CookSessionHandler {
	return &CookSessionHandler{svc: svc}
}

type sessionLinkResponse struct {
	PlateID     int64   `json:"plate_id"`
	ComponentID int64   `json:"component_id"`
	Date        string  `json:"date"`
	SlotID      int64   `json:"slot_id"`
	Portions    float64 `json:"portions"`
	Kind        string  `json:"kind"`
	Position    int     `json:"position"`
}

type sessionWarningResponse struct {
	Code    string `json:"code"`
	PlateID *int64 `json:"plate_id"`
}

type sessionResponse struct {
	ID             int64                    `json:"id"`
	FoodID         int64                    `json:"food_id"`
	FoodName       string                   `json:"food_name"`
	CookDate       string                   `json:"cook_date"`
	CookSlotID     int64                    `json:"cook_slot_id"`
	Portions       float64                  `json:"portions"`
	FrozenPortions float64                  `json:"frozen_portions"`
	CookedAt       *string                  `json:"cooked_at"`
	Note           *string                  `json:"note"`
	KeepDays       int                      `json:"keep_days"`
	UseBy          string                   `json:"use_by"`
	Consumed       float64                  `json:"consumed"`
	FrozenConsumed float64                  `json:"frozen_consumed"`
	Surplus        float64                  `json:"surplus"`
	Links          []sessionLinkResponse    `json:"links"`
	Meals          int                      `json:"meals"`
	Warnings       []sessionWarningResponse `json:"warnings"`
}

func toSessionResponse(v *plate.SessionView) sessionResponse {
	resp := sessionResponse{
		ID: v.ID, FoodID: v.FoodID, FoodName: v.FoodName,
		CookDate: v.CookDate.Format(time.DateOnly), CookSlotID: v.CookSlotID,
		Portions: v.Portions, FrozenPortions: v.FrozenPortions, Note: v.Note,
		KeepDays: v.KeepDays, UseBy: v.UseBy.Format(time.DateOnly),
		Consumed: v.Consumed, FrozenConsumed: v.FrozenConsumed, Surplus: v.Surplus,
		Links: make([]sessionLinkResponse, len(v.Links)), Meals: v.Meals,
		Warnings: make([]sessionWarningResponse, len(v.Warnings)),
	}
	if v.CookedAt != nil {
		s := v.CookedAt.UTC().Format(time.RFC3339)
		resp.CookedAt = &s
	}
	for i, l := range v.Links {
		resp.Links[i] = toSessionLinkResponse(l)
	}
	for i, w := range v.Warnings {
		resp.Warnings[i] = sessionWarningResponse{Code: w.Code, PlateID: w.PlateID}
	}
	return resp
}

func toSessionLinkResponse(l plate.Link) sessionLinkResponse {
	return sessionLinkResponse{
		PlateID: l.PlateID, ComponentID: l.ComponentID, Date: l.Cell.Date.Format(time.DateOnly),
		SlotID: l.Cell.SlotID, Portions: l.Portions, Kind: string(l.Kind), Position: l.Position,
	}
}

type cellJSON struct {
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

// Get handles GET /api/cook-sessions/{id}.
func (h *CookSessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	v, err := h.svc.Session(r.Context(), id)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSessionResponse(v))
}

type setSessionPortionsRequest struct {
	Portions       *float64 `json:"portions"`
	FrozenPortions *float64 `json:"frozen_portions"`
	Note           *string  `json:"note"`
}

// Update handles PUT /api/cook-sessions/{id} with {portions, frozen_portions,
// note}. portions is required; frozen_portions defaults to 0 and a missing
// note clears it.
func (h *CookSessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req setSessionPortionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Portions == nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	in := plate.SessionPortions{Portions: *req.Portions, Note: req.Note}
	if req.FrozenPortions != nil {
		in.FrozenPortions = *req.FrozenPortions
	}
	v, err := h.svc.SetSessionPortions(r.Context(), id, in)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSessionResponse(v))
}

// Cooked handles POST /api/cook-sessions/{id}/cooked with {cooked: bool}.
func (h *CookSessionHandler) Cooked(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Cooked *bool `json:"cooked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Cooked == nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	v, err := h.svc.SetCooked(r.Context(), id, *req.Cooked)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSessionResponse(v))
}

// LeftoverCells handles GET /api/cook-sessions/{id}/leftover-cells?count=N.
func (h *CookSessionHandler) LeftoverCells(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	cells, err := h.svc.LeftoverCells(r.Context(), id, count)
	if err != nil {
		writePlateError(w, err)
		return
	}
	out := make([]cellJSON, len(cells))
	for i, c := range cells {
		out[i] = cellJSON{Date: c.Date.Format(time.DateOnly), SlotID: c.SlotID}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cells": out})
}

// Leftovers handles POST /api/cook-sessions/{id}/leftovers with {cells}.
func (h *CookSessionHandler) Leftovers(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Cells []cellJSON `json:"cells"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	cells := make([]plate.CellKey, len(req.Cells))
	for i, c := range req.Cells {
		d, err := time.Parse(time.DateOnly, c.Date)
		if err != nil || c.SlotID <= 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		cells[i] = plate.CellKey{Date: d, SlotID: c.SlotID}
	}
	v, err := h.svc.AddLeftovers(r.Context(), id, cells)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSessionResponse(v))
}

// Delete handles DELETE /api/cook-sessions/{id} ("Remove batch") and returns
// what restores it.
func (h *CookSessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	snap, err := h.svc.DeleteSession(r.Context(), id)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, removedResponse{Removed: snap})
}
