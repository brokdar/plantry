package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// FoodHandler hosts the HTTP surface for the unified food resource.
type FoodHandler struct {
	svc      *food.Service
	resolver *food.NutritionResolver
	store    *imagestore.Store
	now      func() time.Time
}

const maxImageSize = 10 << 20 // 10 MB

// NewFoodHandler wires services into the handler.
func NewFoodHandler(svc *food.Service, resolver *food.NutritionResolver, store *imagestore.Store) *FoodHandler {
	return &FoodHandler{svc: svc, resolver: resolver, store: store, now: time.Now}
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
	ToTaste   bool    `json:"to_taste"`
}

type foodInstructionRequest struct {
	StepNumber int    `json:"step_number"`
	Text       string `json:"text"`
}

type foodRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`

	// Shared. On update a nil SourceURL/Description keeps the stored value.
	SourceURL   *string `json:"source_url,omitempty"`
	Description *string `json:"description,omitempty"`
	// ImageURL is downloaded into the image store after create; ignored on update.
	ImageURL *string `json:"image_url,omitempty"`

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
	KeepDays          *int                     `json:"keep_days,omitempty"`
	Freezable         *bool                    `json:"freezable,omitempty"`
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
	ToTaste     bool    `json:"to_taste"`
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
	KeepDays          *int                      `json:"keep_days,omitempty"`
	Freezable         *bool                     `json:"freezable,omitempty"`
	Children          []foodChildResponse       `json:"children,omitempty"`
	Instructions      []foodInstructionResponse `json:"instructions,omitempty"`
	Tags              []string                  `json:"tags,omitempty"`

	// Shared
	SourceURL   *string `json:"source_url,omitempty"`
	Description *string `json:"description,omitempty"`
	ImagePath   *string `json:"image_path,omitempty"`
	Favorite    bool    `json:"favorite"`
	Archived    bool    `json:"archived"`
	// Trial: an assistant-added recipe on trial (NEW-1); ToTry shows its
	// To try badge until the recipe is opened (NEW-3).
	Trial     bool   `json:"trial"`
	ToTry     bool   `json:"to_try"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// DietClass is the effective diet class (D28) and DietClassSource where
	// it comes from: auto (leaf name), user (override) or derived (recipe).
	// Set by food.Service.Derive.
	DietClass       string `json:"diet_class"`
	DietClassSource string `json:"diet_class_source"`

	// NoCook is the effective "needs no cooking" flag and NoCookSource where
	// it comes from: user (override) or derived (planning.NoCook).
	NoCook       bool   `json:"no_cook"`
	NoCookSource string `json:"no_cook_source"`
	// ServingG is a leaf's stored serving size in grams (null: none).
	ServingG *float64 `json:"serving_g"`

	// Derived (§3.3), one of them per kind: leafDerived is set by
	// toFoodResponse, composedDerived by FoodHandler.respond. Call
	// food.Service.Derive first.
	*composedDerived
	*leafDerived
}

// composedDerived is a recipe's eating history (D16) and per-portion summary.
type composedDerived struct {
	LastEaten   *string            `json:"last_eaten"`
	TimesCooked int                `json:"times_cooked"`
	PerPortion  perPortionResponse `json:"per_portion"`
}

type perPortionResponse struct {
	Kcal     float64 `json:"kcal"`
	Protein  float64 `json:"protein"`
	Complete bool    `json:"complete"`
}

// leafDerived is a leaf's recipe usage and effective store section (D18):
// the user's override (source "user") or the name classification ("auto").
type leafDerived struct {
	UsedInCount       int    `json:"used_in_count"`
	ShopSection       string `json:"shop_section"`
	ShopSectionSource string `json:"shop_section_source"`
	// RoleSource says where the leaf's role (effective, in role) comes from:
	// user (override), derived (store section) or null (no role).
	RoleSource *string `json:"role_source"`
}

// Store section sources.
const (
	shopSectionAuto = "auto"
	shopSectionUser = "user"
)

