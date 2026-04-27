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
	Update(ctx context.Context, p *plate.Plate) error
	Delete(ctx context.Context, id int64) error
	Create(ctx context.Context, p *plate.Plate) error
	Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
	Day(ctx context.Context, date time.Time) ([]plate.Plate, error)
	AddComponent(ctx context.Context, plateID, foodID int64, portions float64) (*plate.PlateComponent, error)
	SwapComponent(ctx context.Context, plateComponentID, newFoodID int64, portionsOverride *float64) (*plate.PlateComponent, error)
	UpdateComponentPortions(ctx context.Context, plateComponentID int64, portions float64) (*plate.PlateComponent, error)
	RemoveComponent(ctx context.Context, plateComponentID int64) error
	SetSkipped(ctx context.Context, plateID int64, skipped bool, note *string) (*plate.Plate, error)
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

type updatePlateRequest struct {
	SlotID  *int64  `json:"slot_id"`
	Note    *string `json:"note"`
	Date    *string `json:"date"`
	NoteSet bool    `json:"-"`
}

func (r *updatePlateRequest) UnmarshalJSON(b []byte) error {
	type alias struct {
		SlotID *int64  `json:"slot_id"`
		Note   *string `json:"note"`
		Date   *string `json:"date"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	r.SlotID = a.SlotID
	r.Note = a.Note
	r.Date = a.Date
	// Detect whether "note" key was present in the JSON.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	_, r.NoteSet = raw["note"]
	return nil
}

func plateError(err error) (int, string) {
	return toHTTPWithResource(err, "plate")
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
		status, key := plateError(err)
		writeError(w, status, key)
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
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, platesListResponse(plates))
}

type createPlateByDateRequest struct {
	Date   string  `json:"date"`
	SlotID int64   `json:"slot_id"`
	Note   *string `json:"note"`
}

// Create handles POST /api/plates with body {date, slot_id, note?}.
func (h *PlateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createPlateByDateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.Date == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	d, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.SlotID <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	p := &plate.Plate{
		Date:   d,
		SlotID: req.SlotID,
		Note:   req.Note,
	}
	if err := h.svc.Create(r.Context(), p); err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toPlateResponse(p, nil))
}

// Get handles GET /api/plates/{id}.
func (h *PlateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(p, nil))
}

// Update handles PUT /api/plates/{id}.
func (h *PlateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	var req updatePlateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.SlotID != nil {
		existing.SlotID = *req.SlotID
	}
	if req.NoteSet {
		existing.Note = req.Note
	}
	if req.Date != nil && *req.Date != "" {
		d, err := time.Parse("2006-01-02", *req.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		existing.Date = d
	}
	if err := h.svc.Update(r.Context(), existing); err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(existing, nil))
}

// Delete handles DELETE /api/plates/{id}.
func (h *PlateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type addComponentRequest struct {
	FoodID   int64   `json:"food_id"`
	Portions float64 `json:"portions"`
}

// AddComponent handles POST /api/plates/{id}/components.
func (h *PlateHandler) AddComponent(w http.ResponseWriter, r *http.Request) {
	plateID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req addComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	pc, err := h.svc.AddComponent(r.Context(), plateID, req.FoodID, req.Portions)
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toPlateComponentResponse(pc))
}

type updateComponentRequest struct {
	FoodID   *int64   `json:"food_id"`
	Portions *float64 `json:"portions"`
}

// UpdateComponent handles PUT /api/plates/{id}/components/{pcId}.
// Supports two modes: swap (food_id provided) or rescale (portions only).
func (h *PlateHandler) UpdateComponent(w http.ResponseWriter, r *http.Request) {
	pcID, err := strconv.ParseInt(chi.URLParam(r, "pcId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req updateComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	var pc *plate.PlateComponent
	switch {
	case req.FoodID != nil:
		pc, err = h.svc.SwapComponent(r.Context(), pcID, *req.FoodID, req.Portions)
	case req.Portions != nil:
		pc, err = h.svc.UpdateComponentPortions(r.Context(), pcID, *req.Portions)
	default:
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPlateComponentResponse(pc))
}

type skipRequest struct {
	Skipped bool    `json:"skipped"`
	Note    *string `json:"note"`
}

// SetSkipped handles POST /api/plates/{id}/skip.
// Marks a slot as prospectively skipped (eating out / canteen) so the fill-empty
// kitchen agent leaves it alone. Enabling skip clears attached components.
func (h *PlateHandler) SetSkipped(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req skipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	p, err := h.svc.SetSkipped(r.Context(), id, req.Skipped, req.Note)
	if err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPlateResponse(p, nil))
}

// DeleteComponent handles DELETE /api/plates/{id}/components/{pcId}.
func (h *PlateHandler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	pcID, err := strconv.ParseInt(chi.URLParam(r, "pcId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.RemoveComponent(r.Context(), pcID); err != nil {
		status, key := plateError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// platesListResponse wraps a slice of plates in the standard envelope.
func platesListResponse(plates []plate.Plate) map[string]any {
	resp := make([]plateResponse, len(plates))
	for i := range plates {
		resp[i] = toPlateResponse(&plates[i], nil)
	}
	return map[string]any{"plates": resp}
}
