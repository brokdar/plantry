package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// IngredientHandler holds the HTTP handlers for the ingredient resource.
type IngredientHandler struct {
	svc *ingredient.Service
}

// NewIngredientHandler creates a new IngredientHandler.
func NewIngredientHandler(svc *ingredient.Service) *IngredientHandler {
	return &IngredientHandler{svc: svc}
}

type ingredientRequest struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	Barcode     *string `json:"barcode"`
	OffID       *string `json:"off_id"`
	FdcID       *string `json:"fdc_id"`
	ImagePath   *string `json:"image_path"`
	Kcal100g    float64 `json:"kcal_100g"`
	Protein100g float64 `json:"protein_100g"`
	Fat100g     float64 `json:"fat_100g"`
	Carbs100g   float64 `json:"carbs_100g"`
	Fiber100g   float64 `json:"fiber_100g"`
	Sodium100g  float64 `json:"sodium_100g"`
}

type ingredientResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	Barcode     *string `json:"barcode,omitempty"`
	OffID       *string `json:"off_id,omitempty"`
	FdcID       *string `json:"fdc_id,omitempty"`
	ImagePath   *string `json:"image_path,omitempty"`
	Kcal100g    float64 `json:"kcal_100g"`
	Protein100g float64 `json:"protein_100g"`
	Fat100g     float64 `json:"fat_100g"`
	Carbs100g   float64 `json:"carbs_100g"`
	Fiber100g   float64 `json:"fiber_100g"`
	Sodium100g  float64 `json:"sodium_100g"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type listResponse struct {
	Items []ingredientResponse `json:"items"`
	Total int                  `json:"total"`
}

func toResponse(i *ingredient.Ingredient) ingredientResponse {
	return ingredientResponse{
		ID:          i.ID,
		Name:        i.Name,
		Source:      i.Source,
		Barcode:     i.Barcode,
		OffID:       i.OffID,
		FdcID:       i.FdcID,
		ImagePath:   i.ImagePath,
		Kcal100g:    i.Kcal100g,
		Protein100g: i.Protein100g,
		Fat100g:     i.Fat100g,
		Carbs100g:   i.Carbs100g,
		Fiber100g:   i.Fiber100g,
		Sodium100g:  i.Sodium100g,
		CreatedAt:   i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   i.UpdatedAt.Format(time.RFC3339),
	}
}

func (h *IngredientHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req ingredientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	i := &ingredient.Ingredient{
		Name:        req.Name,
		Source:      req.Source,
		Barcode:     req.Barcode,
		OffID:       req.OffID,
		FdcID:       req.FdcID,
		ImagePath:   req.ImagePath,
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

	writeJSON(w, http.StatusCreated, toResponse(i))
}

func (h *IngredientHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	i, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(i))
}

func (h *IngredientHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	var req ingredientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	i := &ingredient.Ingredient{
		ID:          id,
		Name:        req.Name,
		Source:      req.Source,
		Barcode:     req.Barcode,
		OffID:       req.OffID,
		FdcID:       req.FdcID,
		ImagePath:   req.ImagePath,
		Kcal100g:    req.Kcal100g,
		Protein100g: req.Protein100g,
		Fat100g:     req.Fat100g,
		Carbs100g:   req.Carbs100g,
		Fiber100g:   req.Fiber100g,
		Sodium100g:  req.Sodium100g,
	}

	if err := h.svc.Update(r.Context(), i); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(i))
}

func (h *IngredientHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *IngredientHandler) List(w http.ResponseWriter, r *http.Request) {
	q := ingredient.ListQuery{
		Search:   r.URL.Query().Get("search"),
		SortBy:   r.URL.Query().Get("sort"),
		SortDesc: r.URL.Query().Get("order") == "desc",
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Offset = n
		}
	}

	result, err := h.svc.List(r.Context(), q)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	items := make([]ingredientResponse, len(result.Items))
	for idx := range result.Items {
		items[idx] = toResponse(&result.Items[idx])
	}

	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: result.Total})
}