// foodCreateResponse is a food plus non-fatal problems hit while creating it
// (e.g. "image_download_failed").
type foodCreateResponse struct {
	foodResponse
	Warnings []string `json:"warnings,omitempty"`
}

// Create warnings.
const warnImageDownloadFailed = "image_download_failed"

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
	// Complete is false when a leaf lacks kcal/protein/fat/carbs; Missing
	// lists those leaves.
	Complete bool                  `json:"complete"`
	Missing  []missingLeafResponse `json:"missing"`
}

type missingLeafResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ── Mapping ───────────────────────────────────────────────────────────

func requestToFood(req *foodRequest) *food.Food {
	f := &food.Food{
		Name:        req.Name,
		Kind:        food.Kind(req.Kind),
		SourceURL:   req.SourceURL,
		Description: req.Description,
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
		if req.Role != "" {
			r := food.Role(req.Role)
			f.Role = &r
		}
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
	f.KeepDays = req.KeepDays
	f.Freezable = req.Freezable

	f.Children = make([]food.FoodComponent, len(req.Children))
	for i, ch := range req.Children {
		f.Children[i] = food.FoodComponent{
			ChildID:   ch.ChildID,
			Amount:    ch.Amount,
			Unit:      ch.Unit,
			Grams:     ch.Grams,
			SortOrder: ch.SortOrder,
			ToTaste:   ch.ToTaste,
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
		ID:          f.ID,
		Name:        f.Name,
		Kind:        string(f.Kind),
		SourceURL:   f.SourceURL,
		Description: f.Description,
		ImagePath:   f.ImagePath,
		Favorite:    f.Favorite,
		Archived:    f.Archived,
		Trial:       f.OnTrial(),
		ToTry:       f.ToTry(),
		CreatedAt:   f.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   f.UpdatedAt.Format(time.RFC3339),

		DietClass:       string(f.DietClass),
		DietClassSource: string(f.DietClassSource),
		NoCook:          planning.NoCook(f),
		NoCookSource:    planning.NoCookSource(f),
		ServingG:        f.ServingG,
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
		resp.leafDerived = &leafDerived{
			UsedInCount:       f.UsedInCount,
			ShopSection:       string(shopping.SectionOf(f)),
			ShopSectionSource: shopSectionAuto,
		}
		if f.ShopSection != nil {
			resp.ShopSectionSource = shopSectionUser
		}
		if role, src := shopping.EffectiveRole(f); role != nil {
			r := string(*role)
			resp.Role = &r
			resp.RoleSource = &src
		}
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
	resp.KeepDays = f.KeepDays
	resp.Freezable = f.Freezable
	resp.Children = make([]foodChildResponse, len(f.Children))
	for i, ch := range f.Children {
		resp.Children[i] = foodChildResponse{
			ID: ch.ID, ParentID: ch.ParentID, ChildID: ch.ChildID,
			ChildName: ch.ChildName, ChildKind: string(ch.ChildKind),
			Amount: ch.Amount, Unit: ch.Unit, Grams: ch.Grams,
			GramsSource: ch.GramsSource, SortOrder: ch.SortOrder, ToTaste: ch.ToTaste,
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

// respond maps foods to responses with their derived fields as of today:
// history and per-portion summary for recipes, recipe usage for leaves.
func (h *FoodHandler) respond(ctx context.Context, today time.Time, foods ...*food.Food) ([]foodResponse, error) {
	if err := h.svc.Derive(ctx, today, foods...); err != nil {
		return nil, err
	}
	var recipes []int64
	for _, f := range foods {
		if f.Kind == food.KindComposed {
			recipes = append(recipes, f.ID)
		}
	}
	perPortion, err := h.resolver.PerPortionDetailedMany(ctx, recipes)
	if err != nil {
		return nil, err
	}
	out := make([]foodResponse, len(foods))
	for i, f := range foods {
		out[i] = toFoodResponse(f)
		if f.Kind != food.KindComposed {
			continue
		}
		d := perPortion[f.ID]
		cd := &composedDerived{
			TimesCooked: f.History.TimesCooked,
			PerPortion:  perPortionResponse{Kcal: d.Macros.Kcal, Protein: d.Macros.Protein, Complete: d.Complete},
		}
		if f.History.LastEaten != nil {
			s := dateString(*f.History.LastEaten)
			cd.LastEaten = &s
		}
		out[i].composedDerived = cd
	}
	return out, nil
}

// writeFood writes one food with its derived fields as of today (D25).
// Handlers parse today before any write, so a bad value never follows a
// saved change.
func (h *FoodHandler) writeFood(w http.ResponseWriter, r *http.Request, status int, today time.Time, f *food.Food) {
	resp, err := h.respond(r.Context(), today, f)
	if err != nil {
		code, key := foodHTTPError(err)
		writeError(w, code, key)
		return
	}
	writeJSON(w, status, resp[0])
}

// ── Handlers ──────────────────────────────────────────────────────────

func (h *FoodHandler) Create(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
	var req foodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	f := requestToFood(&req)
	if err := h.svc.Create(r.Context(), f); err != nil {
		status, key := foodHTTPError(err)
		if errors.Is(err, domain.ErrDuplicateName) && f.Kind == food.KindLeaf {
			if existingID, lookupErr := h.svc.LeafIDByName(r.Context(), f.Name); lookupErr == nil {
				writeJSON(w, status, map[string]any{
					"message_key": key,
					"status":      status,
					"existing_id": existingID,
				})
				return
			}
		}
		writeError(w, status, key)
		return
	}
	var warnings []string
	if req.ImageURL != nil && *req.ImageURL != "" {
		if !h.saveImageFromURL(r, f, *req.ImageURL) {
			warnings = append(warnings, warnImageDownloadFailed)
		}
	}
	// Re-load to hydrate child names + portions after create.
	loaded, err := h.svc.Get(r.Context(), f.ID)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	resp, err := h.respond(r.Context(), today, loaded)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, foodCreateResponse{foodResponse: resp[0], Warnings: warnings})
}

// saveImageFromURL downloads imageURL into the image store and records the
// path on f. Reports false when no store is configured or any step fails;
// the food itself stays created either way.
func (h *FoodHandler) saveImageFromURL(r *http.Request, f *food.Food, imageURL string) bool {
	if h.store == nil {
		return false
	}
	imgPath, err := h.store.SaveFromURL(r.Context(), imageURL, "foods", f.ID)
	if err != nil {
		slog.WarnContext(r.Context(), "food image download failed", "food_id", f.ID, "error", err)
		return false
	}
	f.ImagePath = &imgPath
	if err := h.svc.Update(r.Context(), f); err != nil {
		slog.WarnContext(r.Context(), "food image path update failed", "food_id", f.ID, "error", err)
		// The row doesn't reference the file, so don't leave it orphaned.
		_ = h.store.Delete("foods", f.ID)
		f.ImagePath = nil
		return false
	}
	return true
}

func (h *FoodHandler) Get(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
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
	h.writeFood(w, r, http.StatusOK, today, f)
}

func (h *FoodHandler) Update(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
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
	// Editors unaware of these fields must not wipe them.
	if f.SourceURL == nil {
		f.SourceURL = existing.SourceURL
	}
	if f.Description == nil {
		f.Description = existing.Description
	}
	if f.Role == nil {
		f.Role = existing.Role
	}
	f.Favorite = existing.Favorite
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
	h.writeFood(w, r, http.StatusOK, today, loaded)
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
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
	q.Today = today
	for param, dst := range map[string]*int{
		"max_minutes":         &q.MaxMinutes,
		"not_eaten_weeks":     &q.NotEatenWeeks,
		"created_within_days": &q.CreatedWithinDays,
	} {
		v := r.URL.Query().Get(param)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_query")
			return
		}
		*dst = n
	}
	if k := r.URL.Query().Get("kind"); k != "" {
		q.Kind = food.Kind(k)
		if q.Kind != food.KindLeaf && q.Kind != food.KindComposed {
			writeError(w, http.StatusBadRequest, "error.invalid_kind")
			return
		}
	}
	q.Diet = diet.Class(r.URL.Query().Get("diet"))
	q.Search = r.URL.Query().Get("search")
	q.Role = r.URL.Query().Get("role")
	q.Tag = r.URL.Query().Get("tag")
	q.FavoriteOnly = r.URL.Query().Get("favorite") == "1"
	if r.URL.Query().Get("archived") == "1" {
		q.Archive = food.ArchiveOnly
	}
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
	foods := make([]*food.Food, len(result.Items))
	for i := range result.Items {
		foods[i] = &result.Items[i]
	}
	items, err := h.respond(r.Context(), today, foods...)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, foodListResponse{Items: items, Total: result.Total})
}

func (h *FoodHandler) Nutrition(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	d, err := h.resolver.PerPortionDetailed(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	missing := make([]missingLeafResponse, len(d.Missing))
	for i, l := range d.Missing {
		missing[i] = missingLeafResponse{ID: l.ID, Name: l.Name}
	}
	m := d.Macros
	writeJSON(w, http.StatusOK, foodNutritionResponse{
		Kcal: m.Kcal, Protein: m.Protein, Fat: m.Fat,
		Carbs: m.Carbs, Fiber: m.Fiber, Sodium: m.Sodium,
		Complete: d.Complete, Missing: missing,
	})
}

// foodMacrosEntry is one item in the batch macros response. Macros are
// per-portion for composed foods and per-100 g for leaf foods, mirroring
// `NutritionResolver.PerPortion`.
type foodMacrosEntry struct {
	FoodID int64          `json:"food_id"`
	Macros macrosResponse `json:"macros"`
}

type foodMacrosResponse struct {
	Foods []foodMacrosEntry `json:"foods"`
}

// BatchMacros handles GET /api/foods/macros?ids=1,2,3.
//
// Unknown ids are silently omitted from the response so a single bad id
// doesn't sink the whole batch — callers (e.g. the planner tray) usually
// want a best-effort lookup.
func (h *FoodHandler) BatchMacros(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("ids"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_query")
		return
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_query")
			return
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_query")
		return
	}

	out := make([]foodMacrosEntry, 0, len(ids))
	for _, id := range ids {
		m, err := h.resolver.PerPortion(r.Context(), id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				continue
			}
			status, key := foodHTTPError(err)
			writeError(w, status, key)
			return
		}
		out = append(out, foodMacrosEntry{FoodID: id, Macros: toMacrosResponse(m)})
	}
	writeJSON(w, http.StatusOK, foodMacrosResponse{Foods: out})
}

type foodFavoriteRequest struct {
	Favorite bool `json:"favorite"`
}

func (h *FoodHandler) SetFavorite(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
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
	h.writeFood(w, r, http.StatusOK, today, f)
}

// Archive handles POST /api/foods/{id}/archive: the recipe leaves planning
// and the lists (LIB-4).
func (h *FoodHandler) Archive(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, h.svc.Archive)
}

// Unarchive handles POST /api/foods/{id}/restore.
func (h *FoodHandler) Unarchive(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, h.svc.Unarchive)
}

func (h *FoodHandler) setArchived(w http.ResponseWriter, r *http.Request, do func(context.Context, int64) (*food.Food, foodevent.Undo, error)) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	f, _, err := do(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	h.writeFood(w, r, http.StatusOK, today, f)
}

type foodShopSectionRequest struct {
	Section *food.ShopSection `json:"section"`
}

// SetShopSection handles PUT /api/foods/{id}/shop-section {section | null}:
// a leaf's store section override; null reverts to the classification.
func (h *FoodHandler) SetShopSection(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req foodShopSectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	f, err := h.svc.SetShopSection(r.Context(), id, req.Section)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	h.writeFood(w, r, http.StatusOK, today, f)
}

type foodDietClassRequest struct {
	Class *diet.Class `json:"class"`
}

// SetDietClass handles PUT /api/foods/{id}/diet-class {class | null}: a
// food's diet class override; null reverts to the derived class.
func (h *FoodHandler) SetDietClass(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req foodDietClassRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	f, err := h.svc.SetDietClass(r.Context(), id, req.Class)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	h.writeFood(w, r, http.StatusOK, today, f)
}

type foodNoCookRequest struct {
	Value    *bool    `json:"value"`
	ServingG *float64 `json:"serving_g"`
}

// foodNoCookBody is the request body plus whether serving_g was present, so
// an absent field and an explicit null can be told apart.
type foodNoCookBody struct {
	foodNoCookRequest
	servingGSet bool
}

func (b *foodNoCookBody) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &b.foodNoCookRequest); err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	_, b.servingGSet = keys["serving_g"]
	return nil
}

