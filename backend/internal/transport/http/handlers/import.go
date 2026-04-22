package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// ImportService is the port the ImportHandler calls. The concrete implementation
// is domain/importer.Service.
type ImportService interface {
	Extract(ctx context.Context, in importer.ExtractInput) (*importer.Draft, error)
	ResolveLine(ctx context.Context, query, lang string) ([]ingredient.Candidate, error)
	Finalize(ctx context.Context, in importer.FinalizeInput) (*importer.FinalizedComponent, error)
}

// ImportHandler is the HTTP transport for the recipe import pipeline.
type ImportHandler struct {
	svc ImportService
}

// NewImportHandler constructs an ImportHandler.
func NewImportHandler(svc ImportService) *ImportHandler {
	return &ImportHandler{svc: svc}
}

// -- DTOs --

type extractRequest struct {
	URL  string `json:"url"`
	HTML string `json:"html"`
}

type extractResponse struct {
	Draft *importer.Draft `json:"draft"`
}

type lookupResponseImport struct {
	Results          []ingredient.Candidate `json:"results"`
	RecommendedIndex int                    `json:"recommended_index"`
}

type resolveIngredientRequest struct {
	Resolution   string  `json:"resolution"`
	IngredientID int64   `json:"existing_ingredient_id"`
	Amount       float64 `json:"amount"`
	Unit         string  `json:"unit"`
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
	Ingredients       []resolveIngredientRequest  `json:"ingredients"`
}

type resolveResponseImport struct {
	Component *importer.FinalizedComponent `json:"component"`
}

// -- handlers --

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
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, extractResponse{Draft: draft})
}

// LookupLine handles GET /api/import/lookup.
func (h *ImportHandler) LookupLine(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("query")
	if q == "" {
		writeError(w, http.StatusBadRequest, "error.ingredient.lookup.missing_param")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "de"
	}

	results, err := h.svc.ResolveLine(r.Context(), q, lang)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	if len(results) == 0 {
		writeJSON(w, http.StatusOK, lookupResponseImport{
			Results:          []ingredient.Candidate{},
			RecommendedIndex: -1,
		})
		return
	}
	writeJSON(w, http.StatusOK, lookupResponseImport{
		Results:          results,
		RecommendedIndex: 0,
	})
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
	for _, ing := range req.Ingredients {
		input.Ingredients = append(input.Ingredients, importer.FinalizedIngredient{
			Resolution:   ing.Resolution,
			IngredientID: ing.IngredientID,
			Amount:       ing.Amount,
			Unit:         ing.Unit,
		})
	}

	out, err := h.svc.Finalize(r.Context(), input)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, resolveResponseImport{Component: out})
}
