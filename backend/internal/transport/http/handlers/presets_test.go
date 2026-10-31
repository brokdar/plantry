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
	composeFn   func(ctx context.Context, in preset.CreateInput) (*preset.Preset, error)
	getFn       func(ctx context.Context, id int64) (*preset.Preset, error)
	listFn      func(ctx context.Context, f preset.ListFilter) (*preset.ListResult, error)
	updateFn    func(ctx context.Context, id int64, in preset.UpdateInput) (*preset.Preset, error)
	patchFn     func(ctx context.Context, id int64, in preset.PatchInput) (*preset.Preset, error)
	deleteFn    func(ctx context.Context, id int64) error
	duplicateFn func(ctx context.Context, id int64) (*preset.Preset, error)
	knownFn     func(ctx context.Context, limit int) ([]preset.TagUsage, error)
	applyFn     func(ctx context.Context, id int64, req preset.ApplyRequest) (*preset.ApplyResult, error)
	copyWeekFn  func(ctx context.Context, req preset.CopyWeekRequest) (*preset.ApplyResult, error)
}

func (s *stubPresetService) CreateFromPlates(ctx context.Context, in preset.CreateFromPlatesInput) (*preset.Preset, error) {
	if s.createFn != nil {
		return s.createFn(ctx, in)
	}
	return &preset.Preset{ID: 1, Name: in.Name, Tags: []string{}, Plates: []preset.Plate{}}, nil
}

func (s *stubPresetService) Create(ctx context.Context, in preset.CreateInput) (*preset.Preset, error) {
	if s.composeFn != nil {
		return s.composeFn(ctx, in)
	}
	return &preset.Preset{ID: 1, Name: in.Name, Tags: []string{}, Plates: in.Plates}, nil
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

func newPresetRouter(svc *stubPresetService) http.Handler {
	h := handlers.NewPresetHandlerFromInterfaces(svc)
	r := chi.NewRouter()
	r.Route("/api/presets", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/known-tags", h.KnownTags)
		r.Post("/", h.Create)
		r.Post("/copy-week", h.CopyWeek)
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

func TestPresetHandler_Create_FromPlates(t *testing.T) {
	var got preset.CreateInput
	svc := &stubPresetService{
		composeFn: func(_ context.Context, in preset.CreateInput) (*preset.Preset, error) {
			got = in
			return &preset.Preset{ID: 4, Name: in.Name, Tags: []string{}, Plates: in.Plates}, nil
		},
		createFn: func(context.Context, preset.CreateFromPlatesInput) (*preset.Preset, error) {
			t.Fatal("plate_ids path must not run")
			return nil, nil
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/presets", strings.NewReader(
		`{"name":"Oats","tags":["quick"],"plates":[{"slot_id":2,"components":[`+
			`{"food_id":7,"portions":1.5},{"food_id":9,"amount":80,"unit":"g"}]}]}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "Oats", got.Name)
	assert.Equal(t, []string{"quick"}, got.Tags)
	require.Len(t, got.Plates, 1)
	assert.Equal(t, int64(2), got.Plates[0].SlotID)
	require.Len(t, got.Plates[0].Components, 2)
	assert.InDelta(t, 1.5, *got.Plates[0].Components[0].Portions, 1e-9)
	assert.Equal(t, "g", *got.Plates[0].Components[1].Unit)
}

func TestPresetHandler_Create_RejectsPlatesAndPlateIDs(t *testing.T) {
	r := newPresetRouter(&stubPresetService{})
	req := httptest.NewRequest(http.MethodPost, "/api/presets", strings.NewReader(
		`{"name":"X","plate_ids":[1],"plates":[{"slot_id":2,"components":[{"food_id":7,"portions":1}]}]}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
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
			return &preset.ApplyResult{CreatedPlateIDs: []int64{7}}, nil
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/presets/1/apply",
		strings.NewReader(`{"target_date":"2026-05-20"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Empty arrays must be `[]`, not `null`.
	assert.JSONEq(t, `{
		"created_plate_ids": [7],
		"skipped_occupied": [], "skipped_unavailable": [], "skipped_no_slot": [],
		"removed": {"plates": [], "sessions": []},
		"created_overrides": []
	}`, w.Body.String())
}

func TestPresetHandler_CopyWeek_AvailabilityJSON(t *testing.T) {
	target := mustDate(t, "2026-05-19")
	note := "kept"
	one := 1.0
	svc := &stubPresetService{
		copyWeekFn: func(_ context.Context, _ preset.CopyWeekRequest) (*preset.ApplyResult, error) {
			return &preset.ApplyResult{
				CreatedPlateIDs:    []int64{9},
				SkippedUnavailable: []plate.CellKey{{Date: target, SlotID: 2}},
				CreatedOverrides:   []plate.CellKey{{Date: target.AddDate(0, 0, 1), SlotID: 3}},
				Removed: plate.PlanSnapshot{
					Plates: []plate.SnapshotPlate{{Date: "2026-05-19", SlotID: 1, Eaters: 2, Note: &note, Components: []plate.SnapshotComponent{
						{FoodID: 4, Portions: &one, Session: &plate.SessionLink{Ref: "s1"}},
					}}},
					Sessions: []plate.SnapshotSession{{Ref: "s1", FoodID: 4, CookDate: "2026-05-19", CookSlotID: 1, Portions: 2}},
				},
			}, nil
		},
	}
	r := newPresetRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/presets/copy-week",
		strings.NewReader(`{"source_start":"2026-05-12","target_start":"2026-05-19"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.JSONEq(t, `{
		"created_plate_ids": [9],
		"skipped_occupied": [],
		"skipped_unavailable": [{"date": "2026-05-19", "slot_id": 2}],
		"skipped_no_slot": [],
		"removed": {
			"plates": [{"date": "2026-05-19", "slot_id": 1, "eaters": 2, "note": "kept",
				"components": [{"food_id": 4, "portions": 1, "sort_order": 0, "session": {"ref": "s1"}}]}],
			"sessions": [{"ref": "s1", "food_id": 4, "cook_date": "2026-05-19", "cook_slot_id": 1,
				"portions": 2, "frozen_portions": 0, "cooked_at": null, "note": null}]
		},
		"created_overrides": [{"date": "2026-05-20", "slot_id": 3}]
	}`, w.Body.String())
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
			return &preset.ApplyResult{CreatedPlateIDs: []int64{7, 8}}, nil
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
	assert.Equal(t, []any{float64(7), float64(8)}, got["created_plate_ids"])
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