// SetNoCook handles PUT /api/foods/{id}/no-cook {value: true|false|null}: a
// food's "needs no cooking" override; null reverts to the derived answer.
// serving_g sets the serving size; an explicit null clears it and an absent
// field keeps the stored one.
func (h *FoodHandler) SetNoCook(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req foodNoCookBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	serving := req.ServingG
	if !req.servingGSet {
		cur, err := h.svc.Get(r.Context(), id)
		if err != nil {
			status, key := foodHTTPError(err)
			writeError(w, status, key)
			return
		}
		serving = cur.ServingG
	}
	f, err := h.svc.SetNoCook(r.Context(), id, req.Value, serving)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	h.writeFood(w, r, http.StatusOK, today, f)
}

func (h *FoodHandler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
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
	h.writeFood(w, r, http.StatusCreated, today, variant)
}

type foodVariantListResponse struct {
	Items []foodResponse `json:"items"`
}

func (h *FoodHandler) ListVariants(w http.ResponseWriter, r *http.Request) {
	today, ok := parseToday(w, r.URL.Query().Get("today"), h.now)
	if !ok {
		return
	}
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
	foods := make([]*food.Food, len(siblings))
	for i := range siblings {
		foods[i] = &siblings[i]
	}
	items, err := h.respond(r.Context(), today, foods...)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, foodVariantListResponse{Items: items})
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

