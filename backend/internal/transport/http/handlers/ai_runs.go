package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

// runsReader is the subset of airun.Insights the runs endpoints use.
type runsReader interface {
	Runs(ctx context.Context, q airun.RunQuery) (airun.RunsPage, error)
	Run(ctx context.Context, id int64) (airun.RunDetail, error)
	Weekly(ctx context.Context, from, to time.Time) (airun.Report, error)
	Price(ctx context.Context, model string) (airun.PriceInfo, error)
}

const (
	defaultRunsLimit = 50
	maxRunsLimit     = 200
	// defaultInsightsWeeks is the range GET /api/ai/insights covers without
	// from/to.
	defaultInsightsWeeks = 12
)

// AIRunsHandler serves the AI run log: /api/ai/runs, /api/ai/insights and
// /api/ai/prices.
type AIRunsHandler struct {
	svc   runsReader
	model func(ctx context.Context) (string, error)
	now   func() time.Time
}

// NewAIRunsHandler creates the handler. model returns the configured AI model,
// the default of GET /api/ai/prices.
func NewAIRunsHandler(svc runsReader, model func(ctx context.Context) (string, error)) *AIRunsHandler {
	return &AIRunsHandler{svc: svc, model: model, now: time.Now}
}

type runTokensResponse struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
}

type runSummaryResponse struct {
	ID              int64             `json:"id"`
	Kind            string            `json:"kind"`
	StartedAt       string            `json:"started_at"`
	FinishedAt      *string           `json:"finished_at"`
	Status          string            `json:"status"`
	Provider        *string           `json:"provider"`
	Model           *string           `json:"model"`
	PromptVersion   *string           `json:"prompt_version"`
	Chooser         *string           `json:"chooser"`
	Tokens          runTokensResponse `json:"tokens"`
	CostEstimateUSD *float64          `json:"cost_estimate_usd"`
	Score           json.RawMessage   `json:"score"`
	Outcome         *string           `json:"outcome"`
	CorrectionRate  *float64          `json:"correction_rate"`
}

type runsPageResponse struct {
	Items      []runSummaryResponse `json:"items"`
	NextCursor *int64               `json:"next_cursor"`
}

type stepTokensResponse struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

// runStepResponse carries args and result as text: stored payloads are cut
// to 8192 characters, so they are not always valid JSON.
type runStepResponse struct {
	Seq             int                `json:"seq"`
	Type            string             `json:"type"`
	Name            string             `json:"name"`
	Args            *string            `json:"args"`
	ArgsTruncated   bool               `json:"args_truncated"`
	Result          *string            `json:"result"`
	ResultTruncated bool               `json:"result_truncated"`
	IsError         bool               `json:"is_error"`
	DurationMs      int64              `json:"duration_ms"`
	Tokens          stepTokensResponse `json:"tokens"`
}

type runDetailResponse struct {
	runSummaryResponse
	Scope       json.RawMessage   `json:"scope"`
	Instruction *string           `json:"instruction"`
	Notes       *string           `json:"notes"`
	Violations  json.RawMessage   `json:"violations"`
	ErrorKey    *string           `json:"error_key"`
	Steps       []runStepResponse `json:"steps"`
	PromptText  *string           `json:"prompt_text"`
}

type insightsWeekResponse struct {
	Start           string   `json:"start"`
	Runs            int      `json:"runs"`
	Accepted        int      `json:"accepted"`
	Discarded       int      `json:"discarded"`
	CorrectionRate  *float64 `json:"correction_rate"`
	CostUSD         float64  `json:"cost_usd"`
	CostPerAccepted *float64 `json:"cost_per_accepted"`
	UnpricedRuns    int      `json:"unpriced_runs"`
}

type insightsVersionResponse struct {
	Kind          string             `json:"kind"`
	PromptVersion string             `json:"prompt_version"`
	Model         string             `json:"model"`
	Runs          int                `json:"runs"`
	ScoreAvg      map[string]float64 `json:"score_avg"`
	Violations    map[string]int     `json:"violations"`
}

