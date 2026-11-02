package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// discoveryService is the part of discovery.Service the endpoints use.
type discoveryService interface {
	Discover(ctx context.Context, req discovery.Request, today time.Time) (int64, error)
	RunState(ctx context.Context, id int64) (discovery.RunState, error)
	Daily(ctx context.Context, day time.Time) (discovery.Daily, error)
	Another(ctx context.Context, day time.Time) (discovery.Daily, error)
	Suggestions(ctx context.Context, status *discovery.Status, runID *int64) ([]discovery.Suggestion, error)
	Accept(ctx context.Context, id, foodID int64) (*discovery.Suggestion, error)
	Reject(ctx context.Context, id int64, reason *discovery.Reason) (*discovery.Suggestion, error)
}

// DiscoveryHandler serves /api/discovery (Phase 5 §3.1).
type DiscoveryHandler struct {
	svc discoveryService
	now func() time.Time
}

// NewDiscoveryHandler creates the handler.
func NewDiscoveryHandler(svc discoveryService) *DiscoveryHandler {
	return &DiscoveryHandler{svc: svc, now: time.Now}
}

type discoverRequest struct {
	Mode    discovery.Mode `json:"mode"`
	Cuisine string         `json:"cuisine"`
	Focus   string         `json:"focus"`
	Today   string         `json:"today"`
}

type runIDResponse struct {
	RunID int64 `json:"run_id"`
}

type discoveryRunResponse struct {
	RunID         int64   `json:"run_id"`
	Status        string  `json:"status"`
	SuggestionIDs []int64 `json:"suggestion_ids"`
	Discarded     int     `json:"discarded"`
	ErrorKey      *string `json:"error_key"`
}

type suggestionMacrosResponse struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
}

type suggestionIngredientResponse struct {
	RawText string   `json:"raw_text"`
	Name    string   `json:"name"`
	Amount  *float64 `json:"amount"`
	Unit    *string  `json:"unit"`
	FoodID  *int64   `json:"food_id"`
	Known   bool     `json:"known"`
}

type suggestionResponse struct {
	ID                int64                          `json:"id"`
	Mode              string                         `json:"mode"`
	Query             *string                        `json:"query"`
	SourceURL         string                         `json:"source_url"`
	ImageURL          *string                        `json:"image_url"`
	Grounded          bool                           `json:"grounded"`
	Status            string                         `json:"status"`
	Reason            *string                        `json:"reason"`
	RunID             *int64                         `json:"run_id"`
	FoodID            *int64                         `json:"food_id"`
	ShownOn           *string                        `json:"shown_on"`
	CreatedAt         string                         `json:"created_at"`
	Name              string                         `json:"name"`
	Cuisine           *string                        `json:"cuisine"`
	TotalMinutes      *int                           `json:"total_minutes"`
	ReferencePortions float64                        `json:"reference_portions"`
	DietClass         diet.Class                     `json:"diet_class"`
	PerPortion        suggestionMacrosResponse       `json:"per_portion"`
	NutritionComplete bool                           `json:"nutrition_complete"`
	Missing           []string                       `json:"missing"`
	Why               string                         `json:"why"`
	KnownShare        float64                        `json:"known_share"`
	Ingredients       []suggestionIngredientResponse `json:"ingredients"`
	Draft             importer.Draft                 `json:"draft"`
}

type suggestionEnvelope struct {
	Suggestion suggestionResponse `json:"suggestion"`
}

type suggestionsResponse struct {
	Items []suggestionResponse `json:"items"`
}