// ── Scaled ingredient list (composed foods) ───────────────────────────

type scaledIngredientResponse struct {
	ChildID   int64           `json:"child_id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Amount    float64         `json:"amount"`
	Unit      string          `json:"unit"`
	Grams     float64         `json:"grams"`
	Display   units.Quantity  `json:"display"`
	Buy       *units.Quantity `json:"buy"`
	DietClass string          `json:"diet_class"`
	ToTaste   bool            `json:"to_taste"`
}

type scaledResponse struct {
	FoodID            int64                      `json:"food_id"`
	Portions          float64                    `json:"portions"`
	ReferencePortions float64                    `json:"reference_portions"`
	Factor            float64                    `json:"factor"`
	Ingredients       []scaledIngredientResponse `json:"ingredients"`
}

// Scaled handles GET /api/foods/{id}/scaled?portions=N.
func (h *FoodHandler) Scaled(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	portions, err := strconv.ParseFloat(r.URL.Query().Get("portions"), 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.food.invalid_portions")
		return
	}
	sc, err := h.svc.Scaled(r.Context(), id, portions)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	resp := scaledResponse{
		FoodID:            sc.FoodID,
		Portions:          sc.Portions,
		ReferencePortions: sc.ReferencePortions,
		Factor:            sc.Factor,
		Ingredients:       make([]scaledIngredientResponse, len(sc.Ingredients)),
	}
	for i, in := range sc.Ingredients {
		resp.Ingredients[i] = scaledIngredientResponse{
			ChildID: in.ChildID, Name: in.Name, Kind: string(in.Kind),
			Amount: in.Amount, Unit: in.Unit, Grams: in.Grams,
			Display: in.Display, Buy: in.Buy, DietClass: string(in.DietClass), ToTaste: in.ToTaste,
		}
	}
	writeJSON(w, http.StatusOK, resp)
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

// WithClock makes "today" read the given clock (the dev clock).
func (h *FoodHandler) WithClock(now func() time.Time) *FoodHandler { h.now = now; return h }
