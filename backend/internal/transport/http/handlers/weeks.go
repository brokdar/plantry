package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

// WeekHandler exposes weekly planner read endpoints.
type WeekHandler struct {
	planner    *planner.Service
	plates     *plate.Service
	components *component.Service
	ingRepo    ingredient.Repository
	feedback   feedback.Repository
}

// NewWeekHandler creates a WeekHandler. The feedback repository is optional;
// pass nil to serve responses without embedded per-plate feedback.
func NewWeekHandler(p *planner.Service, plates *plate.Service, components *component.Service, ingRepo ingredient.Repository, fb feedback.Repository) *WeekHandler {
	return &WeekHandler{planner: p, plates: plates, components: components, ingRepo: ingRepo, feedback: fb}
}

type plateComponentResponse struct {
	ID          int64   `json:"id"`
	PlateID     int64   `json:"plate_id"`
	ComponentID int64   `json:"component_id"`
	Portions    float64 `json:"portions"`
	SortOrder   int     `json:"sort_order"`
}

type plateResponse struct {
	ID         int64                    `json:"id"`
	WeekID     int64                    `json:"week_id"`
	Day        int                      `json:"day"`
	SlotID     int64                    `json:"slot_id"`
	Note       *string                  `json:"note,omitempty"`
	Skipped    bool                     `json:"skipped"`
	Components []plateComponentResponse `json:"components"`
	Feedback   *feedbackResponse        `json:"feedback,omitempty"`
	CreatedAt  string                   `json:"created_at"`
}

type weekResponse struct {
	ID         int64           `json:"id"`
	Year       int             `json:"year"`
	WeekNumber int             `json:"week_number"`
	Plates     []plateResponse `json:"plates"`
	CreatedAt  string          `json:"created_at"`
}

type weekListResponse struct {
	Items []weekResponse `json:"items"`
	Total int64          `json:"total"`
}

func toPlateComponentResponse(pc *plate.PlateComponent) plateComponentResponse {
	return plateComponentResponse{
		ID: pc.ID, PlateID: pc.PlateID, ComponentID: pc.ComponentID,
		Portions: pc.Portions, SortOrder: pc.SortOrder,
	}
}

func toPlateResponse(p *plate.Plate, fb *feedback.PlateFeedback) plateResponse {
	comps := make([]plateComponentResponse, len(p.Components))
	for i := range p.Components {
		comps[i] = toPlateComponentResponse(&p.Components[i])
	}
	resp := plateResponse{
		ID: p.ID, WeekID: p.WeekID, Day: p.Day, SlotID: p.SlotID,
		Note: p.Note, Skipped: p.Skipped, Components: comps,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
	}
	if fb != nil {
		r := toFeedbackResponse(fb)
		resp.Feedback = &r
	}
	return resp
}

func toWeekResponse(w *planner.Week, feedbackByPlate map[int64]*feedback.PlateFeedback) weekResponse {
	plates := make([]plateResponse, len(w.Plates))
	for i := range w.Plates {
		plates[i] = toPlateResponse(&w.Plates[i], feedbackByPlate[w.Plates[i].ID])
	}
	return weekResponse{
		ID: w.ID, Year: w.Year, WeekNumber: w.WeekNumber,
		Plates: plates, CreatedAt: w.CreatedAt.Format(time.RFC3339),
	}
}

// loadFeedbackByWeek returns a plate_id → feedback map for the week. If no
// feedback repository is configured it returns an empty map so the caller
// doesn't have to branch.
func (h *WeekHandler) loadFeedbackByWeek(ctx context.Context, weekID int64) map[int64]*feedback.PlateFeedback {
	out := map[int64]*feedback.PlateFeedback{}
	if h.feedback == nil {
		return out
	}
	rows, err := h.feedback.ListByWeek(ctx, weekID)
	if err != nil {
		// Feedback is non-critical metadata — failures shouldn't 500 the week.
		return out
	}
	for i := range rows {
		row := rows[i]
		out[row.PlateID] = &row
	}
	return out
}

func weekError(err error) (int, string) {
	return toHTTPWithResource(err, "week")
}

// Current handles GET /api/weeks/current.
func (h *WeekHandler) Current(w http.ResponseWriter, r *http.Request) {
	week, err := h.planner.Current(r.Context(), time.Now().UTC())
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toWeekResponse(week, h.loadFeedbackByWeek(r.Context(), week.ID)))
}

