package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// planGenerateService is the generation surface the handler uses.
type planGenerateService interface {
	Generate(ctx context.Context, req planning.Request) (int64, error)
	Regenerate(ctx context.Context, req planning.Request) (int64, error)
	Run(ctx context.Context, id int64) (*planning.RunView, error)
	PreviewReplan(ctx context.Context, q planning.ReplanQuery) (*planning.ReplanPreview, error)
	Replan(ctx context.Context, req planning.ReplanRequest) (*planning.ReplanResult, error)
}

// PlanGenerateHandler starts generations and reports their runs (spec §3.2,
// D9): POST returns 202 with the run id at once; the run goes on without the
// request.
type PlanGenerateHandler struct {
	svc planGenerateService
	now func() time.Time
}

// NewPlanGenerateHandler creates a PlanGenerateHandler.
func NewPlanGenerateHandler(svc planGenerateService) *PlanGenerateHandler {
	return &PlanGenerateHandler{svc: svc, now: time.Now}
}

type generateRequest struct {
	Start            string             `json:"start"`
	Chooser          string             `json:"chooser"`
	Instruction      string             `json:"instruction"`
	EatersByDate     map[string]float64 `json:"eaters_by_date"`
	IncludeNewRecipe *bool              `json:"include_new_recipe"`
}

type scopeRequest struct {
	Kind        string `json:"kind"`
	Date        string `json:"date"`
	PlateID     int64  `json:"plate_id"`
	ComponentID int64  `json:"component_id"`
}

type regenerateRequest struct {
	Scope       scopeRequest `json:"scope"`
	Start       string       `json:"start"`
	Chooser     string       `json:"chooser"`
	Instruction string       `json:"instruction"`
}

type runStartedResponse struct {
	RunID int64 `json:"run_id"`
}

// Generate handles POST /api/plan/generate[?today].
func (h *PlanGenerateHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var body generateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	req, ok := h.request(w, r, body.Start, body.Chooser, body.Instruction)
	if !ok {
		return
	}
	req.EatersByDate, req.IncludeNewRecipe = body.EatersByDate, body.IncludeNewRecipe
	h.started(w, r, h.svc.Generate, req)
}

// Regenerate handles POST /api/plan/regenerate[?today].
func (h *PlanGenerateHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	var body regenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	req, ok := h.request(w, r, body.Start, body.Chooser, body.Instruction)
	if !ok {
		return
	}
	req.Scope = planning.Scope{Kind: planning.ScopeKind(body.Scope.Kind), PlateID: body.Scope.PlateID, ComponentID: body.Scope.ComponentID}
	if body.Scope.Date != "" {
		d, err := time.Parse(time.DateOnly, body.Scope.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.plan.invalid_scope")
			return
		}
		req.Scope.Date = d
	}
	h.started(w, r, h.svc.Regenerate, req)
}

func (h *PlanGenerateHandler) request(w http.ResponseWriter, r *http.Request, start, chooser, instruction string) (planning.Request, bool) {
	s, err := time.Parse(time.DateOnly, start)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.plan.invalid_start")
		return planning.Request{}, false
	}
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return planning.Request{}, false
	}
	return planning.Request{Start: s, Today: today, Chooser: airun.Chooser(chooser), Instruction: instruction}, true
}

func (h *PlanGenerateHandler) started(w http.ResponseWriter, r *http.Request, start func(context.Context, planning.Request) (int64, error), req planning.Request) {
	id, err := start(r.Context(), req)
	if err != nil {
		writePlanError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, runStartedResponse{RunID: id})
}

type planRunCounts struct {
	Plates        int `json:"plates"`
	Sessions      int `json:"sessions"`
	CookDays      int `json:"cook_days"`
	KeptConfirmed int `json:"kept_confirmed"`
}

type planRunWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type planRunResponse struct {
	ID       int64            `json:"id"`
	Kind     string           `json:"kind"`
	Status   string           `json:"status"`
	Stage    *string          `json:"stage"`
	Chooser  *string          `json:"chooser"`
	Notes    *string          `json:"notes"`
	ErrorKey *string          `json:"error_key"`
	Counts   planRunCounts    `json:"counts"`
	Issues   []planning.Issue `json:"issues"`
	Score    *planning.Score  `json:"score"`
	Window   planRunWindow    `json:"window"`
}

