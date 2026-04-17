package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
)

// TemplateHandler exposes template CRUD + apply endpoints.
type TemplateHandler struct {
	svc *template.Service
}

// NewTemplateHandler creates a TemplateHandler.
func NewTemplateHandler(svc *template.Service) *TemplateHandler {
	return &TemplateHandler{svc: svc}
}

type templateComponentInlineInput struct {
	ComponentID int64   `json:"component_id"`
	Portions    float64 `json:"portions"`
}

type createTemplateRequest struct {
	Name        string                         `json:"name"`
	FromPlateID *int64                         `json:"from_plate_id,omitempty"`
	Components  []templateComponentInlineInput `json:"components,omitempty"`
}

type updateTemplateRequest struct {
	Name string `json:"name"`
}

type applyTemplateRequest struct {
	PlateID int64 `json:"plate_id"`
	Merge   bool  `json:"merge,omitempty"`
}

type templateComponentResponse struct {
	ID          int64   `json:"id"`
	TemplateID  int64   `json:"template_id"`
	ComponentID int64   `json:"component_id"`
	Portions    float64 `json:"portions"`
	SortOrder   int     `json:"sort_order"`
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
		ID:          tc.ID,
		TemplateID:  tc.TemplateID,
		ComponentID: tc.ComponentID,
		Portions:    tc.Portions,
		SortOrder:   tc.SortOrder,
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
func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	var comps []template.TemplateComponent
	if len(req.Components) > 0 {
		comps = make([]template.TemplateComponent, len(req.Components))
		for i, c := range req.Components {
			comps[i] = template.TemplateComponent{
				ComponentID: c.ComponentID,
				Portions:    c.Portions,
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

// Apply handles POST /api/templates/{id}/apply.
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
	if req.PlateID <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err := h.svc.Apply(r.Context(), id, req.PlateID, req.Merge); err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
