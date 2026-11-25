package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/devclock"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type dateRecorder struct{ got time.Time }

func (d *dateRecorder) Today(_ context.Context, date time.Time) (*planview.Today, error) {
	d.got = date
	return &planview.Today{}, nil
}

func devRouter(clock *devclock.Clock, devMode bool, rec *dateRecorder) http.Handler {
	h := transport.Handlers{
		Today:   handlers.NewTodayHandler(rec).WithClock(clock.Now),
		DevMode: devMode,
		Dev:     handlers.NewDevHandler(clock),
	}
	return transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), h)
}

func do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The dev clock moves "today": a set and an advance change what GET /api/today
// reads when the client sends no date.
func TestDevClock_MovesToday(t *testing.T) {
	clock := devclock.New()
	rec := &dateRecorder{}
	r := devRouter(clock, true, rec)

	w := do(t, r, http.MethodPut, "/api/dev/clock", `{"now":"2031-03-01"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"today":"2031-03-01"`)

	require.Equal(t, http.StatusOK, do(t, r, http.MethodGet, "/api/today", "").Code)
	assert.Equal(t, "2031-03-01", rec.got.Format(time.DateOnly))

	w = do(t, r, http.MethodPut, "/api/dev/clock", `{"advance_hours":48}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, http.StatusOK, do(t, r, http.MethodGet, "/api/today", "").Code)
	assert.Equal(t, "2031-03-03", rec.got.Format(time.DateOnly))
}

func TestDevClock_BadBody(t *testing.T) {
	r := devRouter(devclock.New(), true, &dateRecorder{})
	for _, body := range []string{`{}`, `{"now":"x"}`, `{"advance_hours":0}`, `{"advance_hours":-5}`, `{"now":"2031-03-01","advance_hours":1}`, `nope`} {
		assert.Equal(t, http.StatusBadRequest, do(t, r, http.MethodPut, "/api/dev/clock", body).Code, body)
	}
}

// Production (dev mode off) has no clock endpoint, even when a handler exists.
func TestDevClock_AbsentWithoutDevMode(t *testing.T) {
	clock := devclock.New()
	r := devRouter(clock, false, &dateRecorder{})
	assert.Equal(t, http.StatusNotFound, do(t, r, http.MethodPut, "/api/dev/clock", `{"now":"2031-03-01"}`).Code)
	assert.Equal(t, http.StatusNotFound, do(t, r, http.MethodGet, "/api/dev/clock", "").Code)
	assert.WithinDuration(t, time.Now(), clock.Now(), time.Minute)
}
