package handlers_test

import (
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

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubTemplateService struct {
	listFn           func(ctx context.Context) ([]template.Template, error)
	listByScopeFn    func(ctx context.Context, scope template.Scope) ([]template.Template, error)
	createFn         func(ctx context.Context, name string, scope template.Scope, fromPlateID *int64, entries []template.TemplateEntry) (*template.Template, error)
	getFn            func(ctx context.Context, id int64) (*template.Template, error)
	updateNameFn     func(ctx context.Context, id int64, name string) (*template.Template, error)
	deleteFn         func(ctx context.Context, id int64) error
	applyFn          func(ctx context.Context, templateID int64, payload template.ApplyPayload) (*template.ApplyResult, error)
	saveAsTemplateFn func(ctx context.Context, name string, plates []plate.Plate, anchorDate time.Time) (*template.Template, error)
}

func (s *stubTemplateService) List(ctx context.Context) ([]template.Template, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return nil, nil
}

func (s *stubTemplateService) ListByScope(ctx context.Context, scope template.Scope) ([]template.Template, error) {
	if s.listByScopeFn != nil {
		return s.listByScopeFn(ctx, scope)
	}
	return nil, nil
}

func (s *stubTemplateService) Create(ctx context.Context, name string, scope template.Scope, fromPlateID *int64, entries []template.TemplateEntry) (*template.Template, error) {
	if s.createFn != nil {
		return s.createFn(ctx, name, scope, fromPlateID, entries)
	}
	return &template.Template{ID: 1, Name: name, Scope: scope}, nil
}

func (s *stubTemplateService) Get(ctx context.Context, id int64) (*template.Template, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return &template.Template{ID: id, Name: "test", Scope: template.ScopeSlot}, nil
}

func (s *stubTemplateService) UpdateName(ctx context.Context, id int64, name string) (*template.Template, error) {
	if s.updateNameFn != nil {
		return s.updateNameFn(ctx, id, name)
	}
	return &template.Template{ID: id, Name: name}, nil
}

func (s *stubTemplateService) Delete(ctx context.Context, id int64) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func (s *stubTemplateService) Apply(ctx context.Context, templateID int64, payload template.ApplyPayload) (*template.ApplyResult, error) {
	if s.applyFn != nil {
		return s.applyFn(ctx, templateID, payload)
	}
	d := time.Time{}
	if payload.Date != nil {
		d = *payload.Date
	} else if payload.StartDate != nil {
		d = *payload.StartDate
	}
	slot := int64(0)
	if payload.SlotID != nil {
		slot = *payload.SlotID
	}
	return &template.ApplyResult{Created: []plate.Plate{{ID: 1, Date: d, SlotID: slot}}}, nil
}

func (s *stubTemplateService) SaveAsTemplate(ctx context.Context, name string, plates []plate.Plate, anchorDate time.Time) (*template.Template, error) {
	if s.saveAsTemplateFn != nil {
		return s.saveAsTemplateFn(ctx, name, plates, anchorDate)
	}
	return &template.Template{ID: 2, Name: name, Scope: template.ScopeWeek}, nil
}

type stubRangeReader struct {
	rangeFn func(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

func (s *stubRangeReader) Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	if s.rangeFn != nil {
		return s.rangeFn(ctx, from, to)
	}
	return nil, nil
}

// ── router helpers ────────────────────────────────────────────────────────────

func newTemplateRouter(svc *stubTemplateService, rr *stubRangeReader) http.Handler {
	h := handlers.NewTemplateHandlerFromInterfaces(svc, rr)
	r := chi.NewRouter()
	r.Route("/api/templates", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.Update)
			r.Delete("/", h.Delete)
			r.Post("/apply", h.Apply)
		})
	})
	return r
}

// ── Apply tests ───────────────────────────────────────────────────────────────

func TestTemplatesHandler_Apply_Slot_LegacyPayload_200(t *testing.T) {
	svc := &stubTemplateService{
		getFn: func(_ context.Context, id int64) (*template.Template, error) {
			return &template.Template{ID: id, Scope: template.ScopeSlot}, nil
		},
		applyFn: func(_ context.Context, templateID int64, payload template.ApplyPayload) (*template.ApplyResult, error) {
			require.NotNil(t, payload.Date)
			require.NotNil(t, payload.SlotID)
			return &template.ApplyResult{Created: []plate.Plate{{ID: 10, Date: *payload.Date, SlotID: *payload.SlotID}}}, nil
		},
	}
	router := newTemplateRouter(svc, &stubRangeReader{})

	body := `{"start_date":"2026-04-25","slot_id":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates/5/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	plates := resp["plates"].([]any)
	assert.Len(t, plates, 1)
	first := plates[0].(map[string]any)
	assert.Equal(t, "2026-04-25", first["date"])
}

func TestTemplatesHandler_Apply_Day_ConflictSkip_200(t *testing.T) {
	d := mustTemplateDate("2026-04-25")
	svc := &stubTemplateService{
		getFn: func(_ context.Context, id int64) (*template.Template, error) {
			return &template.Template{ID: id, Scope: template.ScopeDay}, nil
		},
		applyFn: func(_ context.Context, _ int64, payload template.ApplyPayload) (*template.ApplyResult, error) {
			require.NotNil(t, payload.Date)
			assert.Equal(t, template.ConflictSkip, payload.Conflict)
			return &template.ApplyResult{
				Created: []plate.Plate{{ID: 1, Date: d, SlotID: 2}},
				Skipped: []template.SkippedConflict{{Date: d, SlotID: 1}},
			}, nil
		},
	}
	router := newTemplateRouter(svc, &stubRangeReader{})

	body := `{"date":"2026-04-25","conflict":"skip"}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates/7/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp["plates"].([]any), 1)
	skipped := resp["skipped"].([]any)
	require.Len(t, skipped, 1)
	first := skipped[0].(map[string]any)
	assert.Equal(t, "2026-04-25", first["date"])
	assert.Equal(t, float64(1), first["slot_id"])
}

