package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// platesService is the subset of plate.Service used by PlateHandler.
// Defined as an interface so tests can inject a stub without a real DB.
type platesService interface {
	Get(ctx context.Context, id int64) (*plate.Plate, error)
	Create(ctx context.Context, in plate.PlateInput) (*plate.Plate, error)
	UpdateMeal(ctx context.Context, id int64, patch plate.PlatePatch) (plate.MealResult, error)
	MoveMeal(ctx context.Context, plateID int64, to plate.CellKey, occ plate.Occupant) (plate.MealResult, error)
	ReplaceComponent(ctx context.Context, plateID int64, sel plate.Selector, foodID int64, src plate.Source, qty *plate.PlateComponent) (plate.MealResult, error)
	CopyMeal(ctx context.Context, plateID int64, to []plate.CellKey, as plate.CopyAs) (plate.MealResult, error)
	Recorded(ctx context.Context, fn func(ctx context.Context) error) (plate.Undo, error)
	RemoveMeals(ctx context.Context, ids []int64) (plate.MealResult, error)
	AddComponent(ctx context.Context, plateID int64, pc plate.PlateComponent) (*plate.Plate, error)
	SwapComponent(ctx context.Context, componentID, newFoodID int64, quantity *plate.PlateComponent) (*plate.Plate, error)
	UpdateComponentQuantity(ctx context.Context, componentID int64, q plate.PlateComponent) (*plate.Plate, error)
	RemoveComponent(ctx context.Context, componentID int64) (*plate.Plate, error)
	Confirm(ctx context.Context, seed plate.RowSet) (plate.RowSet, error)
	Unlock(ctx context.Context, seed plate.RowSet) (plate.RowSet, error)
	DryRun(ctx context.Context, fn func(ctx context.Context) error) error
}

// PlateHandler exposes plate + plate_component mutation endpoints.
type PlateHandler struct {
	svc platesService
}

// NewPlateHandler creates a PlateHandler backed by a concrete plate.Service.
func NewPlateHandler(svc *plate.Service) *PlateHandler {
	return &PlateHandler{svc: svc}
}

// NewPlateHandlerFromService creates a PlateHandler from any value satisfying
// the platesService interface. Intended for tests that inject a stub.
func NewPlateHandlerFromService(svc platesService) *PlateHandler {
	return &PlateHandler{svc: svc}
}

func plateError(err error) (int, string) {
	return toHTTPWithResource(err, "plate")
}

func writePlateError(w http.ResponseWriter, err error) {
	status, key := plateError(err)
	writeError(w, status, key)
}

// componentInput is the ComponentInput of the plate API: a composed food
// takes portions (per eater, default 1) and optionally the cook session to
// eat from; a leaf food takes amount + unit.
type componentInput struct {
	FoodID        int64    `json:"food_id"`
	Portions      *float64 `json:"portions,omitempty"`
	Amount        *float64 `json:"amount,omitempty"`
	Unit          *string  `json:"unit,omitempty"`
	CookSessionID *int64   `json:"cook_session_id,omitempty"`
	// Position is main, side or extra; empty means derived.
	Position string `json:"position,omitempty"`
}

func (c componentInput) toComponent() plate.PlateComponent {
	return plate.PlateComponent{
		FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, CookSessionID: c.CookSessionID,
		Position: plate.Position(c.Position),
	}
}

type createPlateRequest struct {
	Date       string           `json:"date"`
	SlotID     int64            `json:"slot_id"`
	Eaters     *float64         `json:"eaters"`
	Note       *string          `json:"note"`
	Components []componentInput `json:"components"`
}

// Create handles POST /api/plates.
func (h *PlateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createPlateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	d, err := time.Parse("2006-01-02", req.Date)
	if err != nil || req.SlotID <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	in := plate.PlateInput{Date: d, SlotID: req.SlotID, Eaters: req.Eaters, Note: req.Note}
	for _, c := range req.Components {
		in.Components = append(in.Components, c.toComponent())
	}
	p, err := h.svc.Create(r.Context(), in)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPlateResponse(p))
}

// Confirm handles POST /api/plates/{id}/confirm: it confirms the plate's
// batch closure and returns {plate_ids, session_ids}.
func (h *PlateHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseIDParam(w, r, "id"); ok {
		writeStateChange(w, r, plate.RowSet{PlateIDs: []int64{id}}, h.svc.Confirm)
	}
}

// Unlock handles POST /api/plates/{id}/unlock: it returns the plate's batch
// closure to draft and returns {plate_ids, session_ids}.
func (h *PlateHandler) Unlock(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseIDParam(w, r, "id"); ok {
		writeStateChange(w, r, plate.RowSet{PlateIDs: []int64{id}}, h.svc.Unlock)
	}
}

// Get handles GET /api/plates/{id}.
func (h *PlateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(p))
}

type updatePlateRequest struct {
	SlotID  *int64   `json:"slot_id"`
	Date    *string  `json:"date"`
	Eaters  *float64 `json:"eaters"`
	Note    *string  `json:"note"`
	NoteSet bool     `json:"-"`
}

