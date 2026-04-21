package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

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
	Results          []ingredient.Candidate  `json:"results"`
	RecommendedIndex int                     `json:"recommended_index"`
	Trace            []ingredient.TraceEntry `json:"trace,omitempty"`
}

// Lookup handles GET /api/ingredients/lookup.
func (h *LookupHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	barcode := r.URL.Query().Get("barcode")
	query := r.URL.Query().Get("query")
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	debug := r.URL.Query().Get("debug") == "true"

	if barcode == "" && query == "" {
		writeError(w, http.StatusBadRequest, "error.ingredient.lookup.missing_param")
		return
	}

	ctx := r.Context()
	var trace *ingredient.LookupTrace
	if debug {
		trace = ingredient.NewLookupTrace()
		ctx = ingredient.WithTrace(ctx, trace)
	}

	results, recommended, err := h.resolver.Lookup(ctx, barcode, query, lang, 5)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	resp := lookupResponse{
		Results:          results,
		RecommendedIndex: recommended,
	}
	if results == nil {
		resp.Results = []ingredient.Candidate{}
	}
	if trace != nil {
		resp.Trace = trace.Entries()
	}
	writeJSON(w, http.StatusOK, resp)
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

	// ServingQuantityG is grams per serving, when supplied by the upstream
	// source (OFF). When present, a "serving" portion is seeded on create.
	ServingQuantityG *float64 `json:"serving_quantity_g"`
}

// Resolve handles POST /api/ingredients/resolve.
func (h *LookupHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	i := &ingredient.Ingredient{
		Name:             req.Name,
		Source:           req.Source,
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

	if err := h.svc.Create(r.Context(), i); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	// Best-effort FDC portion sync so the user gets honey tbsp=21g (etc.)
	// without a second click. Failures are swallowed — the user can still
	// trigger the sync manually from the ingredient detail view.
	if i.FdcID != nil && *i.FdcID != "" {
		_, _ = h.svc.SyncPortionsFromFDC(r.Context(), i.ID)
	}

	// OFF-sourced products carry a per-serving gram weight. Seed it as a
	// "serving" portion so the user can pick "1 serving" as a natural unit.
	if req.ServingQuantityG != nil && *req.ServingQuantityG > 0 {
		_ = h.svc.UpsertPortion(r.Context(), &ingredient.Portion{
			IngredientID: i.ID,
			Unit:         "serving",
			Grams:        *req.ServingQuantityG,
		})
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

// Refetch handles POST /api/ingredients/{id}/refetch. It re-queries the
// upstream source using the ingredient's stored IDs and overwrites the
// nutrient fields (all 22) with the fresh values. Name, image, portions, and
// source IDs are preserved.
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

	// Choose an upstream query based on what was stored when the ingredient
	// was first resolved. Barcode always wins when present (OFF or branded
	// FDC both support it). Fall back to FDC ID as the search term.
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
		writeError(w, http.StatusBadRequest, "error.ingredient.refetch.no_source")
		return
	}

	results, _, err := h.resolver.Lookup(r.Context(), barcode, query, lang, 1)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	if len(results) == 0 {
		writeError(w, http.StatusNotFound, "error.ingredient.refetch.no_results")
		return
	}

	applyCandidateNutrients(existing, &results[0])
	if err := h.svc.Update(r.Context(), existing); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(existing))
}

// applyCandidateNutrients overwrites nutrient fields on the stored ingredient
// with values from a freshly-fetched candidate. It never touches Name,
// ImagePath, or source IDs.
func applyCandidateNutrients(i *ingredient.Ingredient, c *ingredient.Candidate) {
	if c.Kcal100g != nil {
		i.Kcal100g = *c.Kcal100g
	}
	if c.Protein100g != nil {
		i.Protein100g = *c.Protein100g
	}
	if c.Fat100g != nil {
		i.Fat100g = *c.Fat100g
	}
	if c.Carbs100g != nil {
		i.Carbs100g = *c.Carbs100g
	}
	if c.Fiber100g != nil {
		i.Fiber100g = *c.Fiber100g
	}
	if c.Sodium100g != nil {
		i.Sodium100g = *c.Sodium100g
	}
	i.SaturatedFat100g = c.SaturatedFat100g
	i.TransFat100g = c.TransFat100g
	i.Cholesterol100g = c.Cholesterol100g
	i.Sugar100g = c.Sugar100g
	i.Potassium100g = c.Potassium100g
	i.Calcium100g = c.Calcium100g
	i.Iron100g = c.Iron100g
	i.Magnesium100g = c.Magnesium100g
	i.Phosphorus100g = c.Phosphorus100g
	i.Zinc100g = c.Zinc100g
	i.VitaminA100g = c.VitaminA100g
	i.VitaminC100g = c.VitaminC100g
	i.VitaminD100g = c.VitaminD100g
	i.VitaminB12100g = c.VitaminB12100g
	i.VitaminB6100g = c.VitaminB6100g
	i.Folate100g = c.Folate100g
}
