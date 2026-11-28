package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// ImportService is the port the ImportHandler calls.
type ImportService interface {
	Extract(ctx context.Context, in importer.ExtractInput) (*importer.Draft, error)
	ResolveLine(ctx context.Context, query, lang string) ([]food.Candidate, error)
	Finalize(ctx context.Context, in importer.FinalizeInput) (*importer.FinalizedFood, error)
	Check(ctx context.Context, d *importer.Draft, lang string) (*importer.Check, error)
}

// ImportHandler is the HTTP transport for the recipe import pipeline.
type ImportHandler struct {
	svc      ImportService
	outcomes importer.OutcomeStore
}

// NewImportHandler constructs an ImportHandler.
func NewImportHandler(svc ImportService) *ImportHandler {
	return &ImportHandler{svc: svc}
}

type extractRequest struct {
	URL  string `json:"url"`
	HTML string `json:"html"`
	// Lang is the user's language ("de", "en") the import check translates
	// a foreign-language recipe into; default "de".
	Lang string `json:"lang"`
}

type extractResponse struct {
	Draft *importer.Draft `json:"draft"`
	// Check is the import check (IMP): its applied changes are already in
	// the draft. Ran is false with the Chef off or no key.
	Check *importer.Check `json:"check"`
}

type checkRequest struct {
	Draft *importer.Draft `json:"draft"`
	Lang  string          `json:"lang"`
}

