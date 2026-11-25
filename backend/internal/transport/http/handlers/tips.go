package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
	"github.com/jaltszeimer/plantry/backend/internal/domain/tips"
)

// TipService is the tips service as the handler uses it (tips.Service).
type TipService interface {
	Sync(ctx context.Context, date time.Time) (tips.View, error)
	Apply(ctx context.Context, id int64) (plate.Undo, error)
	Dismiss(ctx context.Context, id int64) error
	Counts(ctx context.Context, from, to time.Time) (tips.Counts, error)
}

// TipPlates reads the plates of a range for the trial export.
type TipPlates interface {
	Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

// KindSpend is one capped run kind's stored cost this month.
type KindSpend struct {
	Kind airun.Kind `json:"kind"`
	USD  float64    `json:"usd"`
}

// TipHandler serves /api/tips and the Chef's Insights block and trial
// export under /api/chef.
type TipHandler struct {
	svc    TipService
	plates TipPlates
	// spend returns the capped spend by kind for the month of t.
	spend func(ctx context.Context, t time.Time) ([]KindSpend, error)
	now   func() time.Time
}

// NewTipHandler creates the handler.
func NewTipHandler(svc TipService, plates TipPlates, spend func(ctx context.Context, t time.Time) ([]KindSpend, error)) *TipHandler {
	return &TipHandler{svc: svc, plates: plates, spend: spend, now: time.Now}
}

// WithClock sets the handler's clock (the dev clock).
func (h *TipHandler) WithClock(now func() time.Time) *TipHandler {
	h.now = now
	return h
}

type tipResponse struct {
	ID        int64              `json:"id"`
	PlateID   *int64             `json:"plate_id"`
	Kind      tips.Kind          `json:"kind"`
	Dimension platefit.Dimension `json:"dimension"`
	State     tips.State         `json:"state"`
	Action    string             `json:"action"`
	FoodID    int64              `json:"food_id"`
	FoodName  string             `json:"food_name"`
	FromName  string             `json:"from_name,omitempty"`
	Grams     float64            `json:"grams,omitempty"`
	CreatedAt string             `json:"created_at"`
	UpdatedAt string             `json:"updated_at"`
}

func toTipResponse(t tips.Tip) tipResponse {
	return tipResponse{
		ID: t.ID, PlateID: t.PlateID, Kind: t.Kind, Dimension: t.Dimension, State: t.State,
		Action: t.Op.Action, FoodID: t.Op.FoodID, FoodName: t.FoodName, FromName: t.FromName, Grams: t.Op.Grams,
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

type tipsViewResponse struct {
	On    bool            `json:"on"`
	Start string          `json:"start,omitempty"`
	Tips  []tipResponse   `json:"tips"`
	Fit   []tips.PlateFit `json:"fit"`
}

func (h *TipHandler) dateParam(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	if raw := r.URL.Query().Get("date"); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_date")
			return time.Time{}, false
		}
		return d, true
	}
	n := h.now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC), true
}

// List handles GET /api/tips?date=: the live tips of the window that holds
// date (default today) and each plate's fit; nothing while the Chef is off.
func (h *TipHandler) List(w http.ResponseWriter, r *http.Request) {
	date, ok := h.dateParam(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Sync(r.Context(), date)
	if err != nil {
		slog.ErrorContext(r.Context(), "tips.sync", "error", err)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	resp := tipsViewResponse{On: v.On, Tips: make([]tipResponse, 0, len(v.Tips)), Fit: v.Fit}
	if resp.Fit == nil {
		resp.Fit = []tips.PlateFit{}
	}
	if v.On {
		resp.Start = v.Start.Format(time.DateOnly)
	}
	for _, t := range v.Tips {
		resp.Tips = append(resp.Tips, toTipResponse(t))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *TipHandler) tipError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, tips.ErrConfirmed):
		writeError(w, http.StatusConflict, "error.tip.confirmed")
	case errors.Is(err, tips.ErrStale):
		writeError(w, http.StatusConflict, "error.tip.stale")
	default:
		status, key := toHTTPWithResource(err, "tip")
		if status == http.StatusInternalServerError {
			slog.ErrorContext(r.Context(), "tips.write", "error", err)
		}
		writeError(w, status, key)
	}
}

// Apply handles POST /api/tips/{id}/apply: the tip's change on its plate and
// the Undo token POST /api/plan/revert takes.
func (h *TipHandler) Apply(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	undo, err := h.svc.Apply(r.Context(), id)
	if err != nil {
		h.tipError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]plate.Undo{"undo": undoOf(undo)})
}

