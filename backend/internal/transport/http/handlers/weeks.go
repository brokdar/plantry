package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// WeekHandler exposes weekly planner read endpoints.
type WeekHandler struct {
	planner *planner.Service
	plates  *plate.Service
}

// NewWeekHandler creates a WeekHandler.
func NewWeekHandler(p *planner.Service, plates *plate.Service) *WeekHandler {
	return &WeekHandler{planner: p, plates: plates}
}

type plateComponentResponse struct {
	ID          int64   `json:"id"`
	PlateID     int64   `json:"plate_id"`
	ComponentID int64   `json:"component_id"`
	Portions    float64 `json:"portions"`
	SortOrder   int     `json:"sort_order"`
}

type plateResponse struct {
	ID         int64                    `json:"id"`
	WeekID     int64                    `json:"week_id"`
	Day        int                      `json:"day"`
	SlotID     int64                    `json:"slot_id"`
	Note       *string                  `json:"note,omitempty"`
	Components []plateComponentResponse `json:"components"`
	CreatedAt  string                   `json:"created_at"`
}

type weekResponse struct {
	ID         int64           `json:"id"`
	Year       int             `json:"year"`
	WeekNumber int             `json:"week_number"`
	Plates     []plateResponse `json:"plates"`
	CreatedAt  string          `json:"created_at"`
}

type weekListResponse struct {
	Items []weekResponse `json:"items"`
	Total int64          `json:"total"`
}

func toPlateComponentResponse(pc *plate.PlateComponent) plateComponentResponse {
	return plateComponentResponse{
		ID: pc.ID, PlateID: pc.PlateID, ComponentID: pc.ComponentID,
		Portions: pc.Portions, SortOrder: pc.SortOrder,
	}
}

func toPlateResponse(p *plate.Plate) plateResponse {
	comps := make([]plateComponentResponse, len(p.Components))
	for i := range p.Components {
		comps[i] = toPlateComponentResponse(&p.Components[i])
	}
	return plateResponse{
		ID: p.ID, WeekID: p.WeekID, Day: p.Day, SlotID: p.SlotID,
		Note: p.Note, Components: comps,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
	}
}

func toWeekResponse(w *planner.Week) weekResponse {
	plates := make([]plateResponse, len(w.Plates))
	for i := range w.Plates {
		plates[i] = toPlateResponse(&w.Plates[i])
	}
	return weekResponse{
		ID: w.ID, Year: w.Year, WeekNumber: w.WeekNumber,
		Plates: plates, CreatedAt: w.CreatedAt.Format(time.RFC3339),
	}
}

func weekError(err error) (int, string) {
	return toHTTPWithResource(err, "week")
}

// Current handles GET /api/weeks/current.
func (h *WeekHandler) Current(w http.ResponseWriter, r *http.Request) {
	week, err := h.planner.Current(r.Context(), time.Now().UTC())
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toWeekResponse(week))
}

// ByDate handles GET /api/weeks/by-date?year=&week=.
func (h *WeekHandler) ByDate(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	weekNum, err := strconv.Atoi(r.URL.Query().Get("week"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	week, err := h.planner.ByDate(r.Context(), year, weekNum)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toWeekResponse(week))
}

// Get handles GET /api/weeks/{id}.
func (h *WeekHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	week, err := h.planner.Get(r.Context(), id)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toWeekResponse(week))
}

// List handles GET /api/weeks.
func (h *WeekHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := 25
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	rows, total, err := h.planner.List(r.Context(), limit, offset)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	items := make([]weekResponse, len(rows))
	for i := range rows {
		items[i] = toWeekResponse(&rows[i])
	}
	writeJSON(w, http.StatusOK, weekListResponse{Items: items, Total: total})
}

type copyWeekRequest struct {
	TargetYear int `json:"target_year"`
	TargetWeek int `json:"target_week"`
}

// Copy handles POST /api/weeks/{id}/copy.
func (h *WeekHandler) Copy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req copyWeekRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	target, err := h.planner.Copy(r.Context(), id, req.TargetYear, req.TargetWeek)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toWeekResponse(target))
}

type createPlateRequest struct {
	Day        int                          `json:"day"`
	SlotID     int64                        `json:"slot_id"`
	Note       *string                      `json:"note"`
	Components []createPlateComponentInline `json:"components"`
}

type createPlateComponentInline struct {
	ComponentID int64   `json:"component_id"`
	Portions    float64 `json:"portions"`
}

// CreatePlate handles POST /api/weeks/{id}/plates.
func (h *WeekHandler) CreatePlate(w http.ResponseWriter, r *http.Request) {
	weekID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req createPlateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	p := &plate.Plate{
		WeekID: weekID, Day: req.Day, SlotID: req.SlotID, Note: req.Note,
	}
	for i, c := range req.Components {
		p.Components = append(p.Components, plate.PlateComponent{
			ComponentID: c.ComponentID, Portions: c.Portions, SortOrder: i,
		})
	}
	if err := h.plates.Create(r.Context(), p); err != nil {
		status, key := toHTTPWithResource(err, "plate")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toPlateResponse(p))
}
