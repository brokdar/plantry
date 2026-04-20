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

	SaturatedFat100g *float64 `json:"saturated_fat_100g"`
	TransFat100g     *float64 `json:"trans_fat_100g"`
	Cholesterol100g  *float64 `json:"cholesterol_100g"`
	Sugar100g        *float64 `json:"sugar_100g"`
	Potassium100g    *float64 `json:"potassium_100g"`
	Calcium100g      *float64 `json:"calcium_100g"`
	Iron100g         *float64 `json:"iron_100g"`
	Magnesium100g    *float64 `json:"magnesium_100g"`
	Phosphorus100g   *float64 `json:"phosphorus_100g"`
	Zinc100g         *float64 `json:"zinc_100g"`
	VitaminA100g     *float64 `json:"vitamin_a_100g"`
	VitaminC100g     *float64 `json:"vitamin_c_100g"`
	VitaminD100g     *float64 `json:"vitamin_d_100g"`
	VitaminB12100g   *float64 `json:"vitamin_b12_100g"`
	VitaminB6100g    *float64 `json:"vitamin_b6_100g"`
	Folate100g       *float64 `json:"folate_100g"`
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

	SaturatedFat100g *float64 `json:"saturated_fat_100g,omitempty"`
	TransFat100g     *float64 `json:"trans_fat_100g,omitempty"`
	Cholesterol100g  *float64 `json:"cholesterol_100g,omitempty"`
	Sugar100g        *float64 `json:"sugar_100g,omitempty"`
	Potassium100g    *float64 `json:"potassium_100g,omitempty"`
	Calcium100g      *float64 `json:"calcium_100g,omitempty"`
	Iron100g         *float64 `json:"iron_100g,omitempty"`
	Magnesium100g    *float64 `json:"magnesium_100g,omitempty"`
	Phosphorus100g   *float64 `json:"phosphorus_100g,omitempty"`
	Zinc100g         *float64 `json:"zinc_100g,omitempty"`
	VitaminA100g     *float64 `json:"vitamin_a_100g,omitempty"`
	VitaminC100g     *float64 `json:"vitamin_c_100g,omitempty"`
	VitaminD100g     *float64 `json:"vitamin_d_100g,omitempty"`
	VitaminB12100g   *float64 `json:"vitamin_b12_100g,omitempty"`
	VitaminB6100g    *float64 `json:"vitamin_b6_100g,omitempty"`
	Folate100g       *float64 `json:"folate_100g,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type listResponse struct {
	Items []ingredientResponse `json:"items"`
	Total int                  `json:"total"`
}

func toResponse(i *ingredient.Ingredient) ingredientResponse {
	return ingredientResponse{
		ID:               i.ID,
		Name:             i.Name,
		Source:           i.Source,
		Barcode:          i.Barcode,
		OffID:            i.OffID,
		FdcID:            i.FdcID,
		ImagePath:        i.ImagePath,
		Kcal100g:         i.Kcal100g,
		Protein100g:      i.Protein100g,
		Fat100g:          i.Fat100g,
		Carbs100g:        i.Carbs100g,
		Fiber100g:        i.Fiber100g,
		Sodium100g:       i.Sodium100g,
		SaturatedFat100g: i.SaturatedFat100g,
		TransFat100g:     i.TransFat100g,
		Cholesterol100g:  i.Cholesterol100g,
		Sugar100g:        i.Sugar100g,
		Potassium100g:    i.Potassium100g,
		Calcium100g:      i.Calcium100g,
		Iron100g:         i.Iron100g,
		Magnesium100g:    i.Magnesium100g,
		Phosphorus100g:   i.Phosphorus100g,
		Zinc100g:         i.Zinc100g,
		VitaminA100g:     i.VitaminA100g,
		VitaminC100g:     i.VitaminC100g,
		VitaminD100g:     i.VitaminD100g,
		VitaminB12100g:   i.VitaminB12100g,
		VitaminB6100g:    i.VitaminB6100g,
		Folate100g:       i.Folate100g,
		CreatedAt:        i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        i.UpdatedAt.Format(time.RFC3339),
	}
}

func applyExtendedNutrients(i *ingredient.Ingredient, req *ingredientRequest) {
	i.SaturatedFat100g = req.SaturatedFat100g
	i.TransFat100g = req.TransFat100g
	i.Cholesterol100g = req.Cholesterol100g
	i.Sugar100g = req.Sugar100g
	i.Potassium100g = req.Potassium100g
	i.Calcium100g = req.Calcium100g
	i.Iron100g = req.Iron100g
	i.Magnesium100g = req.Magnesium100g
	i.Phosphorus100g = req.Phosphorus100g
	i.Zinc100g = req.Zinc100g
	i.VitaminA100g = req.VitaminA100g
	i.VitaminC100g = req.VitaminC100g
	i.VitaminD100g = req.VitaminD100g
	i.VitaminB12100g = req.VitaminB12100g
	i.VitaminB6100g = req.VitaminB6100g
	i.Folate100g = req.Folate100g
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
	applyExtendedNutrients(i, &req)

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

	// image_path is managed via dedicated /image endpoints; preserve it here so
	// a regular PUT (which doesn't carry the field) cannot wipe the stored image.
	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	i := &ingredient.Ingredient{
		ID:          id,
		Name:        req.Name,
		Source:      req.Source,
		Barcode:     req.Barcode,
		OffID:       req.OffID,
		FdcID:       req.FdcID,
		ImagePath:   existing.ImagePath,
		Kcal100g:    req.Kcal100g,
		Protein100g: req.Protein100g,
		Fat100g:     req.Fat100g,
		Carbs100g:   req.Carbs100g,
		Fiber100g:   req.Fiber100g,
		Sodium100g:  req.Sodium100g,
	}
	applyExtendedNutrients(i, &req)

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