type insightsResponse struct {
	Weeks      []insightsWeekResponse    `json:"weeks"`
	ByVersion  []insightsVersionResponse `json:"by_version"`
	PricesAsOf string                    `json:"prices_as_of"`
}

type priceResponse struct {
	Model  string       `json:"model"`
	Price  *airun.Price `json:"price"`
	Source string       `json:"source"`
	AsOf   string       `json:"as_of"`
}

// List handles GET /api/ai/runs?kind&from&to&cursor&limit.
func (h *AIRunsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := airun.RunQuery{Kind: airun.Kind(q.Get("kind")), Limit: defaultRunsLimit}
	if query.Kind != "" && !query.Kind.Valid() {
		writeError(w, http.StatusBadRequest, "error.invalid_query")
		return
	}
	from, ok := optionalDate(w, q.Get("from"), "error.invalid_from")
	if !ok {
		return
	}
	to, ok := optionalDate(w, q.Get("to"), "error.invalid_to")
	if !ok {
		return
	}
	if from != nil && to != nil && from.After(*to) {
		writeError(w, http.StatusBadRequest, "error.invalid_date_range")
		return
	}
	query.From = from
	if to != nil {
		end := to.AddDate(0, 0, 1)
		query.To = &end
	}
	if s := q.Get("cursor"); s != "" {
		c, err := strconv.ParseInt(s, 10, 64)
		if err != nil || c <= 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_query")
			return
		}
		query.BeforeID = c
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxRunsLimit {
			writeError(w, http.StatusBadRequest, "error.invalid_query")
			return
		}
		query.Limit = n
	}
	page, err := h.svc.Runs(r.Context(), query)
	if err != nil {
		writeRunsError(w, err)
		return
	}
	resp := runsPageResponse{Items: make([]runSummaryResponse, len(page.Items)), NextCursor: page.NextCursor}
	for i, s := range page.Items {
		resp.Items[i] = toRunSummaryResponse(s)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /api/ai/runs/{id}.
func (h *AIRunsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	d, err := h.svc.Run(r.Context(), id)
	if err != nil {
		writeRunsError(w, err)
		return
	}
	resp := runDetailResponse{
		runSummaryResponse: toRunSummaryResponse(d.RunSummary),
		Scope:              rawJSONOrNull(d.ScopeJSON),
		Instruction:        nilIfEmpty(d.Instruction),
		Notes:              nilIfEmpty(d.Notes),
		Violations:         rawJSONOrNull(d.ViolationsJSON),
		ErrorKey:           nilIfEmpty(d.ErrorKey),
		Steps:              make([]runStepResponse, len(d.Steps)),
		PromptText:         nilIfEmpty(d.PromptText),
	}
	for i, s := range d.Steps {
		resp.Steps[i] = runStepResponse{
			Seq:             s.Seq,
			Type:            string(s.Type),
			Name:            s.Name,
			Args:            nilIfEmpty(s.Args),
			ArgsTruncated:   s.ArgsTruncated,
			Result:          nilIfEmpty(s.Result),
			ResultTruncated: s.ResultTruncated,
			IsError:         s.IsError,
			DurationMs:      s.DurationMs,
			Tokens:          stepTokensResponse{Input: s.InputTokens, Output: s.OutputTokens},
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// Insights handles GET /api/ai/insights?from&to&bucket=week. Without from/to
// it covers the last 12 weeks up to today.
func (h *AIRunsHandler) Insights(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if b := q.Get("bucket"); b != "" && b != "week" {
		writeError(w, http.StatusBadRequest, "error.invalid_query")
		return
	}
	from, ok := optionalDate(w, q.Get("from"), "error.invalid_from")
	if !ok {
		return
	}
	to, ok := optionalDate(w, q.Get("to"), "error.invalid_to")
	if !ok {
		return
	}
	if to == nil {
		n := h.now()
		today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
		to = &today
	}
	if from == nil {
		f := to.AddDate(0, 0, -7*defaultInsightsWeeks+1)
		from = &f
	}
	if from.After(*to) {
		writeError(w, http.StatusBadRequest, "error.invalid_date_range")
		return
	}
	if to.Sub(*from) > 366*24*time.Hour {
		writeError(w, http.StatusBadRequest, "error.date_range_too_large")
		return
	}
	rep, err := h.svc.Weekly(r.Context(), *from, *to)
	if err != nil {
		writeRunsError(w, err)
		return
	}
	resp := insightsResponse{
		Weeks:      make([]insightsWeekResponse, len(rep.Weeks)),
		ByVersion:  make([]insightsVersionResponse, len(rep.ByVersion)),
		PricesAsOf: rep.PricesAsOf,
	}
	for i, wk := range rep.Weeks {
		resp.Weeks[i] = insightsWeekResponse{
			Start:           dateString(wk.Start),
			Runs:            wk.Runs,
			Accepted:        wk.Accepted,
			Discarded:       wk.Discarded,
			CorrectionRate:  wk.CorrectionRate,
			CostUSD:         wk.CostUSD,
			CostPerAccepted: wk.CostPerAccepted,
			UnpricedRuns:    wk.UnpricedRuns,
		}
	}
	for i, g := range rep.ByVersion {
		resp.ByVersion[i] = insightsVersionResponse{
			Kind: string(g.Kind), PromptVersion: g.PromptVersion, Model: g.Model, Runs: g.Runs,
			ScoreAvg: g.ScoreAvg, Violations: g.Violations,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// Prices handles GET /api/ai/prices?model=; the model defaults to the
// configured one.
func (h *AIRunsHandler) Prices(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")
	if model == "" {
		m, err := h.model(r.Context())
		if err != nil {
			writeRunsError(w, err)
			return
		}
		model = m
	}
	info, err := h.svc.Price(r.Context(), model)
	if err != nil {
		writeRunsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, priceResponse{Model: info.Model, Price: info.Price, Source: string(info.Source), AsOf: info.AsOf})
}

func toRunSummaryResponse(s airun.RunSummary) runSummaryResponse {
	resp := runSummaryResponse{
		ID:            s.ID,
		Kind:          string(s.Kind),
		StartedAt:     s.StartedAt.Format(time.RFC3339),
		Status:        string(s.Status),
		Provider:      nilIfEmpty(s.Provider),
		Model:         nilIfEmpty(s.Model),
		PromptVersion: nilIfEmpty(s.PromptVersion),
		Chooser:       nilIfEmpty(string(s.Chooser)),
		Tokens: runTokensResponse{
			Input: s.Tokens.Input, Output: s.Tokens.Output,
			CacheRead: s.Tokens.CacheRead, CacheWrite: s.Tokens.CacheWrite,
		},
		CostEstimateUSD: s.CostUSD,
		Score:           rawJSONOrNull(s.ScoreJSON),
		Outcome:         nilIfEmpty(s.Outcome),
		CorrectionRate:  s.CorrectionRate,
	}
	if s.FinishedAt != nil {
		f := s.FinishedAt.Format(time.RFC3339)
		resp.FinishedAt = &f
	}
	return resp
}

// optionalDate parses a YYYY-MM-DD query value; empty is nil. It writes 400
// with key on a malformed date.
func optionalDate(w http.ResponseWriter, s, key string) (*time.Time, bool) {
	if s == "" {
		return nil, true
	}
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		writeError(w, http.StatusBadRequest, key)
		return nil, false
	}
	return &d, true
}

// rawJSONOrNull passes stored JSON through; empty or invalid JSON is null.
func rawJSONOrNull(s string) json.RawMessage {
	if s == "" || !json.Valid([]byte(s)) {
		return json.RawMessage("null")
	}
	return json.RawMessage(s)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func writeRunsError(w http.ResponseWriter, err error) {
	status, key := toHTTPWithResource(err, "ai_run")
	writeError(w, status, key)
}
