package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
)

// ComponentHandler holds the HTTP handlers for the component resource.
type ComponentHandler struct {
	svc   *component.Service
	store *imagestore.Store
}

// NewComponentHandler creates a new ComponentHandler.
func NewComponentHandler(svc *component.Service, store *imagestore.Store) *ComponentHandler {
	return &ComponentHandler{svc: svc, store: store}
}

// HasImageStore reports whether image upload/delete is available.
func (h *ComponentHandler) HasImageStore() bool {
	return h.store != nil
}

// --- request / response DTOs ---

type componentIngredientRequest struct {
	IngredientID int64   `json:"ingredient_id"`
	Amount       float64 `json:"amount"`
	Unit         string  `json:"unit"`
	Grams        float64 `json:"grams"`
	SortOrder    int     `json:"sort_order"`
}

type instructionRequest struct {
	StepNumber int    `json:"step_number"`
	Text       string `json:"text"`
}

type componentRequest struct {
	Name              string                       `json:"name"`
	Role              string                       `json:"role"`
	ReferencePortions float64                      `json:"reference_portions"`
	PrepMinutes       *int                         `json:"prep_minutes"`
	CookMinutes       *int                         `json:"cook_minutes"`
	Notes             *string                      `json:"notes"`
	Ingredients       []componentIngredientRequest `json:"ingredients"`
	Instructions      []instructionRequest         `json:"instructions"`
	Tags              []string                     `json:"tags"`
}

type componentIngredientResponse struct {
	ID             int64   `json:"id"`
	ComponentID    int64   `json:"component_id"`
	IngredientID   int64   `json:"ingredient_id"`
	IngredientName string  `json:"ingredient_name"`
	Amount         float64 `json:"amount"`
	Unit           string  `json:"unit"`
	Grams          float64 `json:"grams"`
	SortOrder      int     `json:"sort_order"`
}

type instructionResponse struct {
	ID          int64  `json:"id"`
	ComponentID int64  `json:"component_id"`
	StepNumber  int    `json:"step_number"`
	Text        string `json:"text"`
}

type componentResponse struct {
	ID                int64                         `json:"id"`
	Name              string                        `json:"name"`
	Role              string                        `json:"role"`
	VariantGroupID    *int64                        `json:"variant_group_id,omitempty"`
	ReferencePortions float64                       `json:"reference_portions"`
	PrepMinutes       *int                          `json:"prep_minutes"`
	CookMinutes       *int                          `json:"cook_minutes"`
	ImagePath         *string                       `json:"image_path,omitempty"`
	Notes             *string                       `json:"notes,omitempty"`
	LastCookedAt      *string                       `json:"last_cooked_at,omitempty"`
	CookCount         int                           `json:"cook_count"`
	Ingredients       []componentIngredientResponse `json:"ingredients"`
	Instructions      []instructionResponse         `json:"instructions"`
	Tags              []string                      `json:"tags"`
	CreatedAt         string                        `json:"created_at"`
	UpdatedAt         string                        `json:"updated_at"`
}

type componentListResponse struct {
	Items []componentResponse `json:"items"`
	Total int                 `json:"total"`
}

type nutritionResponse struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
	Fiber   float64 `json:"fiber"`
	Sodium  float64 `json:"sodium"`
}

func toComponentResponse(c *component.Component) componentResponse {
	ingredients := make([]componentIngredientResponse, len(c.Ingredients))
	for i, ci := range c.Ingredients {
		ingredients[i] = componentIngredientResponse{
			ID: ci.ID, ComponentID: ci.ComponentID, IngredientID: ci.IngredientID,
			IngredientName: ci.IngredientName,
			Amount:         ci.Amount, Unit: ci.Unit, Grams: ci.Grams, SortOrder: ci.SortOrder,
		}
	}

	instructions := make([]instructionResponse, len(c.Instructions))
	for i, inst := range c.Instructions {
		instructions[i] = instructionResponse{
			ID: inst.ID, ComponentID: inst.ComponentID,
			StepNumber: inst.StepNumber, Text: inst.Text,
		}
	}

	tags := c.Tags
	if tags == nil {
		tags = []string{}
	}

	resp := componentResponse{
		ID:                c.ID,
		Name:              c.Name,
		Role:              string(c.Role),
		VariantGroupID:    c.VariantGroupID,
		ReferencePortions: c.ReferencePortions,
		PrepMinutes:       c.PrepMinutes,
		CookMinutes:       c.CookMinutes,
		ImagePath:         c.ImagePath,
		Notes:             c.Notes,
		CookCount:         c.CookCount,
		Ingredients:       ingredients,
		Instructions:      instructions,
		Tags:              tags,
		CreatedAt:         c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         c.UpdatedAt.Format(time.RFC3339),
	}
	if c.LastCookedAt != nil {
		s := c.LastCookedAt.Format(time.RFC3339)
		resp.LastCookedAt = &s
	}
	return resp
}

func componentError(err error) (int, string) {
	return toHTTPWithResource(err, "component")
}

// --- handlers ---

func (h *ComponentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req componentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	c := requestToComponent(&req)
	if err := h.svc.Create(r.Context(), c); err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusCreated, toComponentResponse(c))
}

func (h *ComponentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	c, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusOK, toComponentResponse(c))
}

func (h *ComponentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	var req componentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	c := requestToComponent(&req)
	c.ID = id
	if err := h.svc.Update(r.Context(), c); err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusOK, toComponentResponse(c))
}

