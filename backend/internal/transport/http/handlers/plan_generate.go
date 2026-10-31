package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// planGenerateService is the generation surface the handler uses.
type planGenerateService interface {
	Generate(ctx context.Context, req planning.Request) (int64, error)
	Regenerate(ctx context.Context, req planning.Request) (int64, error)
	Run(ctx context.Context, id int64) (*planning.RunView, error)
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
