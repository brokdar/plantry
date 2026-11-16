package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// ImportService is the port the ImportHandler calls.
type ImportService interface {
	Extract(ctx context.Context, in importer.ExtractInput) (*importer.Draft, error)
	ResolveLine(ctx context.Context, query, lang string) ([]food.Candidate, error)
	Finalize(ctx context.Context, in importer.FinalizeInput) (*importer.FinalizedFood, error)
}

// ImportHandler is the HTTP transport for the recipe import pipeline.
type ImportHandler struct {
	svc ImportService
}

// NewImportHandler constructs an ImportHandler.
func NewImportHandler(svc ImportService) *ImportHandler {
	return &ImportHandler{svc: svc}
}

type extractRequest struct {
	URL  string `json:"url"`
	HTML string `json:"html"`
}

type extractResponse struct {
	Draft *importer.Draft `json:"draft"`
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
	draft, err := h.svc.Extract(r.Context(), importer.ExtractInput{URL: req.URL, HTML: req.HTML})
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
	slog.InfoContext(r.Context(), "import extract", "request_id", reqID, "outcome", "ok",
		"host", hostOf(req.URL), "method", draft.ExtractMethod, "ingredients", len(draft.Ingredients),
		"warnings", draft.Warnings)
	writeJSON(w, http.StatusOK, extractResponse{Draft: draft})
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
			Amount: ch.Amount, Unit: ch.Unit, PortionGrams: ch.PortionGrams,
		})
	}
	out, err := h.svc.Finalize(r.Context(), input)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, resolveResponseImport{Food: out})
}
