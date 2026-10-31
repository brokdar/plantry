package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
)

// todayReader is the subset of planview.Service GET /api/today uses.
type todayReader interface {
	Today(ctx context.Context, date time.Time) (*planview.Today, error)
}

// TodayHandler serves GET /api/today.
type TodayHandler struct {
	svc todayReader
	now func() time.Time
}

// NewTodayHandler creates a TodayHandler.
func NewTodayHandler(svc todayReader) *TodayHandler {
	return &TodayHandler{svc: svc, now: time.Now}
}

type todayVerdictResponse struct {
	Kind      string   `json:"kind"`
	SessionID *int64   `json:"session_id,omitempty"`
	FoodName  *string  `json:"food_name,omitempty"`
	Portions  *float64 `json:"portions,omitempty"`
	SlotID    *int64   `json:"slot_id,omitempty"`
}

type todayCookResponse struct {
	Session sessionResponse `json:"session"`
	Minutes *int            `json:"minutes"`
}

type todayMealResponse struct {
	SlotID     int64                `json:"slot_id"`
	Mode       string               `json:"mode"`
	PresetName *string              `json:"preset_name"`
	Note       *string              `json:"note"`
	Plate      *plateResponse       `json:"plate"`
	Link       *sessionLinkResponse `json:"link"`
}

type todayReminderResponse struct {
	Kind      string `json:"kind"`
	SessionID int64  `json:"session_id"`
	FoodName  string `json:"food_name"`
	Date      string `json:"date"`
	SlotID    int64  `json:"slot_id"`
}

type todayRateResponse struct {
	PlateID  int64  `json:"plate_id"`
	Date     string `json:"date"`
	SlotID   int64  `json:"slot_id"`
	FoodName string `json:"food_name"`
}

type todayPlanBannerResponse struct {
	Due       bool   `json:"due"`
	NextStart string `json:"next_start"`
}

type todaySetupResponse struct {
	RecipeCount  int `json:"recipe_count"`
	RecipeTarget int `json:"recipe_target"`
}

type todayResponse struct {
	Date        string                  `json:"date"`
	WindowStart string                  `json:"window_start"`
	Verdict     todayVerdictResponse    `json:"verdict"`
	Cook        []todayCookResponse     `json:"cook"`
	Meals       []todayMealResponse     `json:"meals"`
	Reminders   []todayReminderResponse `json:"reminders"`
	Rate        *todayRateResponse      `json:"rate"`
	PlanBanner  todayPlanBannerResponse `json:"plan_banner"`
	Setup       todaySetupResponse      `json:"setup"`
}

// Get handles GET /api/today?date&today. date defaults to today, which
// defaults to the server's local date (D25).
func (h *TodayHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	today, ok := parseToday(w, q.Get("today"), h.now)
	if !ok {
		return
	}
	date := today
	if s := q.Get("date"); s != "" {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		date = d
	}
	td, err := h.svc.Today(r.Context(), date)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTodayResponse(td))
}

func toTodayResponse(td *planview.Today) todayResponse {
	resp := todayResponse{
		Date: dateString(td.Date), WindowStart: dateString(td.WindowStart),
		Verdict:    todayVerdictResponse{Kind: td.Verdict.Kind},
		Cook:       make([]todayCookResponse, len(td.Cook)),
		Meals:      make([]todayMealResponse, len(td.Meals)),
		Reminders:  make([]todayReminderResponse, len(td.Reminders)),
		PlanBanner: todayPlanBannerResponse{Due: td.PlanBanner.Due, NextStart: dateString(td.PlanBanner.NextStart)},
		Setup:      todaySetupResponse{RecipeCount: td.Setup.RecipeCount, RecipeTarget: td.Setup.RecipeTarget},
	}
	if v := td.Verdict; v.Kind == planview.VerdictCook || v.Kind == planview.VerdictQuickCook {
		resp.Verdict.SessionID, resp.Verdict.FoodName = &v.SessionID, &v.FoodName
		resp.Verdict.Portions, resp.Verdict.SlotID = &v.Portions, &v.SlotID
	}
	for i := range td.Cook {
		resp.Cook[i] = todayCookResponse{Session: toSessionResponse(&td.Cook[i].Session), Minutes: td.Cook[i].Minutes}
	}
	for i, m := range td.Meals {
		resp.Meals[i] = todayMealResponse{
			SlotID: m.SlotID, Mode: string(m.Mode), PresetName: m.PresetName, Note: m.Note,
			Plate: optionalPlate(m.Plate),
		}
		if m.Link != nil {
			l := toSessionLinkResponse(*m.Link)
			resp.Meals[i].Link = &l
		}
	}
	for i, rm := range td.Reminders {
		resp.Reminders[i] = todayReminderResponse{
			Kind: rm.Kind, SessionID: rm.SessionID, FoodName: rm.FoodName, Date: dateString(rm.Date), SlotID: rm.SlotID,
		}
	}
	if rt := td.Rate; rt != nil {
		resp.Rate = &todayRateResponse{PlateID: rt.PlateID, Date: dateString(rt.Date), SlotID: rt.SlotID, FoodName: rt.FoodName}
	}
	return resp
}
