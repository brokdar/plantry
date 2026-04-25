package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// FoodHandler hosts the HTTP surface for the unified food resource.
type FoodHandler struct {
	svc      *food.Service
	resolver *food.NutritionResolver
	store    *imagestore.Store
}

const maxImageSize = 10 << 20 // 10 MB

// NewFoodHandler wires services into the handler.
func NewFoodHandler(svc *food.Service, resolver *food.NutritionResolver, store *imagestore.Store) *FoodHandler {
	return &FoodHandler{svc: svc, resolver: resolver, store: store}
}

// HasImageStore reports whether image upload/delete is available.
func (h *FoodHandler) HasImageStore() bool { return h.store != nil }

// ── DTOs ──────────────────────────────────────────────────────────────

type foodChildRequest struct {
	ChildID   int64   `json:"child_id"`
	Amount    float64 `json:"amount"`
	Unit      string  `json:"unit"`
	Grams     float64 `json:"grams"`
	SortOrder int     `json:"sort_order"`
}

type foodInstructionRequest struct {
	StepNumber int    `json:"step_number"`
	Text       string `json:"text"`
}

type foodRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`

	// LEAF
	Source           string   `json:"source,omitempty"`
	Barcode          *string  `json:"barcode,omitempty"`
	OffID            *string  `json:"off_id,omitempty"`
	FdcID            *string  `json:"fdc_id,omitempty"`
	Kcal100g         *float64 `json:"kcal_100g,omitempty"`
	Protein100g      *float64 `json:"protein_100g,omitempty"`
	Fat100g          *float64 `json:"fat_100g,omitempty"`
	Carbs100g        *float64 `json:"carbs_100g,omitempty"`
	Fiber100g        *float64 `json:"fiber_100g,omitempty"`
	Sodium100g       *float64 `json:"sodium_100g,omitempty"`
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

	// COMPOSED
	Role              string                   `json:"role,omitempty"`
	ReferencePortions *float64                 `json:"reference_portions,omitempty"`
	PrepMinutes       *int                     `json:"prep_minutes,omitempty"`
	CookMinutes       *int                     `json:"cook_minutes,omitempty"`
	Notes             *string                  `json:"notes,omitempty"`
	Children          []foodChildRequest       `json:"children,omitempty"`
	Instructions      []foodInstructionRequest `json:"instructions,omitempty"`
	Tags              []string                 `json:"tags,omitempty"`
}

type foodChildResponse struct {
	ID          int64   `json:"id"`
	ParentID    int64   `json:"parent_id"`
	ChildID     int64   `json:"child_id"`
	ChildName   string  `json:"child_name"`
	ChildKind   string  `json:"child_kind"`
	Amount      float64 `json:"amount"`
	Unit        string  `json:"unit"`
	Grams       float64 `json:"grams"`
	GramsSource string  `json:"grams_source,omitempty"`
	SortOrder   int     `json:"sort_order"`
}

type foodInstructionResponse struct {
	ID         int64  `json:"id"`
	FoodID     int64  `json:"food_id"`
	StepNumber int    `json:"step_number"`
	Text       string `json:"text"`
}

type foodPortionResponse struct {
	FoodID int64   `json:"food_id"`
	Unit   string  `json:"unit"`
	Grams  float64 `json:"grams"`
}

type foodResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`

	// LEAF fields (nil/zero for composed)
	Source           *string               `json:"source,omitempty"`
	Barcode          *string               `json:"barcode,omitempty"`
	OffID            *string               `json:"off_id,omitempty"`
	FdcID            *string               `json:"fdc_id,omitempty"`
	Kcal100g         *float64              `json:"kcal_100g,omitempty"`
	Protein100g      *float64              `json:"protein_100g,omitempty"`
	Fat100g          *float64              `json:"fat_100g,omitempty"`
	Carbs100g        *float64              `json:"carbs_100g,omitempty"`
	Fiber100g        *float64              `json:"fiber_100g,omitempty"`
	Sodium100g       *float64              `json:"sodium_100g,omitempty"`
	SaturatedFat100g *float64              `json:"saturated_fat_100g,omitempty"`
	TransFat100g     *float64              `json:"trans_fat_100g,omitempty"`
	Cholesterol100g  *float64              `json:"cholesterol_100g,omitempty"`
	Sugar100g        *float64              `json:"sugar_100g,omitempty"`
	Potassium100g    *float64              `json:"potassium_100g,omitempty"`
	Calcium100g      *float64              `json:"calcium_100g,omitempty"`
	Iron100g         *float64              `json:"iron_100g,omitempty"`
	Magnesium100g    *float64              `json:"magnesium_100g,omitempty"`
	Phosphorus100g   *float64              `json:"phosphorus_100g,omitempty"`
	Zinc100g         *float64              `json:"zinc_100g,omitempty"`
	VitaminA100g     *float64              `json:"vitamin_a_100g,omitempty"`
	VitaminC100g     *float64              `json:"vitamin_c_100g,omitempty"`
	VitaminD100g     *float64              `json:"vitamin_d_100g,omitempty"`
	VitaminB12100g   *float64              `json:"vitamin_b12_100g,omitempty"`
	VitaminB6100g    *float64              `json:"vitamin_b6_100g,omitempty"`
	Folate100g       *float64              `json:"folate_100g,omitempty"`
	Portions         []foodPortionResponse `json:"portions,omitempty"`

	// COMPOSED fields
	Role              *string                   `json:"role,omitempty"`
	VariantGroupID    *int64                    `json:"variant_group_id,omitempty"`
	ReferencePortions *float64                  `json:"reference_portions,omitempty"`
	PrepMinutes       *int                      `json:"prep_minutes,omitempty"`
	CookMinutes       *int                      `json:"cook_minutes,omitempty"`
	Notes             *string                   `json:"notes,omitempty"`
	Children          []foodChildResponse       `json:"children,omitempty"`
	Instructions      []foodInstructionResponse `json:"instructions,omitempty"`
	Tags              []string                  `json:"tags,omitempty"`

	// Shared
	ImagePath    *string `json:"image_path,omitempty"`
	Favorite     bool    `json:"favorite"`
	LastCookedAt *string `json:"last_cooked_at,omitempty"`
	CookCount    int     `json:"cook_count"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type foodListResponse struct {
	Items []foodResponse `json:"items"`
	Total int            `json:"total"`
}

type foodNutritionResponse struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
	Fiber   float64 `json:"fiber"`
	Sodium  float64 `json:"sodium"`
}

// ── Mapping ───────────────────────────────────────────────────────────

func requestToFood(req *foodRequest) *food.Food {
	f := &food.Food{
		Name: req.Name,
		Kind: food.Kind(req.Kind),
	}

	if req.Kind == string(food.KindLeaf) {
		if req.Source != "" {
			src := food.Source(req.Source)
			f.Source = &src
		}
		f.Barcode = req.Barcode
		f.OffID = req.OffID
		f.FdcID = req.FdcID
		f.Kcal100g = req.Kcal100g
		f.Protein100g = req.Protein100g
		f.Fat100g = req.Fat100g
		f.Carbs100g = req.Carbs100g
		f.Fiber100g = req.Fiber100g
		f.Sodium100g = req.Sodium100g
		f.SaturatedFat100g = req.SaturatedFat100g
		f.TransFat100g = req.TransFat100g
		f.Cholesterol100g = req.Cholesterol100g
		f.Sugar100g = req.Sugar100g
		f.Potassium100g = req.Potassium100g
		f.Calcium100g = req.Calcium100g
		f.Iron100g = req.Iron100g
		f.Magnesium100g = req.Magnesium100g
		f.Phosphorus100g = req.Phosphorus100g
		f.Zinc100g = req.Zinc100g
		f.VitaminA100g = req.VitaminA100g
		f.VitaminC100g = req.VitaminC100g
		f.VitaminD100g = req.VitaminD100g
		f.VitaminB12100g = req.VitaminB12100g
		f.VitaminB6100g = req.VitaminB6100g
		f.Folate100g = req.Folate100g
		return f
	}

	// Composed
	if req.Role != "" {
		r := food.Role(req.Role)
		f.Role = &r
	}
	f.ReferencePortions = req.ReferencePortions
	f.PrepMinutes = req.PrepMinutes
	f.CookMinutes = req.CookMinutes
	f.Notes = req.Notes

	f.Children = make([]food.FoodComponent, len(req.Children))
	for i, ch := range req.Children {
		f.Children[i] = food.FoodComponent{
			ChildID:   ch.ChildID,
			Amount:    ch.Amount,
			Unit:      ch.Unit,
			Grams:     ch.Grams,
			SortOrder: ch.SortOrder,
		}
	}
	f.Instructions = make([]food.Instruction, len(req.Instructions))
	for i, inst := range req.Instructions {
		f.Instructions[i] = food.Instruction{
			StepNumber: inst.StepNumber,
			Text:       inst.Text,
		}
	}
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	f.Tags = tags
	return f
}

func toFoodResponse(f *food.Food) foodResponse {
	resp := foodResponse{
		ID:        f.ID,
		Name:      f.Name,
		Kind:      string(f.Kind),
		ImagePath: f.ImagePath,
		Favorite:  f.Favorite,
		CookCount: f.CookCount,
		CreatedAt: f.CreatedAt.Format(time.RFC3339),
		UpdatedAt: f.UpdatedAt.Format(time.RFC3339),
	}
	if f.LastCookedAt != nil {
		s := f.LastCookedAt.Format(time.RFC3339)
		resp.LastCookedAt = &s
	}

	if f.Kind == food.KindLeaf {
		if f.Source != nil {
			s := string(*f.Source)
			resp.Source = &s
		}
		resp.Barcode = f.Barcode
		resp.OffID = f.OffID
		resp.FdcID = f.FdcID
		resp.Kcal100g = f.Kcal100g
		resp.Protein100g = f.Protein100g
		resp.Fat100g = f.Fat100g
		resp.Carbs100g = f.Carbs100g
		resp.Fiber100g = f.Fiber100g
		resp.Sodium100g = f.Sodium100g
		resp.SaturatedFat100g = f.SaturatedFat100g
		resp.TransFat100g = f.TransFat100g
		resp.Cholesterol100g = f.Cholesterol100g
		resp.Sugar100g = f.Sugar100g
		resp.Potassium100g = f.Potassium100g
		resp.Calcium100g = f.Calcium100g
		resp.Iron100g = f.Iron100g
		resp.Magnesium100g = f.Magnesium100g
		resp.Phosphorus100g = f.Phosphorus100g
		resp.Zinc100g = f.Zinc100g
		resp.VitaminA100g = f.VitaminA100g
		resp.VitaminC100g = f.VitaminC100g
		resp.VitaminD100g = f.VitaminD100g
		resp.VitaminB12100g = f.VitaminB12100g
		resp.VitaminB6100g = f.VitaminB6100g
		resp.Folate100g = f.Folate100g
		if len(f.Portions) > 0 {
			resp.Portions = make([]foodPortionResponse, len(f.Portions))
			for i, p := range f.Portions {
				resp.Portions[i] = foodPortionResponse{FoodID: p.FoodID, Unit: p.Unit, Grams: p.Grams}
			}
		}
		return resp
	}

	// Composed
	if f.Role != nil {
		r := string(*f.Role)
		resp.Role = &r
	}
	resp.VariantGroupID = f.VariantGroupID
	resp.ReferencePortions = f.ReferencePortions
	resp.PrepMinutes = f.PrepMinutes
	resp.CookMinutes = f.CookMinutes
	resp.Notes = f.Notes
	resp.Children = make([]foodChildResponse, len(f.Children))
	for i, ch := range f.Children {
		resp.Children[i] = foodChildResponse{
			ID: ch.ID, ParentID: ch.ParentID, ChildID: ch.ChildID,
			ChildName: ch.ChildName, ChildKind: string(ch.ChildKind),
			Amount: ch.Amount, Unit: ch.Unit, Grams: ch.Grams,
			GramsSource: ch.GramsSource, SortOrder: ch.SortOrder,
		}
	}
	resp.Instructions = make([]foodInstructionResponse, len(f.Instructions))
	for i, inst := range f.Instructions {
		resp.Instructions[i] = foodInstructionResponse{
			ID: inst.ID, FoodID: inst.FoodID,
			StepNumber: inst.StepNumber, Text: inst.Text,
		}
	}
	tags := f.Tags
	if tags == nil {
		tags = []string{}
	}
	resp.Tags = tags
	return resp
}

func foodHTTPError(err error) (int, string) { return toHTTPWithResource(err, "food") }

// ── Handlers ──────────────────────────────────────────────────────────

func (h *FoodHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req foodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	f := requestToFood(&req)
	if err := h.svc.Create(r.Context(), f); err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	// Re-load to hydrate child names + portions after create.
	loaded, err := h.svc.Get(r.Context(), f.ID)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toFoodResponse(loaded))
}

func (h *FoodHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	f, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toFoodResponse(f))
}

func (h *FoodHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req foodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	if req.Kind != "" && food.Kind(req.Kind) != existing.Kind {
		writeError(w, http.StatusUnprocessableEntity, "error.kind_immutable")
		return
	}
	f := requestToFood(&req)
	f.ID = id
	f.Kind = existing.Kind // kind is immutable after creation
	// image_path managed separately
	f.ImagePath = existing.ImagePath
	// cook tracking preserved
	f.Favorite = existing.Favorite
	f.CookCount = existing.CookCount
	f.LastCookedAt = existing.LastCookedAt
	// variant group sticky
	if f.Kind == food.KindComposed && f.VariantGroupID == nil {
		f.VariantGroupID = existing.VariantGroupID
	}
	if err := h.svc.Update(r.Context(), f); err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	loaded, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toFoodResponse(loaded))
}

func (h *FoodHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FoodHandler) List(w http.ResponseWriter, r *http.Request) {
	var q food.ListQuery
	if k := r.URL.Query().Get("kind"); k != "" {
		q.Kind = food.Kind(k)
		if q.Kind != food.KindLeaf && q.Kind != food.KindComposed {
			writeError(w, http.StatusBadRequest, "error.invalid_kind")
			return
		}
	}
	q.Search = r.URL.Query().Get("search")
	q.Role = r.URL.Query().Get("role")
	q.Tag = r.URL.Query().Get("tag")
	q.FavoriteOnly = r.URL.Query().Get("favorite") == "1"
	q.SortBy = r.URL.Query().Get("sort")
	q.SortDesc = r.URL.Query().Get("order") == "desc"
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
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	items := make([]foodResponse, len(result.Items))
	for i := range result.Items {
		items[i] = toFoodResponse(&result.Items[i])
	}
	writeJSON(w, http.StatusOK, foodListResponse{Items: items, Total: result.Total})
}

func (h *FoodHandler) Nutrition(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	m, err := h.resolver.PerPortion(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, foodNutritionResponse{
		Kcal: m.Kcal, Protein: m.Protein, Fat: m.Fat,
		Carbs: m.Carbs, Fiber: m.Fiber, Sodium: m.Sodium,
	})
}

type foodFavoriteRequest struct {
	Favorite bool `json:"favorite"`
}

func (h *FoodHandler) SetFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req foodFavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	f, err := h.svc.SetFavorite(r.Context(), id, req.Favorite)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toFoodResponse(f))
}

func (h *FoodHandler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	variant, err := h.svc.CreateVariant(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toFoodResponse(variant))
}

type foodVariantListResponse struct {
	Items []foodResponse `json:"items"`
}

func (h *FoodHandler) ListVariants(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	siblings, err := h.svc.ListVariants(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	items := make([]foodResponse, len(siblings))
	for i := range siblings {
		items[i] = toFoodResponse(&siblings[i])
	}
	writeJSON(w, http.StatusOK, foodVariantListResponse{Items: items})
}

// foodSummary is a slim shape for rotation-insight cards.
type foodSummary struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Role         *string `json:"role,omitempty"`
	ImagePath    *string `json:"image_path,omitempty"`
	CookCount    int     `json:"cook_count"`
	LastCookedAt *string `json:"last_cooked_at,omitempty"`
}

type foodInsightsResponse struct {
	Forgotten  []foodSummary `json:"forgotten"`
	MostCooked []foodSummary `json:"most_cooked"`
}

func toFoodSummary(f *food.Food) foodSummary {
	s := foodSummary{
		ID: f.ID, Name: f.Name, ImagePath: f.ImagePath, CookCount: f.CookCount,
	}
	if f.Role != nil {
		r := string(*f.Role)
		s.Role = &r
	}
	if f.LastCookedAt != nil {
		t := f.LastCookedAt.Format(time.RFC3339)
		s.LastCookedAt = &t
	}
	return s
}

// Insights handles GET /api/foods/insights.
func (h *FoodHandler) Insights(w http.ResponseWriter, r *http.Request) {
	q := food.InsightsQuery{}
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
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	forgotten := make([]foodSummary, len(out.Forgotten))
	for i := range out.Forgotten {
		forgotten[i] = toFoodSummary(&out.Forgotten[i])
	}
	mostCooked := make([]foodSummary, len(out.MostCooked))
	for i := range out.MostCooked {
		mostCooked[i] = toFoodSummary(&out.MostCooked[i])
	}
	writeJSON(w, http.StatusOK, foodInsightsResponse{Forgotten: forgotten, MostCooked: mostCooked})
}

// ── Image upload/delete ───────────────────────────────────────────────

func (h *FoodHandler) Upload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	f, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
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
	imgPath, err := h.store.SaveUpload(r.Context(), file, "foods", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	f.ImagePath = &imgPath
	if err := h.svc.Update(r.Context(), f); err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"image_path": imgPath})
}

func (h *FoodHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	f, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	if err := h.store.Delete("foods", id); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	f.ImagePath = nil
	if err := h.svc.Update(r.Context(), f); err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Portions (leaf foods only) ────────────────────────────────────────

type foodPortionRequest struct {
	Unit  string  `json:"unit"`
	Grams float64 `json:"grams"`
}

func (h *FoodHandler) ListPortions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	portions, err := h.svc.ListPortions(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	resp := make([]foodPortionResponse, len(portions))
	for i, p := range portions {
		resp[i] = foodPortionResponse{FoodID: p.FoodID, Unit: p.Unit, Grams: p.Grams}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *FoodHandler) UpsertPortion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req foodPortionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	p := &food.Portion{FoodID: id, Unit: req.Unit, Grams: req.Grams}
	if err := h.svc.UpsertPortion(r.Context(), p); err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, foodPortionResponse{FoodID: p.FoodID, Unit: p.Unit, Grams: p.Grams})
}

func (h *FoodHandler) DeletePortion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	unit := chi.URLParam(r, "unit")
	if unit == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err := h.svc.DeletePortion(r.Context(), id, unit); err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FoodHandler) SyncPortions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	count, err := h.svc.SyncPortionsFromFDC(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	portions, err := h.svc.ListPortions(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	resp := struct {
		Added    int                   `json:"added"`
		Portions []foodPortionResponse `json:"portions"`
	}{Added: count, Portions: make([]foodPortionResponse, len(portions))}
	for i, p := range portions {
		resp.Portions[i] = foodPortionResponse{FoodID: p.FoodID, Unit: p.Unit, Grams: p.Grams}
	}
	writeJSON(w, http.StatusOK, resp)
}