// Dismiss handles POST /api/tips/{id}/dismiss.
func (h *TipHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.Dismiss(r.Context(), id); err != nil {
		h.tipError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type chefInsightsResponse struct {
	From  string      `json:"from"`
	To    string      `json:"to"`
	Tips  tips.Counts `json:"tips"`
	Month string      `json:"month"`
	Spend []KindSpend `json:"spend"`
}

// Insights handles GET /api/chef/insights?from&to (§10, first metrics): the
// tips created in the range by state (default the last 8 weeks) and this
// month's capped spend by kind from the stored run costs.
func (h *TipHandler) Insights(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, ok := optionalDate(w, q.Get("from"), "error.invalid_from")
	if !ok {
		return
	}
	to, ok := optionalDate(w, q.Get("to"), "error.invalid_to")
	if !ok {
		return
	}
	now := h.now()
	if to == nil {
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		to = &t
	}
	if from == nil {
		f := to.AddDate(0, 0, -7*defaultInsightsWeeks+1)
		from = &f
	}
	counts, err := h.svc.Counts(r.Context(), *from, to.AddDate(0, 0, 1))
	if err != nil {
		slog.ErrorContext(r.Context(), "chef.insights.tips", "error", err)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	spend, err := h.spend(r.Context(), now)
	if err != nil {
		slog.ErrorContext(r.Context(), "chef.insights.spend", "error", err)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, chefInsightsResponse{
		From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Tips: counts,
		Month: now.Format("2006-01"), Spend: spend,
	})
}

type exportComponent struct {
	FoodID   int64   `json:"food_id"`
	Name     string  `json:"name"`
	Position string  `json:"position"`
	Portions float64 `json:"portions,omitempty"`
	Grams    float64 `json:"grams,omitempty"`
}

type exportPlate struct {
	ID         int64             `json:"id"`
	Date       string            `json:"date"`
	SlotID     int64             `json:"slot_id"`
	State      string            `json:"state"`
	Eaters     float64           `json:"eaters"`
	Rating     string            `json:"rating,omitempty"`
	Raised     []string          `json:"raised"`
	Components []exportComponent `json:"components"`
}

type trialExportResponse struct {
	Start  string        `json:"start"`
	End    string        `json:"end"`
	Plates []exportPlate `json:"plates"`
	Tips   []tipResponse `json:"tips"`
	Spend  []KindSpend   `json:"spend"`
}

// TrialExport handles GET /api/chef/trial-export?date=: one window as the
// time-lapse driver records it (user review B3): its plates with their
// components, rating and plate-fit reading, every tip of the window and the capped
// spend by kind of the window's month. Empty tips while the Chef is off.
func (h *TipHandler) TrialExport(w http.ResponseWriter, r *http.Request) {
	date, ok := h.dateParam(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Sync(r.Context(), date)
	if err != nil {
		slog.ErrorContext(r.Context(), "chef.export.tips", "error", err)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	start := v.Start
	if !v.On {
		start = date
	}
	end := start.AddDate(0, 0, 6)
	plates, err := h.plates.Range(r.Context(), start, end)
	if err != nil {
		slog.ErrorContext(r.Context(), "chef.export.plates", "error", err)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	spend, err := h.spend(r.Context(), start)
	if err != nil {
		slog.ErrorContext(r.Context(), "chef.export.spend", "error", err)
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	raised := map[int64][]string{}
	for _, f := range v.Fit {
		ds := make([]string, len(f.Raised))
		for i, d := range f.Raised {
			ds[i] = string(d)
		}
		raised[f.PlateID] = ds
	}
	resp := trialExportResponse{
		Start: start.Format(time.DateOnly), End: end.Format(time.DateOnly),
		Plates: make([]exportPlate, 0, len(plates)), Tips: make([]tipResponse, 0, len(v.Tips)), Spend: spend,
	}
	for _, p := range plates {
		ep := exportPlate{
			ID: p.ID, Date: p.DateString(), SlotID: p.SlotID, State: string(p.State), Eaters: p.Eaters,
			Raised: raised[p.ID], Components: make([]exportComponent, 0, len(p.Components)),
		}
		if ep.Raised == nil {
			ep.Raised = []string{}
		}
		if p.Feedback != nil {
			ep.Rating = p.Feedback.Status
		}
		for _, c := range p.Components {
			ec := exportComponent{FoodID: c.FoodID, Name: c.FoodName, Position: string(c.Position)}
			if c.Portions != nil {
				ec.Portions = *c.Portions
			}
			if c.Grams != nil {
				ec.Grams = *c.Grams
			}
			ep.Components = append(ep.Components, ec)
		}
		resp.Plates = append(resp.Plates, ep)
	}
	for _, t := range append(v.Tips, v.Ended...) {
		resp.Tips = append(resp.Tips, toTipResponse(t))
	}
	writeJSON(w, http.StatusOK, resp)
}
