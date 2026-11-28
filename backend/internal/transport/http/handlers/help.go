package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chefhelp"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// helpService is the subset of chefhelp.Service the endpoint uses.
type helpService interface {
	Help(ctx context.Context, req chefhelp.Request) (chefhelp.Answer, error)
}

// HelpHandler serves the cook-mode model-backed help (HELP-2).
type HelpHandler struct {
	svc      helpService
	verdicts chefhelp.VerdictStore
}

// NewHelpHandler creates a HelpHandler.
func NewHelpHandler(svc helpService) *HelpHandler { return &HelpHandler{svc: svc} }

type helpRequest struct {
	SessionID int64  `json:"session_id"`
	StepIndex int    `json:"step_index"`
	Kind      string `json:"kind"`
	Problem   string `json:"problem"`
	Text      string `json:"text"`
}

// Help handles POST /api/help: one answer of at most three lines for the
// step being cooked. Never a cap dialog (HELP-8).
func (h *HelpHandler) Help(w http.ResponseWriter, r *http.Request) {
	var req helpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	ans, err := h.svc.Help(r.Context(), chefhelp.Request{
		SessionID: req.SessionID, StepIndex: req.StepIndex, Kind: req.Kind, Problem: req.Problem, Text: req.Text,
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"answer": ans.Lines, "run_id": ans.RunID})
	case errors.Is(err, chefhelp.ErrChefOff):
		writeError(w, http.StatusConflict, "error.chef.off")
	case errors.Is(err, llm.ErrProviderMissing):
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
	case errors.Is(err, chefhelp.ErrModel):
		writeError(w, http.StatusBadGateway, "error.ai.provider_error")
	default:
		writePlateError(w, err)
	}
}

// WithVerdicts sets the store of the one-tap help verdicts.
func (h *HelpHandler) WithVerdicts(v chefhelp.VerdictStore) *HelpHandler {
	h.verdicts = v
	return h
}

// Verdict handles PUT /api/chef/help/{runId}/verdict: the one-tap "Helped" /
// "Didn't help" under an answer; a later call changes it.
func (h *HelpHandler) Verdict(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	var req struct {
		Verdict string `json:"verdict"`
	}
	if err != nil || runID <= 0 || json.NewDecoder(r.Body).Decode(&req) != nil ||
		(req.Verdict != chefhelp.VerdictHelped && req.Verdict != chefhelp.VerdictNotHelped) {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	switch err := h.verdicts.SetVerdict(r.Context(), runID, req.Verdict, time.Now().UTC()); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "error.not_found")
	default:
		writePlateError(w, err)
	}
}
