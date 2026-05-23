package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type stubPresetService struct {
	createFn    func(ctx context.Context, in preset.CreateFromPlatesInput) (*preset.Preset, error)
	getFn       func(ctx context.Context, id int64) (*preset.Preset, error)
	listFn      func(ctx context.Context, f preset.ListFilter) (*preset.ListResult, error)
	updateFn    func(ctx context.Context, id int64, in preset.UpdateInput) (*preset.Preset, error)
	patchFn     func(ctx context.Context, id int64, in preset.PatchInput) (*preset.Preset, error)
	deleteFn    func(ctx context.Context, id int64) error
	duplicateFn func(ctx context.Context, id int64) (*preset.Preset, error)
	knownFn     func(ctx context.Context, limit int) ([]preset.TagUsage, error)
	applyFn     func(ctx context.Context, id int64, req preset.ApplyRequest) (*preset.ApplyResult, error)
	copyWeekFn  func(ctx context.Context, req preset.CopyWeekRequest) (*preset.ApplyResult, error)
	undoFn      func(ctx context.Context, snap preset.ApplySnapshot) error
}

func (s *stubPresetService) CreateFromPlates(ctx context.Context, in preset.CreateFromPlatesInput) (*preset.Preset, error) {
	if s.createFn != nil {
		return s.createFn(ctx, in)
	}
	return &preset.Preset{ID: 1, Name: in.Name, Tags: []string{}, Plates: []preset.Plate{}}, nil
}

func (s *stubPresetService) Get(ctx context.Context, id int64) (*preset.Preset, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return &preset.Preset{ID: id, Name: "x", Tags: []string{}, Plates: []preset.Plate{}}, nil
}

func (s *stubPresetService) List(ctx context.Context, f preset.ListFilter) (*preset.ListResult, error) {
	if s.listFn != nil {
		return s.listFn(ctx, f)
	}
	return &preset.ListResult{Items: nil, Total: 0}, nil
}

func (s *stubPresetService) UpdateName(ctx context.Context, id int64, name string) (*preset.Preset, error) {
	return &preset.Preset{ID: id, Name: name, Tags: []string{}}, nil
}

func (s *stubPresetService) Update(ctx context.Context, id int64, in preset.UpdateInput) (*preset.Preset, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, id, in)
	}
	return &preset.Preset{ID: id, Tags: []string{}}, nil
}

func (s *stubPresetService) Patch(ctx context.Context, id int64, in preset.PatchInput) (*preset.Preset, error) {
	if s.patchFn != nil {
		return s.patchFn(ctx, id, in)
	}
	return &preset.Preset{ID: id, Tags: []string{}}, nil
}

func (s *stubPresetService) Delete(ctx context.Context, id int64) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func (s *stubPresetService) Duplicate(ctx context.Context, id int64) (*preset.Preset, error) {
	if s.duplicateFn != nil {
		return s.duplicateFn(ctx, id)
	}
	return &preset.Preset{ID: id + 1, Name: "x (copy)", Tags: []string{}}, nil
}

func (s *stubPresetService) KnownTags(ctx context.Context, limit int) ([]preset.TagUsage, error) {
	if s.knownFn != nil {
		return s.knownFn(ctx, limit)
	}
	return nil, nil
}

func (s *stubPresetService) Apply(ctx context.Context, id int64, req preset.ApplyRequest) (*preset.ApplyResult, error) {
	if s.applyFn != nil {
		return s.applyFn(ctx, id, req)
	}
	return &preset.ApplyResult{}, nil
}

func (s *stubPresetService) CopyWeek(ctx context.Context, req preset.CopyWeekRequest) (*preset.ApplyResult, error) {
	if s.copyWeekFn != nil {
		return s.copyWeekFn(ctx, req)
	}
	return &preset.ApplyResult{}, nil
}

func (s *stubPresetService) UndoApply(ctx context.Context, snap preset.ApplySnapshot) error {
	if s.undoFn != nil {
		return s.undoFn(ctx, snap)
	}
	return nil
}

func newPresetRouter(svc *stubPresetService) http.Handler {
	h := handlers.NewPresetHandlerFromInterfaces(svc)
	r := chi.NewRouter()
	r.Route("/api/presets", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/known-tags", h.KnownTags)
		r.Post("/", h.Create)
		r.Post("/copy-week", h.CopyWeek)
		r.Post("/undo-apply", h.UndoApply)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.Update)
			r.Patch("/", h.Patch)
			r.Delete("/", h.Delete)
			r.Post("/duplicate", h.Duplicate)
			r.Post("/apply", h.Apply)
		})
	})
	return r
}

