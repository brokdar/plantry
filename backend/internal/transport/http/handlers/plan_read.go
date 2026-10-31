package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// planReader is the subset of planview.Service GET /api/plan uses.
type planReader interface {
	Plan(ctx context.Context, start *time.Time, today time.Time) (*planview.Plan, error)
}

// PlanReadHandler serves GET /api/plan.
type PlanReadHandler struct {
	svc planReader
	now func() time.Time
}

// NewPlanReadHandler creates a PlanReadHandler.
func NewPlanReadHandler(svc planReader) *PlanReadHandler {
	return &PlanReadHandler{svc: svc, now: time.Now}
}

type planWindowResponse struct {
	Start       string `json:"start"`
	End         string `json:"end"`
	Today       string `json:"today"`
	IsCurrent   bool   `json:"is_current"`
	PrevStart   string `json:"prev_start"`
	NextStart   string `json:"next_start"`
	ShoppingDay int    `json:"shopping_day"`
}

type planSlotResponse struct {
	ID        int64  `json:"id"`
	NameKey   string `json:"name_key"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sort_order"`
}

type planCellResponse struct {
	Date       string         `json:"date"`
	SlotID     int64          `json:"slot_id"`
	Mode       string         `json:"mode"`
	Source     string         `json:"source"`
	PresetID   *int64         `json:"preset_id"`
	PresetName *string        `json:"preset_name"`
	Note       *string        `json:"note"`
	Plate      *plateResponse `json:"plate"`
}

type dayMacrosResponse struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
}

type planDayResponse struct {
	Date     string            `json:"date"`
	Macros   dayMacrosResponse `json:"macros"`
	Complete bool              `json:"complete"`
	Missing  []string          `json:"missing"`
}

type planTargetsResponse struct {
	Kcal     float64 `json:"kcal"`
	ProteinG float64 `json:"protein_g"`
	FatG     float64 `json:"fat_g"`
	CarbsG   float64 `json:"carbs_g"`
}

type planStatsResponse struct {
	Plates   int `json:"plates"`
	CookDays int `json:"cook_days"`
	Warnings int `json:"warnings"`
}

type planResponse struct {
	Window   planWindowResponse   `json:"window"`
	Slots    []planSlotResponse   `json:"slots"`
	Cells    []planCellResponse   `json:"cells"`
	Sessions []sessionResponse    `json:"sessions"`
	Days     []planDayResponse    `json:"days"`
	Targets  *planTargetsResponse `json:"targets"`
	Stats    planStatsResponse    `json:"stats"`
}

// Get handles GET /api/plan?start&today. Without start it opens the anchored
// window; today defaults to the server's local date (D25).
func (h *PlanReadHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	today, ok := parseToday(w, q.Get("today"), h.now)
	if !ok {
		return
	}
	var start *time.Time
	if s := q.Get("start"); s != "" {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.plan.invalid_start")
			return
		}
		start = &d
	}
	p, err := h.svc.Plan(r.Context(), start, today)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlanResponse(p))
}

func toPlanResponse(p *planview.Plan) planResponse {
	resp := planResponse{
		Window: planWindowResponse{
			Start: dateString(p.Window.Start), End: dateString(p.Window.End), Today: dateString(p.Today),
			IsCurrent: p.IsCurrent, PrevStart: dateString(p.Window.Prev()), NextStart: dateString(p.Window.Next()),
			ShoppingDay: p.ShoppingDay,
		},
		Slots:    make([]planSlotResponse, len(p.Slots)),
		Cells:    make([]planCellResponse, len(p.Cells)),
		Sessions: make([]sessionResponse, len(p.Sessions)),
		Days:     make([]planDayResponse, len(p.Days)),
		Stats:    planStatsResponse{Plates: p.Stats.Plates, CookDays: p.Stats.CookDays, Warnings: p.Stats.Warnings},
	}
	for i, s := range p.Slots {
		resp.Slots[i] = planSlotResponse{ID: s.ID, NameKey: s.NameKey, Icon: s.Icon, SortOrder: s.SortOrder}
	}
	for i, c := range p.Cells {
		resp.Cells[i] = planCellResponse{
			Date: dateString(c.Date), SlotID: c.SlotID, Mode: string(c.Mode), Source: string(c.Source),
			PresetID: c.PresetID, PresetName: c.PresetName, Note: c.Note, Plate: optionalPlate(c.Plate),
		}
	}
	for i := range p.Sessions {
		resp.Sessions[i] = toSessionResponse(&p.Sessions[i])
	}
	for i, d := range p.Days {
		resp.Days[i] = planDayResponse{
			Date:     dateString(d.Date),
			Macros:   dayMacrosResponse{Kcal: d.Macros.Kcal, Protein: d.Macros.Protein, Fat: d.Macros.Fat, Carbs: d.Macros.Carbs},
			Complete: d.Complete, Missing: d.Missing,
		}
	}
	if t := p.Targets; t != nil {
		resp.Targets = &planTargetsResponse{Kcal: t.Kcal, ProteinG: t.ProteinG, FatG: t.FatG, CarbsG: t.CarbsG}
	}
	return resp
}

func optionalPlate(p *plate.Plate) *plateResponse {
	if p == nil {
		return nil
	}
	resp := toPlateResponse(p)
	return &resp
}

// parseToday parses the optional today parameter (D25); empty means the
// server's local date.
func parseToday(w http.ResponseWriter, s string, now func() time.Time) (time.Time, bool) {
	if s == "" {
		n := now()
		return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC), true
	}
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return time.Time{}, false
	}
	return d, true
}

func dateString(t time.Time) string { return t.Format(time.DateOnly) }
