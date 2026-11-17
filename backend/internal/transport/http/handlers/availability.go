package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
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

// setOverrideResponse carries the cell and, when remove_plate deleted the
// cell's plate, the snapshot that restores it (POST /api/plan/restore), and
// the batches whose cook moved off the cell (empty when none did).
type setOverrideResponse struct {
	Cell        availabilityCellJSON `json:"cell"`
	Removed     *plate.PlanSnapshot  `json:"removed"`
	AlsoChanged []plate.AlsoChanged  `json:"also_changed"`
}

type awayCellJSON struct {
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

type awayRequest struct {
	Cells []awayCellJSON `json:"cells"`
	Date  *string        `json:"date"`
	Note  *string        `json:"note"`
}

// overrideJSON is a cell's stored override; null in awayBeforeJSON means
// the cell had none.
type overrideJSON struct {
	Mode     string  `json:"mode"`
	PresetID *int64  `json:"preset_id"`
	Note     *string `json:"note"`
}

type awayBeforeJSON struct {
	Date     string        `json:"date"`
	SlotID   int64         `json:"slot_id"`
	Override *overrideJSON `json:"override"`
}

type awayResponse struct {
	Cells       []availabilityCellJSON `json:"cells"`
	Before      []awayBeforeJSON       `json:"before"`
	Removed     plate.PlanSnapshot     `json:"removed"`
	AlsoChanged []plate.AlsoChanged    `json:"also_changed"`
	Undo        plate.Undo             `json:"undo"`
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
	cell, res, err := h.svc.SetOverrideMeals(r.Context(), availability.OverrideInput{
		Date: date, SlotID: slotID, Mode: availability.Mode(req.Mode),
		PresetID: req.PresetID, Note: req.Note, RemovePlate: req.RemovePlate,
	})
	if err != nil {
		availabilityError(w, err)
		return
	}
	out := setOverrideResponse{Cell: toAvailabilityCellJSON(cell), AlsoChanged: []plate.AlsoChanged{}}
	if res != nil {
		out.AlsoChanged = res.AlsoChanged
		if len(res.Removed.Plates) > 0 {
			out.Removed = &res.Removed
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// PostAway handles POST /api/availability/away?today=: the cells (or, with
// date alone, every active slot of that date that is not routine) become
// No plate and their plates are removed in one unit of work.
func (h *AvailabilityHandler) PostAway(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), time.Now)
	if !ok {
		return
	}
	var req awayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	in := availability.AwayInput{Note: req.Note, Today: today}
	if len(req.Cells) == 0 && req.Date == nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	for _, c := range req.Cells {
		d, err := time.Parse(time.DateOnly, c.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		in.Cells = append(in.Cells, plate.CellKey{Date: d, SlotID: c.SlotID})
	}
	if req.Date != nil && len(in.Cells) == 0 {
		d, err := time.Parse(time.DateOnly, *req.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		in.Date = &d
	}
	res, err := h.svc.SetAway(r.Context(), in)
	if err != nil {
		availabilityError(w, err)
		return
	}
	out := awayResponse{
		Cells: make([]availabilityCellJSON, len(res.Cells)), Before: make([]awayBeforeJSON, len(res.Before)),
		Removed: res.Removed, AlsoChanged: res.AlsoChanged, Undo: undoOf(res.Undo),
	}
	for i, c := range res.Cells {
		out.Cells[i] = toAvailabilityCellJSON(c)
	}
	for i, b := range res.Before {
		out.Before[i] = awayBeforeJSON{Date: b.Date.Format(time.DateOnly), SlotID: b.SlotID}
		if o := b.Override; o != nil {
			out.Before[i].Override = &overrideJSON{Mode: string(o.Mode), PresetID: o.PresetID, Note: o.Note}
		}
	}
	writeJSON(w, http.StatusOK, out)
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
