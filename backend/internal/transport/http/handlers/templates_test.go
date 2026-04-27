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
	createFn         func(ctx context.Context, name string, fromPlateID *int64, components []template.TemplateComponent) (*template.Template, error)
	getFn            func(ctx context.Context, id int64) (*template.Template, error)
	updateNameFn     func(ctx context.Context, id int64, name string) (*template.Template, error)
	deleteFn         func(ctx context.Context, id int64) error
	applyFn          func(ctx context.Context, templateID int64, startDate time.Time, slotID int64) ([]plate.Plate, error)
	saveAsTemplateFn func(ctx context.Context, name string, plates []plate.Plate, anchorDate time.Time) (*template.Template, error)
}

func (s *stubTemplateService) List(ctx context.Context) ([]template.Template, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return nil, nil
}

func (s *stubTemplateService) Create(ctx context.Context, name string, fromPlateID *int64, components []template.TemplateComponent) (*template.Template, error) {
	if s.createFn != nil {
		return s.createFn(ctx, name, fromPlateID, components)
	}
	return &template.Template{ID: 1, Name: name}, nil
}

func (s *stubTemplateService) Get(ctx context.Context, id int64) (*template.Template, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return &template.Template{ID: id, Name: "test"}, nil
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

func (s *stubTemplateService) Apply(ctx context.Context, templateID int64, startDate time.Time, slotID int64) ([]plate.Plate, error) {
	if s.applyFn != nil {
		return s.applyFn(ctx, templateID, startDate, slotID)
	}
	return []plate.Plate{
		{ID: 1, Date: startDate, SlotID: slotID},
	}, nil
}

func (s *stubTemplateService) SaveAsTemplate(ctx context.Context, name string, plates []plate.Plate, anchorDate time.Time) (*template.Template, error) {
	if s.saveAsTemplateFn != nil {
		return s.saveAsTemplateFn(ctx, name, plates, anchorDate)
	}
	return &template.Template{ID: 2, Name: name}, nil
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

func TestTemplatesHandler_Apply_200(t *testing.T) {
	svc := &stubTemplateService{
		applyFn: func(_ context.Context, templateID int64, startDate time.Time, slotID int64) ([]plate.Plate, error) {
			return []plate.Plate{
				{ID: 10, Date: startDate, SlotID: slotID},
			}, nil
		},
	}
	rr := &stubRangeReader{}
	router := newTemplateRouter(svc, rr)

	body := `{"start_date":"2026-04-25","slot_id":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/templates/5/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	plates, ok := resp["plates"].([]any)
	require.True(t, ok)
	assert.Len(t, plates, 1)
	first := plates[0].(map[string]any)
	assert.Equal(t, "2026-04-25", first["date"])
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

// ── Create (range) tests ──────────────────────────────────────────────────────

func TestTemplatesHandler_Create_Range_201(t *testing.T) {
	// Three plates will come back from the range reader, template will be created.
	d0 := mustTemplateDate("2026-04-25")
	d1 := mustTemplateDate("2026-04-26")
	d2 := mustTemplateDate("2026-04-27")

	rr := &stubRangeReader{
		rangeFn: func(_ context.Context, from, to time.Time) ([]plate.Plate, error) {
			return []plate.Plate{
				{ID: 1, Date: d0, Components: []plate.PlateComponent{{FoodID: 10, Portions: 1}}},
				{ID: 2, Date: d1, Components: []plate.PlateComponent{{FoodID: 20, Portions: 1}}},
				{ID: 3, Date: d2, Components: []plate.PlateComponent{{FoodID: 30, Portions: 1}}},
			}, nil
		},
	}

	var capturedPlates []plate.Plate
	svc := &stubTemplateService{
		saveAsTemplateFn: func(_ context.Context, name string, plates []plate.Plate, anchor time.Time) (*template.Template, error) {
			capturedPlates = plates
			return &template.Template{
				ID:   99,
				Name: name,
				Components: []template.TemplateComponent{
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

	// Verify day_offset is included in component responses.
	comps, ok := resp["components"].([]any)
	require.True(t, ok)
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