func toSuggestionResponse(s *discovery.Suggestion) suggestionResponse {
	d, ann := s.Draft, s.Annotations
	out := suggestionResponse{
		ID: s.ID, Mode: string(s.Mode), Query: s.Query, SourceURL: s.SourceURL, Grounded: s.Grounded,
		Status: string(s.Status), RunID: s.RunID, FoodID: s.FoodID, ShownOn: s.ShownOn,
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339),
		Name:      d.Name, Cuisine: optString(ann.Cuisine), TotalMinutes: d.TotalMinutes,
		ReferencePortions: d.ReferencePortions,
		PerPortion: suggestionMacrosResponse{
			Kcal: ann.PerPortion.Kcal, Protein: ann.PerPortion.Protein, Fat: ann.PerPortion.Fat, Carbs: ann.PerPortion.Carbs,
		},
		NutritionComplete: ann.NutritionComplete, Missing: ann.Missing, Why: ann.Why, KnownShare: ann.KnownShare,
		Ingredients: make([]suggestionIngredientResponse, len(d.Ingredients)), Draft: d,
	}
	if out.Missing == nil {
		out.Missing = []string{}
	}
	if s.ImagePath != nil {
		u := "/images/" + *s.ImagePath
		out.ImageURL = &u
	}
	if s.Reason != nil {
		r := string(*s.Reason)
		out.Reason = &r
	}
	classes := make([]diet.Class, len(d.Ingredients))
	for i, ing := range d.Ingredients {
		line := suggestionIngredientResponse{RawText: ing.RawText, Name: ing.Name, Unit: optString(ing.Unit)}
		if ing.Amount != 0 {
			line.Amount = &ing.Amount
		}
		if i < len(ann.Lines) {
			line.FoodID, line.Known = ann.Lines[i].FoodID, ann.Lines[i].Known
		}
		out.Ingredients[i] = line
		classes[i] = ing.DietClass
	}
	out.DietClass = diet.Max(classes...)
	return out
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// writeDaily answers a daily endpoint: 200 {suggestion}, 202 {run_id} or
// 204 (silent).
func writeDaily(w http.ResponseWriter, d discovery.Daily) {
	switch {
	case d.Suggestion != nil:
		writeJSON(w, http.StatusOK, suggestionEnvelope{Suggestion: toSuggestionResponse(d.Suggestion)})
	case d.RunID != 0:
		writeJSON(w, http.StatusAccepted, runIDResponse{RunID: d.RunID})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeDiscoveryError(w http.ResponseWriter, err error) {
	status, key := toHTTPWithResource(err, "discovery")
	writeError(w, status, key)
}

// Start handles POST /api/discovery.
func (h *DiscoveryHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req discoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	today, ok := parseToday(w, req.Today, h.now)
	if !ok {
		return
	}
	id, err := h.svc.Discover(r.Context(), discovery.Request{Mode: req.Mode, Cuisine: req.Cuisine, Focus: req.Focus}, today)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, runIDResponse{RunID: id})
}

// Run handles GET /api/discovery/runs/{id}.
func (h *DiscoveryHandler) Run(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	st, err := h.svc.RunState(r.Context(), id)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, discoveryRunResponse{
		RunID: st.RunID, Status: st.Status, SuggestionIDs: st.SuggestionIDs, Discarded: st.Discarded, ErrorKey: st.ErrorKey,
	})
}

// parseDay parses a required YYYY-MM-DD date.
func parseDay(w http.ResponseWriter, s string) (time.Time, bool) {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return time.Time{}, false
	}
	return d, true
}

// Daily handles GET /api/discovery/daily?date= (D12).
func (h *DiscoveryHandler) Daily(w http.ResponseWriter, r *http.Request) {
	day, ok := parseDay(w, r.URL.Query().Get("date"))
	if !ok {
		return
	}
	d, err := h.svc.Daily(r.Context(), day)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	writeDaily(w, d)
}

// Another handles POST /api/discovery/daily/another {date} (D12).
func (h *DiscoveryHandler) Another(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	day, ok := parseDay(w, req.Date)
	if !ok {
		return
	}
	d, err := h.svc.Another(r.Context(), day)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	writeDaily(w, d)
}

// List handles GET /api/discovery/suggestions?status=&run_id=.
func (h *DiscoveryHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var status *discovery.Status
	if v := q.Get("status"); v != "" {
		s := discovery.Status(v)
		if s != discovery.StatusPending && s != discovery.StatusAccepted && s != discovery.StatusRejected {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		status = &s
	}
	var runID *int64
	if v := q.Get("run_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		runID = &n
	}
	sugs, err := h.svc.Suggestions(r.Context(), status, runID)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	out := suggestionsResponse{Items: make([]suggestionResponse, len(sugs))}
	for i := range sugs {
		out.Items[i] = toSuggestionResponse(&sugs[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// Accept handles POST /api/discovery/suggestions/{id}/accept {food_id}.
func (h *DiscoveryHandler) Accept(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		FoodID int64 `json:"food_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FoodID <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	sug, err := h.svc.Accept(r.Context(), id, req.FoodID)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestionEnvelope{Suggestion: toSuggestionResponse(sug)})
}

// Reject handles POST /api/discovery/suggestions/{id}/reject {reason?}; an
// empty body means no reason.
func (h *DiscoveryHandler) Reject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason *discovery.Reason `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.Reason != nil {
		switch *req.Reason {
		case discovery.ReasonTooLong, discovery.ReasonIngredients, discovery.ReasonTaste:
		default:
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
	}
	sug, err := h.svc.Reject(r.Context(), id, req.Reason)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestionEnvelope{Suggestion: toSuggestionResponse(sug)})
}

// pathID parses the {id} URL parameter.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return 0, false
	}
	return id, true
}
