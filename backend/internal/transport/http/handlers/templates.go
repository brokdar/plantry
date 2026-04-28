package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
)

// templateService is the subset of template.Service used by TemplateHandler.
type templateService interface {
	List(ctx context.Context) ([]template.Template, error)
	Create(ctx context.Context, name string, fromPlateID *int64, components []template.TemplateComponent) (*template.Template, error)
	Get(ctx context.Context, id int64) (*template.Template, error)
	UpdateName(ctx context.Context, id int64, name string) (*template.Template, error)
	Delete(ctx context.Context, id int64) error
	Apply(ctx context.Context, templateID int64, startDate time.Time, slotID int64) ([]plate.Plate, error)
	SaveAsTemplate(ctx context.Context, name string, plates []plate.Plate, anchorDate time.Time) (*template.Template, error)
}

// rangeReader reads plates over a date range.
type rangeReader interface {
	Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

// TemplateHandler exposes template CRUD + apply endpoints.
type TemplateHandler struct {
	svc    templateService
	plates rangeReader
}

// NewTemplateHandler creates a TemplateHandler.
func NewTemplateHandler(svc *template.Service, plates *plate.Service) *TemplateHandler {
	return &TemplateHandler{svc: svc, plates: plates}
}

// NewTemplateHandlerFromInterfaces creates a TemplateHandler from service
// interfaces. Intended for tests that inject stubs.
func NewTemplateHandlerFromInterfaces(svc templateService, plates rangeReader) *TemplateHandler {
	return &TemplateHandler{svc: svc, plates: plates}
}

type templateComponentInlineInput struct {
	FoodID   int64   `json:"food_id"`
	Portions float64 `json:"portions"`
}

type createTemplateRequest struct {
	Name        string                         `json:"name"`
	FromPlateID *int64                         `json:"from_plate_id,omitempty"`
	Components  []templateComponentInlineInput `json:"components,omitempty"`
	// Range-based creation: build template from existing plates in [From, To].
	From *string `json:"from,omitempty"`
	To   *string `json:"to,omitempty"`
}

type updateTemplateRequest struct {
	Name string `json:"name"`
}

type applyTemplateRequest struct {
	StartDate string `json:"start_date"`
	SlotID    int64  `json:"slot_id"`
}

type templateComponentResponse struct {
	ID         int64   `json:"id"`
	TemplateID int64   `json:"template_id"`
	FoodID     int64   `json:"food_id"`
	Portions   float64 `json:"portions"`
	SortOrder  int     `json:"sort_order"`
	DayOffset  int     `json:"day_offset"`
}

type templateResponse struct {
	ID         int64                       `json:"id"`
	Name       string                      `json:"name"`
	Components []templateComponentResponse `json:"components"`
	CreatedAt  string                      `json:"created_at"`
}

type templateListResponse struct {
	Items []templateResponse `json:"items"`
}

func toTemplateComponentResponse(tc template.TemplateComponent) templateComponentResponse {
	return templateComponentResponse{
		ID:         tc.ID,
		TemplateID: tc.TemplateID,
		FoodID:     tc.FoodID,
		Portions:   tc.Portions,
		SortOrder:  tc.SortOrder,
		DayOffset:  tc.DayOffset,
	}
}

func toTemplateResponse(t *template.Template) templateResponse {
	comps := make([]templateComponentResponse, len(t.Components))
	for i, tc := range t.Components {
		comps[i] = toTemplateComponentResponse(tc)
	}
	var createdAt string
	if !t.CreatedAt.IsZero() {
		createdAt = t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return templateResponse{
		ID:         t.ID,
		Name:       t.Name,
		Components: comps,
		CreatedAt:  createdAt,
	}
}

func templateError(err error) (int, string) {
	return toHTTPWithResource(err, "template")
}

// List handles GET /api/templates.
func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}
	out := templateListResponse{Items: make([]templateResponse, len(items))}
	for i := range items {
		out.Items[i] = toTemplateResponse(&items[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// Create handles POST /api/templates.
// If {from, to} fields are provided, builds the template from existing plates
// in that date range using SaveAsTemplate. Otherwise creates normally.
func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	// Range-based creation path.
	if req.From != nil || req.To != nil {
		if req.From == nil || req.To == nil {
			writeError(w, http.StatusBadRequest, "error.invalid_date_range")
			return
		}
		from, err := time.Parse("2006-01-02", *req.From)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_date")
			return
		}
		to, err := time.Parse("2006-01-02", *req.To)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_date")
			return
		}
		if from.After(to) {
			writeError(w, http.StatusBadRequest, "error.invalid_date_range")
			return
		}
		plates, err := h.plates.Range(r.Context(), from, to)
		if err != nil {
			status, key := templateError(err)
			writeError(w, status, key)
			return
		}
		t, err := h.svc.SaveAsTemplate(r.Context(), req.Name, plates, from)
		if err != nil {
			status, key := templateError(err)
			writeError(w, status, key)
			return
		}
		writeJSON(w, http.StatusCreated, toTemplateResponse(t))
		return
	}

	var comps []template.TemplateComponent
	if len(req.Components) > 0 {
		comps = make([]template.TemplateComponent, len(req.Components))
		for i, c := range req.Components {
			comps[i] = template.TemplateComponent{
				FoodID:   c.FoodID,
				Portions: c.Portions,
			}
		}
	}
	t, err := h.svc.Create(r.Context(), req.Name, req.FromPlateID, comps)
	if err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toTemplateResponse(t))
}

// Get handles GET /api/templates/{id}.
func (h *TemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toTemplateResponse(t))
}

// Update handles PUT /api/templates/{id}.
func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req updateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	t, err := h.svc.UpdateName(r.Context(), id, req.Name)
	if err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toTemplateResponse(t))
}

// Delete handles DELETE /api/templates/{id}.
func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// plateResponse mirrors the plates handler response shape used here for Apply output.
type applyTemplatesResponse struct {
	Plates []plateApplyItem `json:"plates"`
}

type plateApplyItem struct {
	ID     int64  `json:"id"`
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

// Apply handles POST /api/templates/{id}/apply.
// Body: {"start_date": "YYYY-MM-DD", "slot_id": N}
func (h *TemplateHandler) Apply(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req applyTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.StartDate == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_date")
		return
	}
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_date")
		return
	}
	if req.SlotID <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	created, err := h.svc.Apply(r.Context(), id, startDate, req.SlotID)
	if err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}
	items := make([]plateApplyItem, len(created))
	for i, p := range created {
		items[i] = plateApplyItem{
			ID:     p.ID,
			Date:   p.DateString(),
			SlotID: p.SlotID,
		}
	}
	writeJSON(w, http.StatusOK, applyTemplatesResponse{Plates: items})
}