func (r *updatePlateRequest) UnmarshalJSON(b []byte) error {
	type alias struct {
		SlotID *int64   `json:"slot_id"`
		Date   *string  `json:"date"`
		Eaters *float64 `json:"eaters"`
		Note   *string  `json:"note"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	r.SlotID, r.Date, r.Eaters, r.Note = a.SlotID, a.Date, a.Eaters, a.Note
	// Detect whether "note" key was present in the JSON.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	_, r.NoteSet = raw["note"]
	return nil
}

// Update handles PUT /api/plates/{id} with body {date?, slot_id?, eaters?, note?}.
func (h *PlateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req updatePlateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	patch := plate.PlatePatch{SlotID: req.SlotID, Eaters: req.Eaters, Note: req.Note, SetNote: req.NoteSet}
	if req.Date != nil && *req.Date != "" {
		d, err := time.Parse("2006-01-02", *req.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		patch.Date = &d
	}
	res, err := h.svc.UpdateMeal(r.Context(), id, patch)
	if err != nil {
		writeMealError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plateChangeResponse{
		plateResponse: toPlateResponse(&res.Plates[0]), AlsoChanged: alsoChanged(res.AlsoChanged), Undo: undoOf(res.Undo),
	})
}

// plateChangeResponse is a plate with what else changed and the revert
// token of the change (POST /api/plan/revert).
type plateChangeResponse struct {
	plateResponse
	AlsoChanged []plate.AlsoChanged `json:"also_changed"`
	Undo        plate.Undo          `json:"undo"`
}

func alsoChanged(a []plate.AlsoChanged) []plate.AlsoChanged {
	if a == nil {
		return []plate.AlsoChanged{}
	}
	return a
}

func undoOf(u plate.Undo) plate.Undo {
	if u.Cells == nil {
		u.Cells = []plate.CellUndo{}
	}
	return u
}

// writeComponentChange runs a component edit under plate.Service.Recorded
// and writes the plate with its undo token.
func (h *PlateHandler) writeComponentChange(w http.ResponseWriter, r *http.Request, status int, edit func(ctx context.Context) (*plate.Plate, error)) {
	var p *plate.Plate
	undo, err := h.svc.Recorded(r.Context(), func(ctx context.Context) error {
		var err error
		p, err = edit(ctx)
		return err
	})
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, status, plateChangeResponse{plateResponse: toPlateResponse(p), AlsoChanged: []plate.AlsoChanged{}, Undo: undoOf(undo)})
}

// Delete handles DELETE /api/plates/{id} and returns what restores it.
func (h *PlateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	res, err := h.svc.RemoveMeals(r.Context(), []int64{id})
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// AddComponent handles POST /api/plates/{id}/components and returns the plate.
func (h *PlateHandler) AddComponent(w http.ResponseWriter, r *http.Request) {
	plateID, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req componentInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	h.writeComponentChange(w, r, http.StatusCreated, func(ctx context.Context) (*plate.Plate, error) {
		return h.svc.AddComponent(ctx, plateID, req.toComponent())
	})
}

// updateComponentRequest supports two modes:
//   - Swap: food_id present (with optional new quantity for the new food)
//   - Quantity update: any of portions/amount/unit set (food_id absent)
type updateComponentRequest struct {
	FoodID   *int64   `json:"food_id,omitempty"`
	Portions *float64 `json:"portions,omitempty"`
	Amount   *float64 `json:"amount,omitempty"`
	Unit     *string  `json:"unit,omitempty"`
}

func (req updateComponentRequest) hasQuantity() bool {
	return req.Portions != nil || req.Amount != nil || req.Unit != nil
}

func (req updateComponentRequest) toQuantity() plate.PlateComponent {
	return plate.PlateComponent{Portions: req.Portions, Amount: req.Amount, Unit: req.Unit}
}

// UpdateComponent handles PUT /api/plates/{id}/components/{pcId} and returns
// the plate. Supports two modes: swap (food_id provided) or quantity update.
func (h *PlateHandler) UpdateComponent(w http.ResponseWriter, r *http.Request) {
	pcID, ok := parseIDParam(w, r, "pcId")
	if !ok {
		return
	}
	var req updateComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	switch {
	case req.FoodID != nil:
		var override *plate.PlateComponent
		if req.hasQuantity() {
			q := req.toQuantity()
			override = &q
		}
		h.writeComponentChange(w, r, http.StatusOK, func(ctx context.Context) (*plate.Plate, error) {
			return h.svc.SwapComponent(ctx, pcID, *req.FoodID, override)
		})
	case req.hasQuantity():
		h.writeComponentChange(w, r, http.StatusOK, func(ctx context.Context) (*plate.Plate, error) {
			return h.svc.UpdateComponentQuantity(ctx, pcID, req.toQuantity())
		})
	default:
		writeError(w, http.StatusBadRequest, "error.invalid_body")
	}
}

// DeleteComponent handles DELETE /api/plates/{id}/components/{pcId} and
// returns the plate.
func (h *PlateHandler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	pcID, ok := parseIDParam(w, r, "pcId")
	if !ok {
		return
	}
	h.writeComponentChange(w, r, http.StatusOK, func(ctx context.Context) (*plate.Plate, error) {
		return h.svc.RemoveComponent(ctx, pcID)
	})
}

// parseIDParam parses the int64 URL parameter name, writing 400
// error.invalid_id when it is not a number.
func parseIDParam(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return 0, false
	}
	return id, true
}
