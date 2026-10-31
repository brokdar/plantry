package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
)

// presetService is the subset of preset.Service used by PresetHandler.
type presetService interface {
	CreateFromPlates(ctx context.Context, in preset.CreateFromPlatesInput) (*preset.Preset, error)
	Create(ctx context.Context, in preset.CreateInput) (*preset.Preset, error)
	Get(ctx context.Context, id int64) (*preset.Preset, error)
	List(ctx context.Context, filter preset.ListFilter) (*preset.ListResult, error)
	UpdateName(ctx context.Context, id int64, name string) (*preset.Preset, error)
	Update(ctx context.Context, id int64, in preset.UpdateInput) (*preset.Preset, error)
	Patch(ctx context.Context, id int64, in preset.PatchInput) (*preset.Preset, error)
	Delete(ctx context.Context, id int64) error
	Duplicate(ctx context.Context, id int64) (*preset.Preset, error)
	KnownTags(ctx context.Context, limit int) ([]preset.TagUsage, error)
	Apply(ctx context.Context, presetID int64, req preset.ApplyRequest) (*preset.ApplyResult, error)
	CopyWeek(ctx context.Context, req preset.CopyWeekRequest) (*preset.ApplyResult, error)
}

// PresetHandler exposes preset CRUD endpoints. Apply + copy-week routes are
// registered separately because they depend on a different service surface.
type PresetHandler struct {
	svc presetService
}

// NewPresetHandler creates a PresetHandler.
func NewPresetHandler(svc *preset.Service) *PresetHandler {
	return &PresetHandler{svc: svc}
}

// NewPresetHandlerFromInterfaces creates a PresetHandler from service
// interfaces. Intended for tests that inject stubs.
func NewPresetHandlerFromInterfaces(svc presetService) *PresetHandler {
	return &PresetHandler{svc: svc}
}

type presetComponentResponse struct {
	ID          int64    `json:"id"`
	FoodID      int64    `json:"food_id"`
	Portions    *float64 `json:"portions,omitempty"`
	Amount      *float64 `json:"amount,omitempty"`
	Unit        *string  `json:"unit,omitempty"`
	Grams       *float64 `json:"grams,omitempty"`
	GramsSource *string  `json:"grams_source,omitempty"`
	Note        *string  `json:"note,omitempty"`
	SortOrder   int      `json:"sort_order"`
}

type presetPlateResponse struct {
	ID         int64                     `json:"id"`
	SlotID     int64                     `json:"slot_id"`
	SortOrder  int                       `json:"sort_order"`
	Components []presetComponentResponse `json:"components"`
}

type presetResponse struct {
	ID         int64                 `json:"id"`
	Name       string                `json:"name"`
	Tags       []string              `json:"tags"`
	Plates     []presetPlateResponse `json:"plates"`
	CreatedAt  string                `json:"created_at"`
	UpdatedAt  string                `json:"updated_at"`
	LastUsedAt *string               `json:"last_used_at,omitempty"`
}

type presetListResponse struct {
	Items     []presetResponse `json:"items"`
	Total     int              `json:"total"`
	KnownTags []knownTagItem   `json:"known_tags"`
}

type knownTagItem struct {
	Tag   string `json:"tag"`
	Count int64  `json:"count"`
}

func toPresetResponse(p *preset.Preset) presetResponse {
	plates := make([]presetPlateResponse, len(p.Plates))
	for i, pp := range p.Plates {
		comps := make([]presetComponentResponse, len(pp.Components))
		for j, c := range pp.Components {
			comps[j] = presetComponentResponse{
				ID:          c.ID,
				FoodID:      c.FoodID,
				Portions:    c.Portions,
				Amount:      c.Amount,
				Unit:        c.Unit,
				Grams:       c.Grams,
				GramsSource: c.GramsSource,
				Note:        c.Note,
				SortOrder:   c.SortOrder,
			}
		}
		plates[i] = presetPlateResponse{
			ID:         pp.ID,
			SlotID:     pp.SlotID,
			SortOrder:  pp.SortOrder,
			Components: comps,
		}
	}
	r := presetResponse{
		ID:        p.ID,
		Name:      p.Name,
		Tags:      p.Tags,
		Plates:    plates,
		CreatedAt: formatRFC(p.CreatedAt),
		UpdatedAt: formatRFC(p.UpdatedAt),
	}
	if r.Tags == nil {
		r.Tags = []string{}
	}
	if p.LastUsedAt != nil {
		s := formatRFC(*p.LastUsedAt)
		r.LastUsedAt = &s
	}
	return r
}