// Run handles GET /api/plan/runs/{id}.
func (h *PlanGenerateHandler) Run(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	v, err := h.svc.Run(r.Context(), id)
	if err != nil {
		writePlanError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, planRunResponse{
		ID: v.ID, Kind: string(v.Kind), Status: string(v.Status),
		Stage: nonEmpty(string(v.Stage)), Chooser: nonEmpty(string(v.Chooser)), Notes: nonEmpty(v.Notes), ErrorKey: nonEmpty(v.ErrorKey),
		Counts: planRunCounts{
			Plates: v.Counts.Plates, Sessions: v.Counts.Sessions, CookDays: v.Counts.CookDays, KeptConfirmed: v.Counts.KeptConfirmed,
		},
		Issues: v.Issues, Score: v.Score,
		Window: planRunWindow{Start: dateString(v.Window.Start), End: dateString(v.Window.End)},
	})
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func writePlanError(w http.ResponseWriter, err error) {
	status, key := toHTTPWithResource(err, "plan")
	writeError(w, status, key)
}

// ── Mid-week re-plan (§3.2, B8) ─────────────────────────────────────────

type replanCell struct {
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

type replanAffected struct {
	PlateID  int64  `json:"plate_id"`
	Date     string `json:"date"`
	SlotID   int64  `json:"slot_id"`
	FoodName string `json:"food_name"`
}

type replanSpare struct {
	SessionID int64   `json:"session_id"`
	FoodName  string  `json:"food_name"`
	Portions  float64 `json:"portions"`
	Freezable bool    `json:"freezable"`
	Mutable   bool    `json:"mutable"`
}

type replanPreviewResponse struct {
	NoneCells         []replanCell     `json:"none_cells"`
	ConfirmedAffected []replanAffected `json:"confirmed_affected"`
	Spare             []replanSpare    `json:"spare"`
}

type replanRequest struct {
	From            string       `json:"from"`
	To              string       `json:"to"`
	NoneCells       []replanCell `json:"none_cells"`
	UnlockConfirmed bool         `json:"unlock_confirmed"`
	Spare           string       `json:"spare"`
	Regenerate      bool         `json:"regenerate"`
	Chooser         string       `json:"chooser"`
	Instruction     string       `json:"instruction"`
}

// replanResponse carries regenerate_error (a message key) when regenerate
// was asked for but its run did not start; the removal stands.
type replanResponse struct {
	Removed         plate.PlanSnapshot `json:"removed"`
	RunID           *int64             `json:"run_id"`
	RegenerateError string             `json:"regenerate_error,omitempty"`
}

type confirmedAffectedResponse struct {
	MessageKey        string           `json:"message_key"`
	Status            int              `json:"status"`
	ConfirmedAffected []replanAffected `json:"confirmed_affected"`
}

// ReplanPreview handles GET /api/plan/replan/preview?from&to[&none=date:slot…][&today].
func (h *PlanGenerateHandler) ReplanPreview(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	var cells []replanCell
	for _, v := range qs["none"] {
		date, slot, ok := strings.Cut(v, ":")
		id, err := strconv.ParseInt(slot, 10, 64)
		if !ok || err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		cells = append(cells, replanCell{Date: date, SlotID: id})
	}
	q, ok := h.replanQuery(w, r, qs.Get("from"), qs.Get("to"), cells)
	if !ok {
		return
	}
	pv, err := h.svc.PreviewReplan(r.Context(), q)
	if err != nil {
		writePlanError(w, err)
		return
	}
	out := replanPreviewResponse{
		NoneCells: make([]replanCell, 0, len(pv.NoneCells)), ConfirmedAffected: affectedJSON(pv.ConfirmedAffected),
		Spare: make([]replanSpare, 0, len(pv.Spare)),
	}
	for _, c := range pv.NoneCells {
		out.NoneCells = append(out.NoneCells, replanCell{Date: dateString(c.Date), SlotID: c.SlotID})
	}
	for _, sp := range pv.Spare {
		out.Spare = append(out.Spare, replanSpare{SessionID: sp.SessionID, FoodName: sp.FoodName, Portions: sp.Portions, Freezable: sp.Freezable, Mutable: sp.Mutable})
	}
	writeJSON(w, http.StatusOK, out)
}

// Replan handles POST /api/plan/replan[?today]: 409 with the confirmed
// plates when they would go and unlock_confirmed is not set.
func (h *PlanGenerateHandler) Replan(w http.ResponseWriter, r *http.Request) {
	var body replanRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	q, ok := h.replanQuery(w, r, body.From, body.To, body.NoneCells)
	if !ok {
		return
	}
	res, err := h.svc.Replan(r.Context(), planning.ReplanRequest{
		ReplanQuery: q, UnlockConfirmed: body.UnlockConfirmed, Spare: planning.SpareMode(body.Spare),
		Regenerate: body.Regenerate, Chooser: airun.Chooser(body.Chooser), Instruction: body.Instruction,
	})
	var ce *planning.ConfirmedAffectedError
	if errors.As(err, &ce) {
		status, key := toHTTPWithResource(err, "plan")
		writeJSON(w, status, confirmedAffectedResponse{MessageKey: key, Status: status, ConfirmedAffected: affectedJSON(ce.Plates)})
		return
	}
	if err != nil {
		writePlanError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, replanResponse{Removed: res.Removed, RunID: res.RunID, RegenerateError: res.RegenerateError})
}

func (h *PlanGenerateHandler) replanQuery(w http.ResponseWriter, r *http.Request, from, to string, cells []replanCell) (planning.ReplanQuery, bool) {
	f, errF := time.Parse(time.DateOnly, from)
	t, errT := time.Parse(time.DateOnly, to)
	if errF != nil || errT != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return planning.ReplanQuery{}, false
	}
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return planning.ReplanQuery{}, false
	}
	q := planning.ReplanQuery{From: f, To: t, Today: today}
	for _, c := range cells {
		d, err := time.Parse(time.DateOnly, c.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return planning.ReplanQuery{}, false
		}
		q.NoneCells = append(q.NoneCells, plate.CellKey{Date: d, SlotID: c.SlotID})
	}
	return q, true
}

func affectedJSON(ps []planning.AffectedPlate) []replanAffected {
	out := make([]replanAffected, 0, len(ps))
	for _, p := range ps {
		out = append(out, replanAffected{PlateID: p.PlateID, Date: dateString(p.Date), SlotID: p.SlotID, FoodName: p.FoodName})
	}
	return out
}
