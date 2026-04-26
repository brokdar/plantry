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
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// ── stub ──────────────────────────────────────────────────────────────────────

// stubPlateService implements the platesService interface used by PlateHandler.
// Each method has a configurable func field; unset fields return zero values.
type stubPlateService struct {
	getRangeFn func(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
	createFn   func(ctx context.Context, p *plate.Plate) error
	getFn      func(ctx context.Context, id int64) (*plate.Plate, error)
	updateFn   func(ctx context.Context, p *plate.Plate) error
}

func (s *stubPlateService) Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	if s.getRangeFn != nil {
		return s.getRangeFn(ctx, from, to)
	}
	return nil, nil
}

func (s *stubPlateService) Day(ctx context.Context, d time.Time) ([]plate.Plate, error) {
	if s.getRangeFn != nil {
		return s.getRangeFn(ctx, d, d)
	}
	return nil, nil
}

func (s *stubPlateService) Create(ctx context.Context, p *plate.Plate) error {
	if s.createFn != nil {
		return s.createFn(ctx, p)
	}
	p.ID = 1
	return nil
}

func (s *stubPlateService) Get(ctx context.Context, id int64) (*plate.Plate, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	d, _ := time.Parse("2006-01-02", "2025-03-10")
	return &plate.Plate{ID: id, SlotID: 1, Day: 0, Date: d}, nil
}

func (s *stubPlateService) Update(ctx context.Context, p *plate.Plate) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, p)
	}
	return nil
}

func (s *stubPlateService) Delete(_ context.Context, _ int64) error { return nil }
func (s *stubPlateService) AddComponent(_ context.Context, _, _ int64, _ float64) (*plate.PlateComponent, error) {
	return &plate.PlateComponent{}, nil
}

func (s *stubPlateService) SwapComponent(_ context.Context, _ int64, _ int64, _ *float64) (*plate.PlateComponent, error) {
	return &plate.PlateComponent{}, nil
}

func (s *stubPlateService) UpdateComponentPortions(_ context.Context, _ int64, _ float64) (*plate.PlateComponent, error) {
	return &plate.PlateComponent{}, nil
}
func (s *stubPlateService) RemoveComponent(_ context.Context, _ int64) error { return nil }
func (s *stubPlateService) SetSkipped(_ context.Context, _ int64, _ bool, _ *string) (*plate.Plate, error) {
	return &plate.Plate{}, nil
}

// ── router helper ─────────────────────────────────────────────────────────────

func newPlateRouter(stub *stubPlateService) http.Handler {
	h := handlers.NewPlateHandlerFromService(stub)
	r := chi.NewRouter()
	r.Route("/api/plates", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/by-date/{date}", h.Day)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.Update)
			r.Delete("/", h.Delete)
		})
	})
	return r
}

// ── List tests ────────────────────────────────────────────────────────────────

func TestPlatesHandler_List_200(t *testing.T) {
	d, _ := time.Parse("2006-01-02", "2025-03-10")
	stub := &stubPlateService{
		getRangeFn: func(_ context.Context, from, to time.Time) ([]plate.Plate, error) {
			return []plate.Plate{
				{ID: 1, SlotID: 2, Day: 0, Date: d},
			}, nil
		},
	}
	r := newPlateRouter(stub)

	req := httptest.NewRequest(http.MethodGet, "/api/plates?from=2025-03-10&to=2025-03-16", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	plates, ok := body["plates"].([]any)
	require.True(t, ok)
	assert.Len(t, plates, 1)
	// Verify date field is present
	first := plates[0].(map[string]any)
	assert.Equal(t, "2025-03-10", first["date"])
}

func TestPlatesHandler_List_400_MissingFrom(t *testing.T) {
	r := newPlateRouter(&stubPlateService{})
	req := httptest.NewRequest(http.MethodGet, "/api/plates?to=2025-03-16", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPlatesHandler_List_400_MalformedDate(t *testing.T) {
	r := newPlateRouter(&stubPlateService{})
	req := httptest.NewRequest(http.MethodGet, "/api/plates?from=not-a-date&to=2025-03-16", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPlatesHandler_List_400_FromAfterTo(t *testing.T) {
	r := newPlateRouter(&stubPlateService{})
	req := httptest.NewRequest(http.MethodGet, "/api/plates?from=2025-03-17&to=2025-03-10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── Day tests ─────────────────────────────────────────────────────────────────

func TestPlatesHandler_Day_200(t *testing.T) {
	d, _ := time.Parse("2006-01-02", "2025-03-10")
	stub := &stubPlateService{
		getRangeFn: func(_ context.Context, from, to time.Time) ([]plate.Plate, error) {
			return []plate.Plate{{ID: 7, SlotID: 1, Day: 0, Date: d}}, nil
		},
	}
	r := newPlateRouter(stub)

	req := httptest.NewRequest(http.MethodGet, "/api/plates/by-date/2025-03-10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	plates, ok := body["plates"].([]any)
	require.True(t, ok)
	assert.Len(t, plates, 1)
}

func TestPlatesHandler_Day_400_BadDate(t *testing.T) {
	r := newPlateRouter(&stubPlateService{})
	req := httptest.NewRequest(http.MethodGet, "/api/plates/by-date/not-valid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestPlatesHandler_Create_201(t *testing.T) {
	stub := &stubPlateService{
		createFn: func(_ context.Context, p *plate.Plate) error {
			p.ID = 42
			return nil
		},
	}
	r := newPlateRouter(stub)

	body := `{"date":"2025-03-10","slot_id":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/plates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(42), resp["id"])
	assert.Equal(t, "2025-03-10", resp["date"])
}

func TestPlatesHandler_Create_400_MissingDate(t *testing.T) {
	r := newPlateRouter(&stubPlateService{})
	body := `{"slot_id":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/plates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPlatesHandler_Create_400_BadSlotID(t *testing.T) {
	r := newPlateRouter(&stubPlateService{})
	body := `{"date":"2025-03-10","slot_id":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/plates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── Update tests ──────────────────────────────────────────────────────────────

func TestPlatesHandler_Update_AcceptsDate(t *testing.T) {
	var capturedDate time.Time
	stub := &stubPlateService{
		getFn: func(_ context.Context, id int64) (*plate.Plate, error) {
			d, _ := time.Parse("2006-01-02", "2025-03-10")
			return &plate.Plate{ID: id, SlotID: 1, Day: 0, Date: d}, nil
		},
		updateFn: func(_ context.Context, p *plate.Plate) error {
			capturedDate = p.Date
			return nil
		},
	}
	r := newPlateRouter(stub)

	body := `{"date":"2025-03-12"}`
	req := httptest.NewRequest(http.MethodPut, "/api/plates/5", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "2025-03-12", capturedDate.Format("2006-01-02"))

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "2025-03-12", resp["date"])
}
