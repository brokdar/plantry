package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
)

// maxAvailabilityRangeDays caps GET /api/availability (inclusive day count).
const maxAvailabilityRangeDays = 62

// AvailabilityHandler exposes the availability defaults and per-cell overrides.
type AvailabilityHandler struct {
	svc *availability.Service
}

// NewAvailabilityHandler creates an AvailabilityHandler.
func NewAvailabilityHandler(svc *availability.Service) *AvailabilityHandler {
	return &AvailabilityHandler{svc: svc}
}

type availabilityDefaultJSON struct {
	Weekday  int    `json:"weekday"`
	SlotID   int64  `json:"slot_id"`
	Mode     string `json:"mode"`
	PresetID *int64 `json:"preset_id"`
}

type availabilityDefaultsJSON struct {
	Items []availabilityDefaultJSON `json:"items"`
}

type availabilityCellJSON struct {
	Date       string  `json:"date"`
	SlotID     int64   `json:"slot_id"`
	Mode       string  `json:"mode"`
	Source     string  `json:"source"`
	PresetID   *int64  `json:"preset_id"`
	PresetName *string `json:"preset_name"`
	Note       *string `json:"note"`
}

type setOverrideRequest struct {
	Mode        string  `json:"mode"`
	PresetID    *int64  `json:"preset_id"`
	Note        *string `json:"note"`
	RemovePlate bool    `json:"remove_plate"`
}

type setOverrideResponse struct {
	Cell         availabilityCellJSON `json:"cell"`
	RemovedPlate *plateResponse       `json:"removed_plate,omitempty"`
}

func availabilityError(w http.ResponseWriter, err error) {
	status, key := toHTTPWithResource(err, "availability")
	writeError(w, status, key)
}

func toAvailabilityDefaultsJSON(rows []availability.Default) availabilityDefaultsJSON {
	items := make([]availabilityDefaultJSON, len(rows))
	for i, d := range rows {
		items[i] = availabilityDefaultJSON{Weekday: d.Weekday, SlotID: d.SlotID, Mode: string(d.Mode), PresetID: d.PresetID}
	}
	return availabilityDefaultsJSON{Items: items}
}

func toAvailabilityCellJSON(c availability.Cell) availabilityCellJSON {
	return availabilityCellJSON{
		Date: c.Date.Format(time.DateOnly), SlotID: c.SlotID, Mode: string(c.Mode), Source: string(c.Source),
		PresetID: c.PresetID, PresetName: c.PresetName, Note: c.Note,
	}
}

// GetDefaults handles GET /api/availability/defaults.
func (h *AvailabilityHandler) GetDefaults(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Defaults(r.Context())
	if err != nil {
		availabilityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAvailabilityDefaultsJSON(rows))
}

// PutDefaults handles PUT /api/availability/defaults with the full grid.
func (h *AvailabilityHandler) PutDefaults(w http.ResponseWriter, r *http.Request) {
	var req availabilityDefaultsJSON
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Items == nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	rows := make([]availability.Default, len(req.Items))
	for i, it := range req.Items {
		rows[i] = availability.Default{Weekday: it.Weekday, SlotID: it.SlotID, Mode: availability.Mode(it.Mode), PresetID: it.PresetID}
	}
	if err := h.svc.SetDefaults(r.Context(), rows); err != nil {
		availabilityError(w, err)
		return
	}
	h.GetDefaults(w, r)
}

// List handles GET /api/availability?from=YYYY-MM-DD&to=YYYY-MM-DD.
func (h *AvailabilityHandler) List(w http.ResponseWriter, r *http.Request) {
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
	if to.Before(from) || to.Sub(from) >= maxAvailabilityRangeDays*24*time.Hour {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	cells, err := h.svc.Resolve(r.Context(), from, to)
	if err != nil {
		availabilityError(w, err)
		return
	}
	items := make([]availabilityCellJSON, len(cells))
	for i, c := range cells {
		items[i] = toAvailabilityCellJSON(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// parseCell reads {date} and {slotId} from the path. It writes the 400 and
// returns ok=false on failure.
func parseCell(w http.ResponseWriter, r *http.Request) (time.Time, int64, bool) {
	date, err := time.Parse(time.DateOnly, chi.URLParam(r, "date"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return time.Time{}, 0, false
	}
	slotID, err := strconv.ParseInt(chi.URLParam(r, "slotId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return time.Time{}, 0, false
	}
	return date, slotID, true
}

// PutOverride handles PUT /api/availability/{date}/{slotId}.
func (h *AvailabilityHandler) PutOverride(w http.ResponseWriter, r *http.Request) {
	date, slotID, ok := parseCell(w, r)
	if !ok {
		return
	}
	var req setOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	cell, removed, err := h.svc.SetOverride(r.Context(), availability.OverrideInput{
		Date: date, SlotID: slotID, Mode: availability.Mode(req.Mode),
		PresetID: req.PresetID, Note: req.Note, RemovePlate: req.RemovePlate,
	})
	if err != nil {
		availabilityError(w, err)
		return
	}
	resp := setOverrideResponse{Cell: toAvailabilityCellJSON(cell)}
	if removed != nil {
		p := toPlateResponse(removed)
		resp.RemovedPlate = &p
	}
	writeJSON(w, http.StatusOK, resp)
}

// DeleteOverride handles DELETE /api/availability/{date}/{slotId}. Idempotent.
func (h *AvailabilityHandler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	date, slotID, ok := parseCell(w, r)
	if !ok {
		return
	}
	if err := h.svc.ClearOverride(r.Context(), date, slotID); err != nil {
		availabilityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
