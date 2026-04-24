package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// LookupHandler hosts external food lookup (OFF/FDC) and the "resolve into a
// new leaf food" + refetch flows.
type LookupHandler struct {
	resolver *food.Resolver
	imgStore *imagestore.Store
	svc      *food.Service
}

// NewLookupHandler constructs a LookupHandler.
func NewLookupHandler(resolver *food.Resolver, imgStore *imagestore.Store, svc *food.Service) *LookupHandler {
	return &LookupHandler{resolver: resolver, imgStore: imgStore, svc: svc}
}

type lookupResponse struct {
	Results          []food.Candidate  `json:"results"`
	RecommendedIndex int               `json:"recommended_index"`
	Trace            []food.TraceEntry `json:"trace,omitempty"`
}

// Lookup handles GET /api/foods/lookup.
func (h *LookupHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	barcode := r.URL.Query().Get("barcode")
	query := r.URL.Query().Get("query")
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	debug := r.URL.Query().Get("debug") == "true"

	if barcode == "" && query == "" {
		writeError(w, http.StatusBadRequest, "error.food.lookup.missing_param")
		return
	}

	ctx := r.Context()
	var trace *food.LookupTrace
	if debug {
		trace = food.NewLookupTrace()
		ctx = food.WithTrace(ctx, trace)
	}

	results, recommended, err := h.resolver.Lookup(ctx, barcode, query, lang, 5)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	resp := lookupResponse{Results: results, RecommendedIndex: recommended}
	if results == nil {
		resp.Results = []food.Candidate{}
	}
	if trace != nil {
		resp.Trace = trace.Entries()
	}
	writeJSON(w, http.StatusOK, resp)
}

type resolveRequest struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Barcode     *string  `json:"barcode"`
	FdcID       *string  `json:"fdc_id"`
	ImageURL    *string  `json:"image_url"`
	Kcal100g    *float64 `json:"kcal_100g"`
	Protein100g *float64 `json:"protein_100g"`
	Fat100g     *float64 `json:"fat_100g"`
	Carbs100g   *float64 `json:"carbs_100g"`
	Fiber100g   *float64 `json:"fiber_100g"`
	Sodium100g  *float64 `json:"sodium_100g"`

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

	// Grams per serving (from OFF) — seeds a "serving" portion.
	ServingQuantityG *float64 `json:"serving_quantity_g"`
}

// Resolve handles POST /api/foods/resolve — creates a new leaf food from a
// candidate and best-effort syncs portions + image.
func (h *LookupHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	src := food.Source(req.Source)
	if src == "" {
		src = food.SourceManual
	}
	f := &food.Food{
		Name:             req.Name,
		Kind:             food.KindLeaf,
		Source:           &src,
		Barcode:          req.Barcode,
		FdcID:            req.FdcID,
		Kcal100g:         req.Kcal100g,
		Protein100g:      req.Protein100g,
		Fat100g:          req.Fat100g,
		Carbs100g:        req.Carbs100g,
		Fiber100g:        req.Fiber100g,
		Sodium100g:       req.Sodium100g,
		SaturatedFat100g: req.SaturatedFat100g,
		TransFat100g:     req.TransFat100g,
		Cholesterol100g:  req.Cholesterol100g,
		Sugar100g:        req.Sugar100g,
		Potassium100g:    req.Potassium100g,
		Calcium100g:      req.Calcium100g,
		Iron100g:         req.Iron100g,
		Magnesium100g:    req.Magnesium100g,
		Phosphorus100g:   req.Phosphorus100g,
		Zinc100g:         req.Zinc100g,
		VitaminA100g:     req.VitaminA100g,
		VitaminC100g:     req.VitaminC100g,
		VitaminD100g:     req.VitaminD100g,
		VitaminB12100g:   req.VitaminB12100g,
		VitaminB6100g:    req.VitaminB6100g,
		Folate100g:       req.Folate100g,
	}

	if err := h.svc.Create(r.Context(), f); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	// Best-effort FDC portion sync so the user gets honey tbsp=21g etc.
	if f.FdcID != nil && *f.FdcID != "" {
		_, _ = h.svc.SyncPortionsFromFDC(r.Context(), f.ID)
	}

	// OFF-sourced products carry a per-serving gram weight.
	if req.ServingQuantityG != nil && *req.ServingQuantityG > 0 {
		_ = h.svc.UpsertPortion(r.Context(), &food.Portion{
			FoodID: f.ID,
			Unit:   "serving",
			Grams:  *req.ServingQuantityG,
		})
	}

	// Download image if URL provided + image store available.
	if req.ImageURL != nil && *req.ImageURL != "" && h.imgStore != nil {
		imgPath, err := h.imgStore.SaveFromURL(r.Context(), *req.ImageURL, "foods", f.ID)
		if err == nil {
			f.ImagePath = &imgPath
			_ = h.svc.Update(r.Context(), f)
		}
	}

	// Re-load to hydrate portions in response.
	loaded, err := h.svc.Get(r.Context(), f.ID)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toFoodResponse(loaded))
}

// Refetch handles POST /api/foods/{id}/refetch. Re-queries the upstream
// source using the food's stored IDs and overwrites the nutrient fields.
func (h *LookupHandler) Refetch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	if existing.Kind != food.KindLeaf {
		writeError(w, http.StatusBadRequest, "error.food.refetch.not_leaf")
		return
	}

	var barcode, query, lang string
	lang = r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	switch {
	case existing.Barcode != nil && *existing.Barcode != "":
		barcode = *existing.Barcode
	case existing.FdcID != nil && *existing.FdcID != "":
		query = *existing.FdcID
	default:
		writeError(w, http.StatusBadRequest, "error.food.refetch.no_source")
		return
	}

	results, _, err := h.resolver.Lookup(r.Context(), barcode, query, lang, 1)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	if len(results) == 0 {
		writeError(w, http.StatusNotFound, "error.food.refetch.no_results")
		return
	}

	applyCandidateNutrients(existing, &results[0])
	if err := h.svc.Update(r.Context(), existing); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toFoodResponse(existing))
}

// applyCandidateNutrients overwrites nutrient fields on the stored food with
// values from a freshly-fetched candidate. Never touches Name, ImagePath, or
// source IDs.
func applyCandidateNutrients(f *food.Food, c *food.Candidate) {
	f.Kcal100g = c.Kcal100g
	f.Protein100g = c.Protein100g
	f.Fat100g = c.Fat100g
	f.Carbs100g = c.Carbs100g
	f.Fiber100g = c.Fiber100g
	f.Sodium100g = c.Sodium100g
	f.SaturatedFat100g = c.SaturatedFat100g
	f.TransFat100g = c.TransFat100g
	f.Cholesterol100g = c.Cholesterol100g
	f.Sugar100g = c.Sugar100g
	f.Potassium100g = c.Potassium100g
	f.Calcium100g = c.Calcium100g
	f.Iron100g = c.Iron100g
	f.Magnesium100g = c.Magnesium100g
	f.Phosphorus100g = c.Phosphorus100g
	f.Zinc100g = c.Zinc100g
	f.VitaminA100g = c.VitaminA100g
	f.VitaminC100g = c.VitaminC100g
	f.VitaminD100g = c.VitaminD100g
	f.VitaminB12100g = c.VitaminB12100g
	f.VitaminB6100g = c.VitaminB6100g
	f.Folate100g = c.Folate100g
}