// ByDate handles GET /api/weeks/by-date?year=&week=.
func (h *WeekHandler) ByDate(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	weekNum, err := strconv.Atoi(r.URL.Query().Get("week"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	week, err := h.planner.ByDate(r.Context(), year, weekNum)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toWeekResponse(week, h.loadFeedbackByWeek(r.Context(), week.ID)))
}

// Get handles GET /api/weeks/{id}.
func (h *WeekHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	week, err := h.planner.Get(r.Context(), id)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toWeekResponse(week, h.loadFeedbackByWeek(r.Context(), week.ID)))
}

// List handles GET /api/weeks.
func (h *WeekHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := 25
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	rows, total, err := h.planner.List(r.Context(), limit, offset)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	items := make([]weekResponse, len(rows))
	for i := range rows {
		items[i] = toWeekResponse(&rows[i], h.loadFeedbackByWeek(r.Context(), rows[i].ID))
	}
	writeJSON(w, http.StatusOK, weekListResponse{Items: items, Total: total})
}

type copyWeekRequest struct {
	TargetYear int `json:"target_year"`
	TargetWeek int `json:"target_week"`
}

// Copy handles POST /api/weeks/{id}/copy.
func (h *WeekHandler) Copy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req copyWeekRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	target, err := h.planner.Copy(r.Context(), id, req.TargetYear, req.TargetWeek)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toWeekResponse(target, h.loadFeedbackByWeek(r.Context(), target.ID)))
}

type createPlateRequest struct {
	Day        int                          `json:"day"`
	SlotID     int64                        `json:"slot_id"`
	Note       *string                      `json:"note"`
	Skipped    bool                         `json:"skipped"`
	Components []createPlateComponentInline `json:"components"`
}

type createPlateComponentInline struct {
	ComponentID int64   `json:"component_id"`
	Portions    float64 `json:"portions"`
}

// CreatePlate handles POST /api/weeks/{id}/plates.
func (h *WeekHandler) CreatePlate(w http.ResponseWriter, r *http.Request) {
	weekID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req createPlateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	p := &plate.Plate{
		WeekID: weekID, Day: req.Day, SlotID: req.SlotID, Note: req.Note,
	}
	for i, c := range req.Components {
		p.Components = append(p.Components, plate.PlateComponent{
			ComponentID: c.ComponentID, Portions: c.Portions, SortOrder: i,
		})
	}
	if err := h.plates.Create(r.Context(), p); err != nil {
		status, key := toHTTPWithResource(err, "plate")
		writeError(w, status, key)
		return
	}
	// Skip is expressed via a separate atomic call so the initial Create
	// doesn't need a new validation path; this also clears any components
	// the client may have accidentally attached to a skipped plate.
	if req.Skipped {
		skipped, err := h.plates.SetSkipped(r.Context(), p.ID, true, req.Note)
		if err != nil {
			status, key := toHTTPWithResource(err, "plate")
			writeError(w, status, key)
			return
		}
		p = skipped
	}
	writeJSON(w, http.StatusCreated, toPlateResponse(p, nil))
}