func TestPresetHandler_Create_Validates(t *testing.T) {
	svc := &stubPresetService{
		createFn: func(_ context.Context, in preset.CreateFromPlatesInput) (*preset.Preset, error) {
			if in.Name == "" {
				return nil, domain.ErrInvalidInput
			}
			return &preset.Preset{ID: 1, Name: in.Name, Tags: []string{}}, nil
		},
	}
	r := newPresetRouter(svc)

	// Missing name → 400.
	req := httptest.NewRequest(http.MethodPost, "/api/presets",
		strings.NewReader(`{"plate_ids":[1]}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Happy path → 201.
	req = httptest.NewRequest(http.MethodPost, "/api/presets",
		strings.NewReader(`{"name":"Test","plate_ids":[1]}`))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "Test", got["name"])
}

func TestPresetHandler_Create_MalformedJSON(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	req := httptest.NewRequest(http.MethodPost, "/api/presets",
		bytes.NewBufferString(`{"name":"`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPresetHandler_Get_NonNumericID(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	req := httptest.NewRequest(http.MethodGet, "/api/presets/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPresetHandler_List_QueryParams(t *testing.T) {
	captured := preset.ListFilter{}
	svc := &stubPresetService{
		listFn: func(_ context.Context, f preset.ListFilter) (*preset.ListResult, error) {
			captured = f
			return &preset.ListResult{Items: []preset.Preset{{ID: 1, Name: "a", Tags: []string{}}}, Total: 1}, nil
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/presets?search=foo&slot_id=2&slot_id=3&tag=quick&tag=vegan&sort=recent&limit=10&offset=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "foo", captured.Search)
	assert.Equal(t, []int64{2, 3}, captured.SlotIDs)
	assert.Equal(t, []string{"quick", "vegan"}, captured.Tags)
	assert.Equal(t, preset.SortRecent, captured.Sort)
	assert.Equal(t, 10, captured.Limit)
	assert.Equal(t, 5, captured.Offset)
}

func TestPresetHandler_List_InvalidSlotID(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	req := httptest.NewRequest(http.MethodGet, "/api/presets?slot_id=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPresetHandler_Delete_NotFound(t *testing.T) {
	svc := &stubPresetService{
		deleteFn: func(_ context.Context, _ int64) error {
			return domain.ErrNotFound
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/presets/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPresetHandler_Patch(t *testing.T) {
	captured := preset.PatchInput{}
	svc := &stubPresetService{
		patchFn: func(_ context.Context, id int64, in preset.PatchInput) (*preset.Preset, error) {
			captured = in
			return &preset.Preset{ID: id, Tags: []string{"quick"}}, nil
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPatch, "/api/presets/1",
		strings.NewReader(`{"add_tags":["quick"],"remove_tags":["slow"]}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"quick"}, captured.AddTags)
	assert.Equal(t, []string{"slow"}, captured.RemoveTags)
}

// --- Apply handler ---

func TestPresetHandler_Apply_HappyPath(t *testing.T) {
	target := mustDate(t, "2026-05-20")
	svc := &stubPresetService{
		applyFn: func(_ context.Context, id int64, req preset.ApplyRequest) (*preset.ApplyResult, error) {
			require.Equal(t, int64(1), id)
			require.True(t, req.TargetDate.Equal(target))
			return &preset.ApplyResult{
				Created: []plate.Plate{{ID: 7, Date: target, SlotID: 3}},
				Snapshot: preset.ApplySnapshot{
					CreatedPlateIDs: []int64{7},
				},
			}, nil
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/presets/1/apply",
		strings.NewReader(`{"target_date":"2026-05-20"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	created, ok := got["created"].([]any)
	require.True(t, ok)
	require.Len(t, created, 1)
	first := created[0].(map[string]any)
	assert.Equal(t, "2026-05-20", first["date"])

	snap, ok := got["snapshot"].(map[string]any)
	require.True(t, ok)
	ids, ok := snap["created_plate_ids"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, ids)

	// Empty arrays must be `[]`, not `null`.
	body := w.Body.String()
	assert.Contains(t, body, `"replaced":[]`)
	assert.Contains(t, body, `"skipped_occupied":[]`)
	assert.Contains(t, body, `"skipped_no_slot":[]`)
	assert.Contains(t, body, `"replaced_plates":[]`)
}

func TestPresetHandler_Apply_InvalidDate(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	req := httptest.NewRequest(http.MethodPost, "/api/presets/1/apply",
		strings.NewReader(`{"target_date":"not-a-date"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPresetHandler_Apply_NotFound(t *testing.T) {
	svc := &stubPresetService{
		applyFn: func(_ context.Context, _ int64, _ preset.ApplyRequest) (*preset.ApplyResult, error) {
			return nil, domain.ErrNotFound
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/presets/1/apply",
		strings.NewReader(`{"target_date":"2026-05-20"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPresetHandler_Apply_NonNumericID(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	req := httptest.NewRequest(http.MethodPost, "/api/presets/abc/apply",
		strings.NewReader(`{"target_date":"2026-05-20"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- CopyWeek handler ---

func TestPresetHandler_CopyWeek_HappyPath(t *testing.T) {
	target := mustDate(t, "2026-05-19")
	svc := &stubPresetService{
		copyWeekFn: func(_ context.Context, req preset.CopyWeekRequest) (*preset.ApplyResult, error) {
			require.True(t, req.SourceStart.Equal(mustDate(t, "2026-05-12")))
			require.True(t, req.TargetStart.Equal(target))
			return &preset.ApplyResult{
				Created: []plate.Plate{
					{ID: 7, Date: target, SlotID: 1},
					{ID: 8, Date: target.AddDate(0, 0, 1), SlotID: 1},
				},
			}, nil
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/presets/copy-week",
		strings.NewReader(`{"source_start":"2026-05-12","target_start":"2026-05-19"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	created := got["created"].([]any)
	require.Len(t, created, 2)
	assert.Equal(t, "2026-05-19", created[0].(map[string]any)["date"])
	assert.Equal(t, "2026-05-20", created[1].(map[string]any)["date"])
}

func TestPresetHandler_CopyWeek_InvalidSourceDate(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	req := httptest.NewRequest(http.MethodPost, "/api/presets/copy-week",
		strings.NewReader(`{"source_start":"bad","target_start":"2026-05-19"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPresetHandler_CopyWeek_InvalidTargetDate(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	req := httptest.NewRequest(http.MethodPost, "/api/presets/copy-week",
		strings.NewReader(`{"source_start":"2026-05-12","target_start":"bad"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- UndoApply handler ---

func TestPresetHandler_UndoApply_HappyPath(t *testing.T) {
	called := false
	svc := &stubPresetService{
		undoFn: func(_ context.Context, snap preset.ApplySnapshot) error {
			called = true
			assert.Equal(t, []int64{7, 8}, snap.CreatedPlateIDs)
			assert.Empty(t, snap.ReplacedPlates)
			return nil
		},
	}
	r := newPresetRouter(svc)
	body := `{"snapshot":{"created_plate_ids":[7,8],"replaced_plates":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/presets/undo-apply",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
	assert.True(t, called)
}

func TestPresetHandler_UndoApply_InvalidDateInSnapshot(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	body := `{"snapshot":{"created_plate_ids":[],"replaced_plates":[{"date":"not-a-date","slot_id":1,"skipped":false,"components":[]}]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/presets/undo-apply",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Update handler ---

func TestPresetHandler_Update_MapsAllFields(t *testing.T) {
	var captured preset.UpdateInput
	svc := &stubPresetService{
		updateFn: func(_ context.Context, id int64, in preset.UpdateInput) (*preset.Preset, error) {
			captured = in
			return &preset.Preset{ID: id, Name: "New Name", Tags: []string{"quick"}}, nil
		},
	}
	r := newPresetRouter(svc)
	body := `{"name":"New Name","tags":["quick"],"plates":[{"slot_id":2,"components":[{"food_id":5,"amount":200,"unit":"g"}]}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/presets/1",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.NotNil(t, captured.Name)
	assert.Equal(t, "New Name", *captured.Name)
	require.NotNil(t, captured.Tags)
	assert.Equal(t, []string{"quick"}, *captured.Tags)
	require.NotNil(t, captured.Plates)
	require.Len(t, *captured.Plates, 1)
	pl := (*captured.Plates)[0]
	assert.Equal(t, int64(2), pl.SlotID)
	require.Len(t, pl.Components, 1)
	c := pl.Components[0]
	assert.Equal(t, int64(5), c.FoodID)
	require.NotNil(t, c.Amount)
	assert.Equal(t, 200.0, *c.Amount)
	require.NotNil(t, c.Unit)
	assert.Equal(t, "g", *c.Unit)
}

func TestPresetHandler_Update_EmptyName(t *testing.T) {
	svc := &stubPresetService{
		updateFn: func(_ context.Context, _ int64, _ preset.UpdateInput) (*preset.Preset, error) {
			return nil, domain.ErrInvalidInput
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/presets/1",
		strings.NewReader(`{"name":""}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Duplicate handler ---

func TestPresetHandler_Duplicate_NotFound(t *testing.T) {
	svc := &stubPresetService{
		duplicateFn: func(_ context.Context, _ int64) (*preset.Preset, error) {
			return nil, domain.ErrNotFound
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/presets/1/duplicate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return tt
}