func TestTemplatesHandler_Apply_Week_200(t *testing.T) {
	svc := &stubTemplateService{
		getFn: func(_ context.Context, id int64) (*template.Template, error) {
			return &template.Template{ID: id, Scope: template.ScopeWeek}, nil
		},
		applyFn: func(_ context.Context, _ int64, payload template.ApplyPayload) (*template.ApplyResult, error) {
			require.NotNil(t, payload.StartDate)
			return &template.ApplyResult{Created: []plate.Plate{}}, nil
		},
	}
	router := newTemplateRouter(svc, &stubRangeReader{})

	body := `{"start_date":"2026-04-25","conflict":"overwrite"}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates/8/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestTemplatesHandler_Apply_400_BadDate(t *testing.T) {
	router := newTemplateRouter(&stubTemplateService{}, &stubRangeReader{})

	body := `{"start_date":"not-a-date","slot_id":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates/5/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "error.invalid_date", resp["message_key"])
}

func TestTemplatesHandler_Apply_400_MissingSlotID(t *testing.T) {
	router := newTemplateRouter(&stubTemplateService{}, &stubRangeReader{})

	body := `{"start_date":"2026-04-25"}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates/5/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTemplatesHandler_Apply_400_MissingDate(t *testing.T) {
	router := newTemplateRouter(&stubTemplateService{}, &stubRangeReader{})

	body := `{"slot_id":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates/5/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── List with scope filter ────────────────────────────────────────────────────

func TestTemplatesHandler_List_ScopeFilter(t *testing.T) {
	var captured template.Scope
	svc := &stubTemplateService{
		listByScopeFn: func(_ context.Context, scope template.Scope) ([]template.Template, error) {
			captured = scope
			return []template.Template{{ID: 1, Name: "X", Scope: scope}}, nil
		},
	}
	router := newTemplateRouter(svc, &stubRangeReader{})
	req := httptest.NewRequest(http.MethodGet, "/api/templates?scope=week", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, template.ScopeWeek, captured)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	items := resp["items"].([]any)
	require.Len(t, items, 1)
	first := items[0].(map[string]any)
	assert.Equal(t, "week", first["scope"])
}

func TestTemplatesHandler_List_ScopeFilter_Invalid(t *testing.T) {
	router := newTemplateRouter(&stubTemplateService{}, &stubRangeReader{})
	req := httptest.NewRequest(http.MethodGet, "/api/templates?scope=bogus", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestTemplatesHandler_Create_DefaultsToSlotScope(t *testing.T) {
	var capturedScope template.Scope
	svc := &stubTemplateService{
		createFn: func(_ context.Context, name string, scope template.Scope, _ *int64, _ []template.TemplateEntry) (*template.Template, error) {
			capturedScope = scope
			return &template.Template{ID: 1, Name: name, Scope: scope}, nil
		},
	}
	router := newTemplateRouter(svc, &stubRangeReader{})
	body := `{"name":"X","components":[{"food_id":1,"portions":1}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, template.ScopeSlot, capturedScope)
}

func TestTemplatesHandler_Create_Range_201(t *testing.T) {
	d0 := mustTemplateDate("2026-04-25")
	d1 := mustTemplateDate("2026-04-26")
	d2 := mustTemplateDate("2026-04-27")

	rr := &stubRangeReader{
		rangeFn: func(_ context.Context, from, to time.Time) ([]plate.Plate, error) {
			return []plate.Plate{
				{ID: 1, Date: d0, SlotID: 1, Components: []plate.PlateComponent{{FoodID: 10, Portions: 1}}},
				{ID: 2, Date: d1, SlotID: 1, Components: []plate.PlateComponent{{FoodID: 20, Portions: 1}}},
				{ID: 3, Date: d2, SlotID: 1, Components: []plate.PlateComponent{{FoodID: 30, Portions: 1}}},
			}, nil
		},
	}

	var capturedPlates []plate.Plate
	svc := &stubTemplateService{
		saveAsTemplateFn: func(_ context.Context, name string, plates []plate.Plate, _ time.Time) (*template.Template, error) {
			capturedPlates = plates
			return &template.Template{
				ID:    99,
				Name:  name,
				Scope: template.ScopeWeek,
				Entries: []template.TemplateEntry{
					{FoodID: 10, DayOffset: 0},
					{FoodID: 20, DayOffset: 1},
					{FoodID: 30, DayOffset: 2},
				},
			}, nil
		},
	}

	router := newTemplateRouter(svc, rr)
	body := `{"name":"My Pattern","from":"2026-04-25","to":"2026-04-27"}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Len(t, capturedPlates, 3)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(99), resp["id"])
	assert.Equal(t, "week", resp["scope"])

	comps := resp["components"].([]any)
	require.Len(t, comps, 3)
	offsets := []float64{0, 1, 2}
	for i, c := range comps {
		cm := c.(map[string]any)
		assert.Equal(t, offsets[i], cm["day_offset"], "component[%d].day_offset", i)
	}
}

func TestTemplatesHandler_Create_Range_400_BadDate(t *testing.T) {
	router := newTemplateRouter(&stubTemplateService{}, &stubRangeReader{})
	body := `{"name":"X","from":"bad","to":"2026-04-27"}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func mustTemplateDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