// ClearPlates handles DELETE /api/weeks/{id}/plates. Removes every plate in a
// week (used by the Fill-empty revert flow to restore the pre-snapshot state).
func (h *WeekHandler) ClearPlates(w http.ResponseWriter, r *http.Request) {
	weekID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if _, err := h.plates.DeleteByWeek(r.Context(), weekID); err != nil {
		status, key := toHTTPWithResource(err, "week")
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Shopping list and nutrition ---

type macrosResponse struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
	Fiber   float64 `json:"fiber"`
	Sodium  float64 `json:"sodium"`
}

type shoppingListResponse struct {
	Items []shopping.ShoppingItem `json:"items"`
}

type nutritionDayResponse struct {
	Day    int            `json:"day"`
	Macros macrosResponse `json:"macros"`
}

type weekNutritionResponse struct {
	Days []nutritionDayResponse `json:"days"`
	Week macrosResponse         `json:"week"`
}

func toMacrosResponse(m nutrition.Macros) macrosResponse {
	return macrosResponse{
		Kcal: m.Kcal, Protein: m.Protein, Fat: m.Fat,
		Carbs: m.Carbs, Fiber: m.Fiber, Sodium: m.Sodium,
	}
}

// loadWeekData collects all component and ingredient data needed by both the
// shopping-list and nutrition endpoints in a single pass, avoiding duplicate
// service calls when both are requested in the same request.
func (h *WeekHandler) loadWeekData(ctx context.Context, plates []plate.Plate) (
	comps map[int64]*component.Component,
	ingMap map[int64]*ingredient.Ingredient,
	err error,
) {
	// Collect unique component IDs.
	compIDSet := make(map[int64]struct{})
	for _, pl := range plates {
		for _, pc := range pl.Components {
			compIDSet[pc.ComponentID] = struct{}{}
		}
	}

	comps = make(map[int64]*component.Component, len(compIDSet))
	ingIDSet := make(map[int64]struct{})
	for id := range compIDSet {
		c, err := h.components.Get(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		comps[id] = c
		for _, ci := range c.Ingredients {
			ingIDSet[ci.IngredientID] = struct{}{}
		}
	}

	if len(ingIDSet) == 0 {
		return comps, map[int64]*ingredient.Ingredient{}, nil
	}

	ids := make([]int64, 0, len(ingIDSet))
	for id := range ingIDSet {
		ids = append(ids, id)
	}
	ingMap, err = h.ingRepo.LookupForNutrition(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	return comps, ingMap, nil
}

// ShoppingList handles GET /api/weeks/{id}/shopping-list.
func (h *WeekHandler) ShoppingList(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	week, err := h.planner.Get(r.Context(), id)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}

	comps, ingMap, err := h.loadWeekData(r.Context(), week.Plates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	refs := make(map[int64]shopping.ComponentRef, len(comps))
	for compID, c := range comps {
		ings := make([]shopping.ComponentIngredient, 0, len(c.Ingredients))
		for _, ci := range c.Ingredients {
			ing, ok := ingMap[ci.IngredientID]
			if !ok {
				continue
			}
			ings = append(ings, shopping.ComponentIngredient{
				IngredientID: ci.IngredientID,
				Name:         ing.Name,
				Grams:        ci.Grams,
			})
		}
		refs[compID] = shopping.ComponentRef{
			ReferencePortions: c.ReferencePortions,
			Ingredients:       ings,
		}
	}

	items := shopping.FromPlates(week.Plates, refs)
	writeJSON(w, http.StatusOK, shoppingListResponse{Items: items})
}

// Nutrition handles GET /api/weeks/{id}/nutrition.
func (h *WeekHandler) Nutrition(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	week, err := h.planner.Get(r.Context(), id)
	if err != nil {
		status, key := weekError(err)
		writeError(w, status, key)
		return
	}

	comps, ingMap, err := h.loadWeekData(r.Context(), week.Plates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	// Build per-portion macros for each unique component.
	perPortion := make(map[int64]nutrition.Macros, len(comps))
	for compID, c := range comps {
		inputs := make([]nutrition.IngredientInput, 0, len(c.Ingredients))
		for _, ci := range c.Ingredients {
			ing, ok := ingMap[ci.IngredientID]
			if !ok {
				continue
			}
			inputs = append(inputs, nutrition.IngredientInput{
				Per100g: nutrition.Macros{
					Kcal:    ing.Kcal100g,
					Protein: ing.Protein100g,
					Fat:     ing.Fat100g,
					Carbs:   ing.Carbs100g,
					Fiber:   ing.Fiber100g,
					Sodium:  ing.Sodium100g,
				},
				Grams: ci.Grams,
			})
		}
		perPortion[compID] = nutrition.PerPortion(nutrition.ComponentInput{
			Ingredients:       inputs,
			ReferencePortions: c.ReferencePortions,
		})
	}

	// Build DayPlate inputs for the domain aggregator.
	dayPlates := make([]nutrition.DayPlate, 0, len(week.Plates))
	for _, pl := range week.Plates {
		comps := make([]nutrition.PlateComponentInput, 0, len(pl.Components))
		for _, pc := range pl.Components {
			m, ok := perPortion[pc.ComponentID]
			if !ok {
				continue
			}
			comps = append(comps, nutrition.PlateComponentInput{Macros: m, Portions: pc.Portions})
		}
		dayPlates = append(dayPlates, nutrition.DayPlate{Day: pl.Day, Plate: nutrition.PlateInput{Components: comps}})
	}

	totals := nutrition.WeekTotals(dayPlates)

	days := make([]nutritionDayResponse, 0, len(totals.Days))
	for day, m := range totals.Days {
		days = append(days, nutritionDayResponse{Day: day, Macros: toMacrosResponse(m)})
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Day < days[j].Day })

	writeJSON(w, http.StatusOK, weekNutritionResponse{
		Days: days,
		Week: toMacrosResponse(totals.Week),
	})
}
