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
	ListByScope(ctx context.Context, scope template.Scope) ([]template.Template, error)
	Create(ctx context.Context, name string, scope template.Scope, fromPlateID *int64, entries []template.TemplateEntry) (*template.Template, error)
	Get(ctx context.Context, id int64) (*template.Template, error)
	UpdateName(ctx context.Context, id int64, name string) (*template.Template, error)
	Delete(ctx context.Context, id int64) error
	Apply(ctx context.Context, templateID int64, payload template.ApplyPayload) (*template.ApplyResult, error)
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

type templateEntryInlineInput struct {
	FoodID    int64   `json:"food_id"`
	Portions  float64 `json:"portions"`
	DayOffset int     `json:"day_offset"`
	SlotID    *int64  `json:"slot_id,omitempty"`
	Note      *string `json:"note,omitempty"`
}

type createTemplateRequest struct {
	Name        string                     `json:"name"`
	Scope       string                     `json:"scope,omitempty"`
	FromPlateID *int64                     `json:"from_plate_id,omitempty"`
	Components  []templateEntryInlineInput `json:"components,omitempty"`
	Entries     []templateEntryInlineInput `json:"entries,omitempty"`
	// Range-based creation: build a week-scope template from existing plates in [From, To].
	From *string `json:"from,omitempty"`
	To   *string `json:"to,omitempty"`
}

type updateTemplateRequest struct {
	Name string `json:"name"`
}

// applyTemplateRequest is the union shape accepted by POST /templates/{id}/apply.
// Validation is scope-driven: the handler loads the template's scope and
// requires the right subset of fields. Legacy clients may send {start_date,
// slot_id} for slot-scope templates.
type applyTemplateRequest struct {
	Date      *string `json:"date,omitempty"`
	SlotID    *int64  `json:"slot_id,omitempty"`
	StartDate *string `json:"start_date,omitempty"`
	Conflict  string  `json:"conflict,omitempty"`
}

type templateEntryResponse struct {
	ID         int64   `json:"id"`
	TemplateID int64   `json:"template_id"`
	FoodID     int64   `json:"food_id"`
	Portions   float64 `json:"portions"`
	SortOrder  int     `json:"sort_order"`
	DayOffset  int     `json:"day_offset"`
	SlotID     *int64  `json:"slot_id,omitempty"`
	Note       *string `json:"note,omitempty"`
}

type templateResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Scope string `json:"scope"`
	// Components is the JSON field name kept stable for backward compatibility
	// with existing frontend clients. Each item is a TemplateEntry.
	Components []templateEntryResponse `json:"components"`
	CreatedAt  string                  `json:"created_at"`
}

type templateListResponse struct {
	Items []templateResponse `json:"items"`
}

func toTemplateEntryResponse(te template.TemplateEntry) templateEntryResponse {
	return templateEntryResponse{
		ID:         te.ID,
		TemplateID: te.TemplateID,
		FoodID:     te.FoodID,
		Portions:   te.Portions,
		SortOrder:  te.SortOrder,
		DayOffset:  te.DayOffset,
		SlotID:     te.SlotID,
		Note:       te.Note,
	}
}

