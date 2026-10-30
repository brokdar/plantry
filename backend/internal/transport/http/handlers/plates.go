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
	Update(ctx context.Context, id int64, patch plate.PlatePatch) (*plate.Plate, error)
	Delete(ctx context.Context, id int64) (plate.PlanSnapshot, error)
	Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
	Day(ctx context.Context, date time.Time) ([]plate.Plate, error)
	AddComponent(ctx context.Context, plateID int64, pc plate.PlateComponent) (*plate.Plate, error)
	SwapComponent(ctx context.Context, componentID, newFoodID int64, quantity *plate.PlateComponent) (*plate.Plate, error)
	UpdateComponentQuantity(ctx context.Context, componentID int64, q plate.PlateComponent) (*plate.Plate, error)
	RemoveComponent(ctx context.Context, componentID int64) (*plate.Plate, error)
	RecentUnitForFood(ctx context.Context, foodID int64) (string, error)
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
}

func (c componentInput) toComponent() plate.PlateComponent {
	return plate.PlateComponent{
		FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, CookSessionID: c.CookSessionID,
	}
}

// List handles GET /api/plates?from=YYYY-MM-DD&to=YYYY-MM-DD.
func (h *PlateHandler) List(w http.ResponseWriter, r *http.Request) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if from.After(to) {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	plates, err := h.svc.Range(r.Context(), from, to)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, platesListResponse(plates))
}

// Day handles GET /api/plates/by-date/{date}.
func (h *PlateHandler) Day(w http.ResponseWriter, r *http.Request) {
	dateStr := chi.URLParam(r, "date")
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	plates, err := h.svc.Day(r.Context(), d)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, platesListResponse(plates))
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
	p, err := h.svc.Update(r.Context(), id, patch)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(p))
}

// Delete handles DELETE /api/plates/{id} and returns what restores it.
func (h *PlateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	snap, err := h.svc.Delete(r.Context(), id)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, removedResponse{Removed: snap})
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
	p, err := h.svc.AddComponent(r.Context(), plateID, req.toComponent())
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPlateResponse(p))
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
	var (
		p   *plate.Plate
		err error
	)
	switch {
	case req.FoodID != nil:
		var override *plate.PlateComponent
		if req.hasQuantity() {
			q := req.toQuantity()
			override = &q
		}
		p, err = h.svc.SwapComponent(r.Context(), pcID, *req.FoodID, override)
	case req.hasQuantity():
		p, err = h.svc.UpdateComponentQuantity(r.Context(), pcID, req.toQuantity())
	default:
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(p))
}

// DeleteComponent handles DELETE /api/plates/{id}/components/{pcId} and
// returns the plate.
func (h *PlateHandler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	pcID, ok := parseIDParam(w, r, "pcId")
	if !ok {
		return
	}
	p, err := h.svc.RemoveComponent(r.Context(), pcID)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(p))
}

// recentUnitResponse is returned by GET /api/foods/{id}/recent-unit. Unit is
// nil when the food has never been used as a leaf component (or has never been
// added to any plate). The frontend uses this to default the quantity-unit
// input on the planner picker.
type recentUnitResponse struct {
	Unit *string `json:"unit"`
}

// RecentUnit handles GET /api/foods/{id}/recent-unit.
//
// Path lives under /api/foods to keep food-centric reads grouped client-side,
// even though the data is owned by the plate aggregate.
func (h *PlateHandler) RecentUnit(w http.ResponseWriter, r *http.Request) {
	foodID, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	unit, err := h.svc.RecentUnitForFood(r.Context(), foodID)
	if err != nil {
		writePlateError(w, err)
		return
	}
	resp := recentUnitResponse{}
	if unit != "" {
		v := unit
		resp.Unit = &v
	}
	writeJSON(w, http.StatusOK, resp)
}

// platesListResponse wraps a slice of plates in the standard envelope.
func platesListResponse(plates []plate.Plate) map[string]any {
	resp := make([]plateResponse, len(plates))
	for i := range plates {
		resp[i] = toPlateResponse(&plates[i])
	}
	return map[string]any{"plates": resp}
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
