package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// LookupHandler holds HTTP handlers for ingredient lookup and resolve.
type LookupHandler struct {
	resolver *ingredient.Resolver
	imgStore *imagestore.Store
	svc      *ingredient.Service
}

// NewLookupHandler creates a new LookupHandler.
func NewLookupHandler(resolver *ingredient.Resolver, imgStore *imagestore.Store, svc *ingredient.Service) *LookupHandler {
	return &LookupHandler{resolver: resolver, imgStore: imgStore, svc: svc}
}

type lookupResponse struct {
	Results          []ingredient.Candidate `json:"results"`
	RecommendedIndex int                    `json:"recommended_index"`
}

// Lookup handles GET /api/ingredients/lookup.
func (h *LookupHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	barcode := r.URL.Query().Get("barcode")
	query := r.URL.Query().Get("query")
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}

	if barcode == "" && query == "" {
		writeError(w, http.StatusBadRequest, "error.ingredient.lookup.missing_param")
		return
	}

	results, err := h.resolver.Lookup(r.Context(), barcode, query, lang, 5)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	if len(results) == 0 {
		writeError(w, http.StatusNotFound, "error.ingredient.lookup.not_found")
		return
	}

	writeJSON(w, http.StatusOK, lookupResponse{
		Results:          results,
		RecommendedIndex: 0,
	})
}

type resolveRequest struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	Barcode     *string `json:"barcode"`
	FdcID       *string `json:"fdc_id"`
	ImageURL    *string `json:"image_url"`
	Kcal100g    float64 `json:"kcal_100g"`
	Protein100g float64 `json:"protein_100g"`
	Fat100g     float64 `json:"fat_100g"`
	Carbs100g   float64 `json:"carbs_100g"`
	Fiber100g   float64 `json:"fiber_100g"`
	Sodium100g  float64 `json:"sodium_100g"`
	Portions    []struct {
		Unit  string  `json:"unit"`
		Grams float64 `json:"grams"`
	} `json:"portions"`
}

// Resolve handles POST /api/ingredients/resolve.
func (h *LookupHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	i := &ingredient.Ingredient{
		Name:        req.Name,
		Source:      req.Source,
		Barcode:     req.Barcode,
		FdcID:       req.FdcID,
		Kcal100g:    req.Kcal100g,
		Protein100g: req.Protein100g,
		Fat100g:     req.Fat100g,
		Carbs100g:   req.Carbs100g,
		Fiber100g:   req.Fiber100g,
		Sodium100g:  req.Sodium100g,
	}

	if err := h.svc.Create(r.Context(), i); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	// Download image if URL is provided and image store is available.
	if req.ImageURL != nil && *req.ImageURL != "" && h.imgStore != nil {
		imgPath, err := h.imgStore.SaveFromURL(r.Context(), *req.ImageURL, "ingredients", i.ID)
		if err == nil {
			i.ImagePath = &imgPath
			// Best-effort update; ignore errors from image path save.
			_ = h.svc.Update(r.Context(), i)
		}
	}

	writeJSON(w, http.StatusCreated, toResponse(i))
}
