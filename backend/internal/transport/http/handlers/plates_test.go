package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// ── stub ──────────────────────────────────────────────────────────────────────

// stubPlateService implements the platesService interface used by PlateHandler.
// Each method has a configurable func field; unset fields return zero values.
type stubPlateService struct {
	createFn func(ctx context.Context, in plate.PlateInput) (*plate.Plate, error)
	getFn    func(ctx context.Context, id int64) (*plate.Plate, error)
	updateFn func(ctx context.Context, id int64, patch plate.PlatePatch) (*plate.Plate, error)
}

func (s *stubPlateService) Create(ctx context.Context, in plate.PlateInput) (*plate.Plate, error) {
	if s.createFn != nil {
		return s.createFn(ctx, in)
	}
	return &plate.Plate{ID: 1, Date: in.Date, SlotID: in.SlotID}, nil
}

func (s *stubPlateService) Get(ctx context.Context, id int64) (*plate.Plate, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	d, _ := time.Parse("2006-01-02", "2025-03-10")
	return &plate.Plate{ID: id, SlotID: 1, Date: d}, nil
}

func (s *stubPlateService) UpdateMeal(ctx context.Context, id int64, patch plate.PlatePatch) (plate.MealResult, error) {
	get := s.Get
	if s.updateFn != nil {
		get = func(ctx context.Context, id int64) (*plate.Plate, error) { return s.updateFn(ctx, id, patch) }
	}
	p, err := get(ctx, id)
	if err != nil {
		return plate.MealResult{}, err
	}
	return plate.MealResult{Plates: []plate.Plate{*p}}, nil
}

func (s *stubPlateService) MoveMeal(context.Context, int64, plate.CellKey, plate.Occupant) (plate.MealResult, error) {
	return plate.MealResult{}, nil
}

func (s *stubPlateService) ReplaceComponent(context.Context, int64, plate.Selector, int64, plate.Source, *plate.PlateComponent) (plate.MealResult, error) {
	return plate.MealResult{}, nil
}

func (s *stubPlateService) CopyMeal(context.Context, int64, []plate.CellKey, plate.CopyAs) (plate.MealResult, error) {
	return plate.MealResult{}, nil
}

func (s *stubPlateService) Recorded(ctx context.Context, fn func(ctx context.Context) error) (plate.Undo, error) {
	return plate.Undo{}, fn(ctx)
}

func (s *stubPlateService) RemoveMeals(context.Context, []int64) (plate.MealResult, error) {
	return plate.MealResult{
		Removed:     plate.PlanSnapshot{Plates: []plate.SnapshotPlate{}, Sessions: []plate.SnapshotSession{}},
		AlsoChanged: []plate.AlsoChanged{},
		Undo:        plate.Undo{FirstEventID: 7, LastEventID: 8, Cells: []plate.CellUndo{}},
	}, nil
}

func (s *stubPlateService) AddComponent(_ context.Context, id int64, _ plate.PlateComponent) (*plate.Plate, error) {
	return &plate.Plate{ID: id}, nil
}

func (s *stubPlateService) SwapComponent(_ context.Context, _ int64, _ int64, _ *plate.PlateComponent) (*plate.Plate, error) {
	return &plate.Plate{}, nil
}

func (s *stubPlateService) UpdateComponentQuantity(_ context.Context, _ int64, _ plate.PlateComponent) (*plate.Plate, error) {
	return &plate.Plate{}, nil
}

func (s *stubPlateService) RemoveComponent(_ context.Context, _ int64) (*plate.Plate, error) {
	return &plate.Plate{}, nil
}

func (s *stubPlateService) Confirm(_ context.Context, seed plate.RowSet) (plate.RowSet, error) {
	return seed, nil
}

func (s *stubPlateService) Unlock(_ context.Context, seed plate.RowSet) (plate.RowSet, error) {
	return seed, nil
}

func (s *stubPlateService) DryRun(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// ── router helper ─────────────────────────────────────────────────────────────

func newPlateRouter(stub *stubPlateService) http.Handler {
	h := handlers.NewPlateHandlerFromService(stub)
	r := chi.NewRouter()
	r.Route("/api/plates", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.Update)
			r.Delete("/", h.Delete)
		})
	})
	return r
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestPlatesHandler_Create_201(t *testing.T) {
	stub := &stubPlateService{
		createFn: func(_ context.Context, in plate.PlateInput) (*plate.Plate, error) {
			return &plate.Plate{ID: 42, Date: in.Date, SlotID: in.SlotID}, nil
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

func TestPlatesHandler_DeleteReturnsUndo(t *testing.T) {
	r := newPlateRouter(&stubPlateService{})

	req := httptest.NewRequest(http.MethodDelete, "/api/plates/7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Removed     map[string]any `json:"removed"`
		AlsoChanged []any          `json:"also_changed"`
		Undo        plate.Undo     `json:"undo"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotNil(t, resp.Removed)
	assert.NotNil(t, resp.AlsoChanged)
	assert.Equal(t, plate.Undo{FirstEventID: 7, LastEventID: 8, Cells: []plate.CellUndo{}}, resp.Undo)
}

func TestPlatesHandler_PlateJSONHasNoSkipped(t *testing.T) {
	r := newPlateRouter(&stubPlateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/plates/7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotContains(t, resp, "skipped")
	assert.Equal(t, float64(7), resp["id"])
}

func TestPlatesHandler_CellConflicts_409(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		err     error
		wantKey string
	}{
		{
			"create into none cell", http.MethodPost, "/api/plates", `{"date":"2025-03-10","slot_id":1}`,
			domain.ErrCellUnavailable, "error.plate.cell_unavailable",
		},
		{
			"create into occupied cell", http.MethodPost, "/api/plates", `{"date":"2025-03-10","slot_id":1}`,
			domain.ErrCellOccupied, "error.plate.cell_occupied",
		},
		{
			"move into none cell", http.MethodPut, "/api/plates/1", `{"date":"2025-03-11"}`,
			domain.ErrCellUnavailable, "error.plate.cell_unavailable",
		},
		{
			"move into occupied cell", http.MethodPut, "/api/plates/1", `{"slot_id":2}`,
			domain.ErrCellOccupied, "error.plate.cell_occupied",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := fmt.Errorf("check cell: %w", tt.err)
			r := newPlateRouter(&stubPlateService{
				createFn: func(context.Context, plate.PlateInput) (*plate.Plate, error) { return nil, wrapped },
				updateFn: func(context.Context, int64, plate.PlatePatch) (*plate.Plate, error) { return nil, wrapped },
			})
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
			var resp map[string]any
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.Equal(t, tt.wantKey, resp["message_key"])
			assert.Equal(t, float64(http.StatusConflict), resp["status"])
		})
	}
}

func TestPlatesRouter_SkipRouteIsGone(t *testing.T) {
	r := transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Plates: handlers.NewPlateHandlerFromService(&stubPlateService{}),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/plates/1/skip", strings.NewReader(`{"skipped":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, w.Code, w.Body.String())
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
			return &plate.Plate{ID: id, SlotID: 1, Date: d}, nil
		},
		updateFn: func(_ context.Context, id int64, patch plate.PlatePatch) (*plate.Plate, error) {
			capturedDate = *patch.Date
			return &plate.Plate{ID: id, SlotID: 1, Date: *patch.Date}, nil
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