type importLookupResponse struct {
	Results          []food.Candidate `json:"results"`
	RecommendedIndex int              `json:"recommended_index"`
	// UnavailableReason: see lookupResponse.
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

type resolveChildRequest struct {
	Resolution string  `json:"resolution"`
	FoodID     int64   `json:"food_id"`
	Amount     float64 `json:"amount"`
	Unit       string  `json:"unit"`
	// PortionGrams is the reviewed grams per count unit; null keeps the
	// leaf's portion or the units.CountGrams estimate.
	PortionGrams *float64 `json:"portion_grams"`
	// ToTaste: a to-taste seasoning line, saved without an amount.
	ToTaste bool `json:"to_taste"`
}

type resolveInstructionRequest struct {
	StepNumber int    `json:"step_number"`
	Text       string `json:"text"`
}

type resolveRequestImport struct {
	Name              string                      `json:"name"`
	Role              string                      `json:"role"`
	ReferencePortions float64                     `json:"reference_portions"`
	PrepMinutes       *int                        `json:"prep_minutes"`
	CookMinutes       *int                        `json:"cook_minutes"`
	Notes             *string                     `json:"notes"`
	Tags              []string                    `json:"tags"`
	Instructions      []resolveInstructionRequest `json:"instructions"`
	Children          []resolveChildRequest       `json:"children"`
}

type resolveResponseImport struct {
	Food *importer.FinalizedFood `json:"food"`
}

// Extract handles POST /api/import/extract.
func (h *ImportHandler) Extract(w http.ResponseWriter, r *http.Request) {
	var req extractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if (req.URL == "" && req.HTML == "") || (req.URL != "" && req.HTML != "") {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	draft, err := h.svc.Extract(r.Context(), importer.ExtractInput{URL: req.URL, HTML: req.HTML, Options: importer.Options{Match: true}})
	reqID := middleware.GetReqID(r.Context())
	if err != nil {
		status, key := toHTTP(err)
		attrs := []any{"request_id", reqID, "outcome", "error", "message_key", key}
		var xe *importer.ExtractError
		if errors.As(err, &xe) {
			attrs = append(attrs, "stage", xe.Stage, "host", xe.Host)
			if xe.Host != "" {
				writeErrorParams(w, status, key, map[string]string{"host": xe.Host})
				slog.WarnContext(r.Context(), "import extract failed", attrs...)
				return
			}
		}
		slog.WarnContext(r.Context(), "import extract failed", attrs...)
		writeError(w, status, key)
		return
	}
	check := h.check(r.Context(), draft, req.Lang)
	slog.InfoContext(r.Context(), "import extract", "request_id", reqID, "outcome", "ok",
		"host", hostOf(req.URL), "method", draft.ExtractMethod, "ingredients", len(draft.Ingredients),
		"warnings", draft.Warnings)
	writeJSON(w, http.StatusOK, extractResponse{Draft: draft, Check: check})
}

// check runs the import check over draft. A failed check is no check: the
// import goes on without it.
func (h *ImportHandler) check(ctx context.Context, draft *importer.Draft, lang string) *importer.Check {
	if lang != "en" {
		lang = "de"
	}
	check, err := h.svc.Check(ctx, draft, lang)
	if err != nil {
		slog.WarnContext(ctx, "import check failed", "error", err)
	}
	return check
}

// Check handles POST /api/import/check: the import check over a draft the
// client already holds (a Discover add), answered like extract.
func (h *ImportHandler) Check(w http.ResponseWriter, r *http.Request) {
	var req checkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Draft == nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	check := h.check(r.Context(), req.Draft, req.Lang)
	writeJSON(w, http.StatusOK, extractResponse{Draft: req.Draft, Check: check})
}

// WithOutcomes sets the store of import-check outcomes.
func (h *ImportHandler) WithOutcomes(o importer.OutcomeStore) *ImportHandler {
	h.outcomes = o
	return h
}

type checkOutcomeRequest struct {
	RunID  int64    `json:"run_id"`
	FoodID int64    `json:"food_id"`
	Kept   []string `json:"kept"`
	Undone []string `json:"undone"`
	Touch  string   `json:"touch"`
	Flags  int      `json:"flags"`
}

// CheckOutcome handles POST /api/import/check/outcome: what the user did
// with the check's findings, sent when the recipe is saved.
func (h *ImportHandler) CheckOutcome(w http.ResponseWriter, r *http.Request) {
	var req checkOutcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RunID <= 0 || req.Flags < 0 ||
		!importer.ValidOutcomeKinds(req.Kept) || !importer.ValidOutcomeKinds(req.Undone) ||
		(req.Touch != "none" && req.Touch != "added" && req.Touch != "skipped") {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	err := h.outcomes.SaveCheckOutcome(r.Context(), importer.CheckOutcome{
		RunID: req.RunID, FoodID: req.FoodID, Kept: req.Kept, Undone: req.Undone, Touch: req.Touch, Flags: req.Flags,
	}, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Hostname()
	}
	return ""
}

// LookupLine handles GET /api/import/lookup.
func (h *ImportHandler) LookupLine(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("query")
	if q == "" {
		writeError(w, http.StatusBadRequest, "error.food.lookup.missing_param")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "de"
	}
	ctx, status := food.WithLookupStatus(r.Context())
	results, err := h.svc.ResolveLine(ctx, q, lang)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	if len(results) == 0 {
		writeJSON(w, http.StatusOK, importLookupResponse{
			Results: []food.Candidate{}, RecommendedIndex: -1, UnavailableReason: status.Reason(),
		})
		return
	}
	writeJSON(w, http.StatusOK, importLookupResponse{Results: results, RecommendedIndex: 0})
}

// Resolve handles POST /api/import/resolve.
func (h *ImportHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	var req resolveRequestImport
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	input := importer.FinalizeInput{
		Name:              req.Name,
		Role:              req.Role,
		ReferencePortions: req.ReferencePortions,
		PrepMinutes:       req.PrepMinutes,
		CookMinutes:       req.CookMinutes,
		Notes:             req.Notes,
		Tags:              req.Tags,
	}
	for _, ins := range req.Instructions {
		input.Instructions = append(input.Instructions, importer.FinalizedInstruction{
			StepNumber: ins.StepNumber, Text: ins.Text,
		})
	}
	for _, ch := range req.Children {
		input.Children = append(input.Children, importer.FinalizedChild{
			Resolution: ch.Resolution, FoodID: ch.FoodID,
			Amount: ch.Amount, Unit: ch.Unit, PortionGrams: ch.PortionGrams, ToTaste: ch.ToTaste,
		})
	}
	out, err := h.svc.Finalize(r.Context(), input)
	if err != nil {
		status, key := toHTTP(err)
		var le *importer.LineError
		if errors.As(err, &le) {
			writeErrorParams(w, status, key, map[string]string{"food": le.Food})
			return
		}
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, resolveResponseImport{Food: out})
}