func formatRFC(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func presetError(err error) (int, string) {
	return toHTTPWithResource(err, "preset")
}

// List handles GET /api/presets. Query params: search, slot_id (repeatable),
// tag (repeatable), sort=name|recent, limit, offset.
func (h *PresetHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := preset.ListFilter{
		Search: q.Get("search"),
		Sort:   preset.SortOrder(q.Get("sort")),
	}
	for _, raw := range q["slot_id"] {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_id")
			return
		}
		filter.SlotIDs = append(filter.SlotIDs, id)
	}
	filter.Tags = q["tag"]
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		filter.Limit = n
	}
	if raw := q.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		filter.Offset = n
	}

	res, err := h.svc.List(r.Context(), filter)
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	tags, err := h.svc.KnownTags(r.Context(), 25)
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	items := make([]presetResponse, len(res.Items))
	for i := range res.Items {
		items[i] = toPresetResponse(&res.Items[i])
	}
	known := make([]knownTagItem, len(tags))
	for i, t := range tags {
		known[i] = knownTagItem{Tag: t.Tag, Count: t.Count}
	}
	writeJSON(w, http.StatusOK, presetListResponse{Items: items, Total: res.Total, KnownTags: known})
}

// KnownTags handles GET /api/presets/known-tags.
func (h *PresetHandler) KnownTags(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		if n > 0 {
			limit = n
		}
	}
	tags, err := h.svc.KnownTags(r.Context(), limit)
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	items := make([]knownTagItem, len(tags))
	for i, t := range tags {
		items[i] = knownTagItem{Tag: t.Tag, Count: t.Count}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Get handles GET /api/presets/{id}.
func (h *PresetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePresetID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPresetResponse(p))
}

type createPresetRequest struct {
	Name     string                   `json:"name"`
	PlateIDs []int64                  `json:"plate_ids"`
	Plates   []updatePresetPlateInput `json:"plates"`
	Tags     []string                 `json:"tags,omitempty"`
}

// Create handles POST /api/presets. The body holds a name plus either
// plate_ids[] (copy planner plates) or plates[] (compose from scratch).
func (h *PresetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createPresetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if len(req.PlateIDs) > 0 && len(req.Plates) > 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	var (
		p   *preset.Preset
		err error
	)
	if len(req.Plates) > 0 {
		p, err = h.svc.Create(r.Context(), preset.CreateInput{
			Name:   req.Name,
			Tags:   req.Tags,
			Plates: toPresetPlates(req.Plates),
		})
	} else {
		p, err = h.svc.CreateFromPlates(r.Context(), preset.CreateFromPlatesInput{
			Name:     req.Name,
			PlateIDs: req.PlateIDs,
			Tags:     req.Tags,
		})
	}
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toPresetResponse(p))
}

// Duplicate handles POST /api/presets/{id}/duplicate.
func (h *PresetHandler) Duplicate(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePresetID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Duplicate(r.Context(), id)
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusCreated, toPresetResponse(p))
}

type updatePresetPlateInput struct {
	SlotID     int64                        `json:"slot_id"`
	Components []updatePresetComponentInput `json:"components"`
}

type updatePresetComponentInput struct {
	FoodID   int64    `json:"food_id"`
	Portions *float64 `json:"portions,omitempty"`
	Amount   *float64 `json:"amount,omitempty"`
	Unit     *string  `json:"unit,omitempty"`
	Note     *string  `json:"note,omitempty"`
}

type updatePresetRequest struct {
	Name   *string                   `json:"name,omitempty"`
	Tags   *[]string                 `json:"tags,omitempty"`
	Plates *[]updatePresetPlateInput `json:"plates,omitempty"`
}

// Update handles PUT /api/presets/{id}. Full editor save.
func (h *PresetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePresetID(w, r)
	if !ok {
		return
	}
	var req updatePresetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	in := preset.UpdateInput{Name: req.Name, Tags: req.Tags}
	if req.Plates != nil {
		plates := toPresetPlates(*req.Plates)
		in.Plates = &plates
	}
	p, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPresetResponse(p))
}

func toPresetPlates(in []updatePresetPlateInput) []preset.Plate {
	plates := make([]preset.Plate, len(in))
	for i, pp := range in {
		comps := make([]preset.Component, len(pp.Components))
		for j, c := range pp.Components {
			comps[j] = preset.Component{
				FoodID:   c.FoodID,
				Portions: c.Portions,
				Amount:   c.Amount,
				Unit:     c.Unit,
				Note:     c.Note,
			}
		}
		plates[i] = preset.Plate{SlotID: pp.SlotID, Components: comps}
	}
	return plates
}

