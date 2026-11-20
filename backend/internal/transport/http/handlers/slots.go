package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// SlotHandler holds the HTTP handlers for time slots.
type SlotHandler struct {
	svc *slot.Service
}

// NewSlotHandler creates a SlotHandler.
func NewSlotHandler(svc *slot.Service) *SlotHandler {
	return &SlotHandler{svc: svc}
}

type slotRequest struct {
	NameKey   string `json:"name_key"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
	Kind      string `json:"kind"`
}

type slotResponse struct {
	ID        int64  `json:"id"`
	NameKey   string `json:"name_key"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
	Kind      string `json:"kind"`
	// BandMinPct and BandMaxPct are the slot's effective energy band, in percent
	// of the day's energy; BandCustom says the user set it (else it is the
	// default of the slot's name or kind).
	BandMinPct float64 `json:"band_min_pct"`
	BandMaxPct float64 `json:"band_max_pct"`
	BandCustom bool    `json:"band_custom"`
}

func toSlotResponse(s *slot.TimeSlot) slotResponse {
	b := s.Effective()
	return slotResponse{
		ID: s.ID, NameKey: s.NameKey, Icon: s.Icon,
		SortOrder: s.SortOrder, Active: s.Active, Kind: string(s.Kind),
		BandMinPct: b.MinPct, BandMaxPct: b.MaxPct, BandCustom: s.BandMinPct != nil && s.BandMaxPct != nil,
	}
}

type bandItem struct {
	SlotID int64   `json:"slot_id"`
	MinPct float64 `json:"min_pct"`
	MaxPct float64 `json:"max_pct"`
}

type bandsResponse struct {
	Items []bandItem `json:"items"`
	// Shares are the expected shares of the day per slot (SB2), as percent.
	Shares map[int64]float64 `json:"shares"`
}

func bandsOut(bands []slot.Band) bandsResponse {
	out := bandsResponse{Items: make([]bandItem, len(bands)), Shares: map[int64]float64{}}
	for i, b := range bands {
		out.Items[i] = bandItem{SlotID: b.SlotID, MinPct: b.MinPct, MaxPct: b.MaxPct}
	}
	for id, v := range slot.Shares(bands) {
		out.Shares[id] = v * 100
	}
	return out
}

// Bands handles GET /api/settings/slots/bands: the effective bands of the active
// slots and their expected shares.
func (h *SlotHandler) Bands(w http.ResponseWriter, r *http.Request) {
	bands, err := h.svc.Bands(r.Context())
	if err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, bandsOut(bands))
}

// SetBands handles PUT /api/settings/slots/bands: the whole set is checked and
// stored together, or refused with error.slot.invalid_bands and nothing changes.
func (h *SlotHandler) SetBands(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []bandItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	set := make([]slot.Band, len(req.Items))
	for i, it := range req.Items {
		set[i] = slot.Band{SlotID: it.SlotID, MinPct: it.MinPct, MaxPct: it.MaxPct}
	}
	if err := h.svc.SetBands(r.Context(), set); err != nil {
		h.bandsError(w, err)
		return
	}
	h.Bands(w, r)
}

// ApplyPreset handles POST /api/settings/slots/bands/preset {name}.
func (h *SlotHandler) ApplyPreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err := h.svc.ApplyPreset(r.Context(), req.Name); err != nil {
		h.bandsError(w, err)
		return
	}
	h.Bands(w, r)
}

// bandsError is slotError plus the refusal's numbers, which the settings form
// shows as they are.
func (h *SlotHandler) bandsError(w http.ResponseWriter, err error) {
	status, key := slotError(err)
	if errors.Is(err, domain.ErrInvalidSlotBands) {
		writeErrorParams(w, status, key, map[string]string{"detail": err.Error()})
		return
	}
	writeError(w, status, key)
}

func slotError(err error) (int, string) {
	return toHTTPWithResource(err, "slot")
}

// List handles GET /api/settings/slots.
func (h *SlotHandler) List(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "true"
	items, err := h.svc.List(r.Context(), activeOnly)
	if err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	out := make([]slotResponse, len(items))
	for i := range items {
		out[i] = toSlotResponse(&items[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Create handles POST /api/settings/slots.
func (h *SlotHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req slotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	s := &slot.TimeSlot{
		NameKey: req.NameKey, Icon: req.Icon,
		SortOrder: req.SortOrder, Active: req.Active, Kind: slot.Kind(req.Kind),
	}
	if err := h.svc.Create(r.Context(), s); err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toSlotResponse(s))
}

// Update handles PUT /api/settings/slots/{id}.
func (h *SlotHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req slotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	s := &slot.TimeSlot{
		ID: id, NameKey: req.NameKey, Icon: req.Icon,
		SortOrder: req.SortOrder, Active: req.Active, Kind: slot.Kind(req.Kind),
	}
	if err := h.svc.Update(r.Context(), s); err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toSlotResponse(s))
}

// Delete handles DELETE /api/settings/slots/{id}.
func (h *SlotHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := slotError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