func toTemplateResponse(t *template.Template) templateResponse {
	entries := make([]templateEntryResponse, len(t.Entries))
	for i, te := range t.Entries {
		entries[i] = toTemplateEntryResponse(te)
	}
	scope := string(t.Scope)
	if scope == "" {
		scope = string(template.ScopeSlot)
	}
	var createdAt string
	if !t.CreatedAt.IsZero() {
		createdAt = t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return templateResponse{
		ID:         t.ID,
		Name:       t.Name,
		Scope:      scope,
		Components: entries,
		CreatedAt:  createdAt,
	}
}

func templateError(err error) (int, string) {
	return toHTTPWithResource(err, "template")
}

// List handles GET /api/templates. Optional ?scope=slot|day|week filter.
func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	var (
		items []template.Template
		err   error
	)
	if scopeQ := r.URL.Query().Get("scope"); scopeQ != "" {
		s := template.Scope(scopeQ)
		if !s.IsValid() {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		items, err = h.svc.ListByScope(r.Context(), s)
	} else {
		items, err = h.svc.List(r.Context())
	}
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
//   - {from, to}                  → builds a week-scope template from existing
//     plates in that range via SaveAsTemplate.
//   - {scope?, from_plate_id}     → slot-scope template cloned from the plate.
//   - {scope?, entries|components}→ template with explicit entries.
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

	scope := template.Scope(req.Scope)
	if scope == "" {
		scope = template.ScopeSlot
	}

	// Backward-compat: accept the old "components" field name as a synonym for "entries".
	rawEntries := req.Entries
	if len(rawEntries) == 0 {
		rawEntries = req.Components
	}

	var entries []template.TemplateEntry
	if len(rawEntries) > 0 {
		entries = make([]template.TemplateEntry, len(rawEntries))
		for i, e := range rawEntries {
			entries[i] = template.TemplateEntry{
				FoodID:    e.FoodID,
				Portions:  e.Portions,
				DayOffset: e.DayOffset,
				SlotID:    e.SlotID,
				Note:      e.Note,
			}
		}
	}
	t, err := h.svc.Create(r.Context(), req.Name, scope, req.FromPlateID, entries)
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

type applyTemplatesResponse struct {
	Plates  []plateApplyItem    `json:"plates"`
	Skipped []plateApplySkipped `json:"skipped,omitempty"`
}

type plateApplyItem struct {
	ID     int64  `json:"id"`
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

type plateApplySkipped struct {
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

// Apply handles POST /api/templates/{id}/apply. Payload shape depends on the
// template's stored scope. See applyTemplateRequest. For backward compatibility
// with slot-scope clients, {start_date, slot_id} is accepted as an alias for
// {date, slot_id}.
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

	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}

	scope := t.Scope
	if scope == "" {
		scope = template.ScopeSlot
	}

	payload, err := buildApplyPayload(scope, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.Apply(r.Context(), id, payload)
	if err != nil {
		status, key := templateError(err)
		writeError(w, status, key)
		return
	}
	items := make([]plateApplyItem, len(result.Created))
	for i, p := range result.Created {
		items[i] = plateApplyItem{
			ID:     p.ID,
			Date:   p.DateString(),
			SlotID: p.SlotID,
		}
	}
	skipped := make([]plateApplySkipped, len(result.Skipped))
	for i, s := range result.Skipped {
		skipped[i] = plateApplySkipped{
			Date:   s.Date.Format("2006-01-02"),
			SlotID: s.SlotID,
		}
	}
	writeJSON(w, http.StatusOK, applyTemplatesResponse{Plates: items, Skipped: skipped})
}

// buildApplyPayload validates an applyTemplateRequest against a template scope
// and returns the strongly-typed ApplyPayload. The string error is one of the
// public message keys returned to clients.
func buildApplyPayload(scope template.Scope, req applyTemplateRequest) (template.ApplyPayload, error) {
	parseDate := func(s string) (*time.Time, error) {
		if s == "" {
			return nil, nil
		}
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			return nil, err
		}
		return &d, nil
	}

	switch scope {
	case template.ScopeSlot:
		// Accept date or legacy start_date.
		raw := ""
		if req.Date != nil {
			raw = *req.Date
		} else if req.StartDate != nil {
			raw = *req.StartDate
		}
		if raw == "" {
			return template.ApplyPayload{}, errMsg("error.invalid_date")
		}
		date, err := parseDate(raw)
		if err != nil {
			return template.ApplyPayload{}, errMsg("error.invalid_date")
		}
		if req.SlotID == nil || *req.SlotID <= 0 {
			return template.ApplyPayload{}, errMsg("error.invalid_body")
		}
		return template.ApplyPayload{Date: date, SlotID: req.SlotID}, nil

	case template.ScopeDay:
		raw := ""
		if req.Date != nil {
			raw = *req.Date
		} else if req.StartDate != nil {
			raw = *req.StartDate
		}
		if raw == "" {
			return template.ApplyPayload{}, errMsg("error.invalid_date")
		}
		date, err := parseDate(raw)
		if err != nil {
			return template.ApplyPayload{}, errMsg("error.invalid_date")
		}
		conflict, err := parseConflict(req.Conflict)
		if err != nil {
			return template.ApplyPayload{}, err
		}
		return template.ApplyPayload{Date: date, Conflict: conflict}, nil

	case template.ScopeWeek:
		raw := ""
		if req.StartDate != nil {
			raw = *req.StartDate
		} else if req.Date != nil {
			raw = *req.Date
		}
		if raw == "" {
			return template.ApplyPayload{}, errMsg("error.invalid_date")
		}
		date, err := parseDate(raw)
		if err != nil {
			return template.ApplyPayload{}, errMsg("error.invalid_date")
		}
		conflict, err := parseConflict(req.Conflict)
		if err != nil {
			return template.ApplyPayload{}, err
		}
		return template.ApplyPayload{StartDate: date, Conflict: conflict}, nil
	}

	return template.ApplyPayload{}, errMsg("error.invalid_body")
}

func parseConflict(s string) (template.ApplyConflict, error) {
	switch s {
	case "":
		return template.ConflictSkip, nil
	case string(template.ConflictSkip):
		return template.ConflictSkip, nil
	case string(template.ConflictOverwrite):
		return template.ConflictOverwrite, nil
	default:
		return "", errMsg("error.invalid_body")
	}
}

// errMsg is a small string-as-error wrapper for buildApplyPayload's flat
// returns. The string is a message key passed straight to writeError.
type errMsg string

func (e errMsg) Error() string { return string(e) }