type patchPresetRequest struct {
	Name       *string  `json:"name,omitempty"`
	AddTags    []string `json:"add_tags,omitempty"`
	RemoveTags []string `json:"remove_tags,omitempty"`
}

// Patch handles PATCH /api/presets/{id}. Partial edit: rename + tag deltas.
func (h *PresetHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePresetID(w, r)
	if !ok {
		return
	}
	var req patchPresetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	p, err := h.svc.Patch(r.Context(), id, preset.PatchInput{
		Name:       req.Name,
		AddTags:    req.AddTags,
		RemoveTags: req.RemoveTags,
	})
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toPresetResponse(p))
}

// Delete handles DELETE /api/presets/{id}.
func (h *PresetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePresetID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parsePresetID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return 0, false
	}
	return id, true
}

// applyCellItem is one (date, slot) cell of an apply or copy-week response.
type applyCellItem struct {
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

// applyResponse is the result of an apply or copy week. Undo is the client's:
// delete created_plate_ids, POST removed to /api/plan/restore and DELETE
// each created_overrides cell's availability override.
type applyResponse struct {
	CreatedPlateIDs    []int64            `json:"created_plate_ids"`
	SkippedOccupied    []applyCellItem    `json:"skipped_occupied"`
	SkippedUnavailable []applyCellItem    `json:"skipped_unavailable"`
	SkippedNoSlot      []applyCellItem    `json:"skipped_no_slot"`
	Removed            plate.PlanSnapshot `json:"removed"`
	CreatedOverrides   []applyCellItem    `json:"created_overrides"`
	// SkippedOverrides are copy week's "none" overrides that failed to copy.
	SkippedOverrides []applyCellItem `json:"skipped_overrides"`
}

func toApplyCells(cells []plate.CellKey) []applyCellItem {
	out := make([]applyCellItem, len(cells))
	for i, c := range cells {
		out[i] = applyCellItem{Date: c.Date.Format(time.DateOnly), SlotID: c.SlotID}
	}
	return out
}

func toApplyResponse(r *preset.ApplyResult) applyResponse {
	removed := r.Removed
	if removed.Plates == nil {
		removed.Plates = []plate.SnapshotPlate{}
	}
	if removed.Sessions == nil {
		removed.Sessions = []plate.SnapshotSession{}
	}
	return applyResponse{
		CreatedPlateIDs:    append([]int64{}, r.CreatedPlateIDs...),
		SkippedOccupied:    toApplyCells(r.SkippedOccupied),
		SkippedUnavailable: toApplyCells(r.SkippedUnavailable),
		SkippedNoSlot:      toApplyCells(r.SkippedNoSlot),
		Removed:            removed,
		CreatedOverrides:   toApplyCells(r.CreatedOverrides),
		SkippedOverrides:   toApplyCells(r.SkippedOverrides),
	}
}

type applyPresetRequest struct {
	TargetDate    string  `json:"target_date"`
	OnConflict    string  `json:"on_conflict,omitempty"`
	SlotIDsFilter []int64 `json:"slot_ids_filter,omitempty"`
}

// Apply handles POST /api/presets/{id}/apply.
func (h *PresetHandler) Apply(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePresetID(w, r)
	if !ok {
		return
	}
	var req applyPresetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	target, err := time.Parse("2006-01-02", req.TargetDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_date")
		return
	}
	result, err := h.svc.Apply(r.Context(), id, preset.ApplyRequest{
		TargetDate:    target,
		OnConflict:    preset.ConflictMode(req.OnConflict),
		SlotIDsFilter: req.SlotIDsFilter,
	})
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toApplyResponse(result))
}

type copyWeekRequest struct {
	SourceStart string `json:"source_start"`
	TargetStart string `json:"target_start"`
	OnConflict  string `json:"on_conflict,omitempty"`
}

// CopyWeek handles POST /api/presets/copy-week.
func (h *PresetHandler) CopyWeek(w http.ResponseWriter, r *http.Request) {
	var req copyWeekRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	src, err := time.Parse("2006-01-02", req.SourceStart)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_date")
		return
	}
	tgt, err := time.Parse("2006-01-02", req.TargetStart)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_date")
		return
	}
	result, err := h.svc.CopyWeek(r.Context(), preset.CopyWeekRequest{
		SourceStart: src,
		TargetStart: tgt,
		OnConflict:  preset.ConflictMode(req.OnConflict),
	})
	if err != nil {
		status, key := presetError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toApplyResponse(result))
}