func (h *ComponentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ComponentHandler) List(w http.ResponseWriter, r *http.Request) {
	q := component.ListQuery{
		Search:   r.URL.Query().Get("search"),
		Role:     r.URL.Query().Get("role"),
		Tag:      r.URL.Query().Get("tag"),
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
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	items := make([]componentResponse, len(result.Items))
	for i := range result.Items {
		items[i] = toComponentResponse(&result.Items[i])
	}

	writeJSON(w, http.StatusOK, componentListResponse{Items: items, Total: result.Total})
}

func (h *ComponentHandler) Nutrition(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	macros, err := h.svc.Nutrition(r.Context(), id)
	if err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusOK, nutritionResponse{
		Kcal:    macros.Kcal,
		Protein: macros.Protein,
		Fat:     macros.Fat,
		Carbs:   macros.Carbs,
		Fiber:   macros.Fiber,
		Sodium:  macros.Sodium,
	})
}

type variantListResponse struct {
	Items []componentResponse `json:"items"`
}

// componentSummary is a slim shape for rotation-insight cards; it omits
// ingredients, instructions, and tags (the badge UI doesn't render them).
type componentSummary struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Role         string  `json:"role"`
	ImagePath    *string `json:"image_path,omitempty"`
	CookCount    int     `json:"cook_count"`
	LastCookedAt *string `json:"last_cooked_at,omitempty"`
}

type insightsResponse struct {
	Forgotten  []componentSummary `json:"forgotten"`
	MostCooked []componentSummary `json:"most_cooked"`
}

func toComponentSummary(c *component.Component) componentSummary {
	s := componentSummary{
		ID:        c.ID,
		Name:      c.Name,
		Role:      string(c.Role),
		ImagePath: c.ImagePath,
		CookCount: c.CookCount,
	}
	if c.LastCookedAt != nil {
		str := c.LastCookedAt.Format(time.RFC3339)
		s.LastCookedAt = &str
	}
	return s
}

// Insights handles GET /api/components/insights.
func (h *ComponentHandler) Insights(w http.ResponseWriter, r *http.Request) {
	q := component.InsightsQuery{}
	if v := r.URL.Query().Get("forgotten_weeks"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.ForgottenWeeks = n
		}
	}
	if v := r.URL.Query().Get("forgotten_limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.ForgottenLimit = n
		}
	}
	if v := r.URL.Query().Get("most_cooked_limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.MostCookedLimit = n
		}
	}

	out, err := h.svc.Insights(r.Context(), q)
	if err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	forgotten := make([]componentSummary, len(out.Forgotten))
	for i := range out.Forgotten {
		forgotten[i] = toComponentSummary(&out.Forgotten[i])
	}
	mostCooked := make([]componentSummary, len(out.MostCooked))
	for i := range out.MostCooked {
		mostCooked[i] = toComponentSummary(&out.MostCooked[i])
	}

	writeJSON(w, http.StatusOK, insightsResponse{Forgotten: forgotten, MostCooked: mostCooked})
}

// CreateVariant handles POST /api/components/{id}/variant.
func (h *ComponentHandler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	variant, err := h.svc.CreateVariant(r.Context(), id)
	if err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusCreated, toComponentResponse(variant))
}

// ListVariants handles GET /api/components/{id}/variants.
func (h *ComponentHandler) ListVariants(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	siblings, err := h.svc.ListVariants(r.Context(), id)
	if err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	items := make([]componentResponse, len(siblings))
	for i := range siblings {
		items[i] = toComponentResponse(&siblings[i])
	}

	writeJSON(w, http.StatusOK, variantListResponse{Items: items})
}

// Upload handles POST /api/components/{id}/image.
func (h *ComponentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	c, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	if err := r.ParseMultipartForm(maxImageSize); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	defer func() { _ = file.Close() }()

	imgPath, err := h.store.SaveUpload(r.Context(), file, "components", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	c.ImagePath = &imgPath
	if err := h.svc.Update(r.Context(), c); err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"image_path": imgPath})
}

// DeleteImage handles DELETE /api/components/{id}/image.
func (h *ComponentHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	c, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	if err := h.store.Delete("components", id); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	c.ImagePath = nil
	if err := h.svc.Update(r.Context(), c); err != nil {
		status, key := componentError(err)
		writeError(w, status, key)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func requestToComponent(req *componentRequest) *component.Component {
	ingredients := make([]component.ComponentIngredient, len(req.Ingredients))
	for i, ci := range req.Ingredients {
		ingredients[i] = component.ComponentIngredient{
			IngredientID: ci.IngredientID,
			Amount:       ci.Amount,
			Unit:         ci.Unit,
			Grams:        ci.Grams,
			SortOrder:    ci.SortOrder,
		}
	}

	instructions := make([]component.Instruction, len(req.Instructions))
	for i, inst := range req.Instructions {
		instructions[i] = component.Instruction{
			StepNumber: inst.StepNumber,
			Text:       inst.Text,
		}
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	return &component.Component{
		Name:              req.Name,
		Role:              component.Role(req.Role),
		ReferencePortions: req.ReferencePortions,
		PrepMinutes:       req.PrepMinutes,
		CookMinutes:       req.CookMinutes,
		Notes:             req.Notes,
		Ingredients:       ingredients,
		Instructions:      instructions,
		Tags:              tags,
	}
}
