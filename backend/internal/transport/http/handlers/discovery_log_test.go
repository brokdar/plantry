package handlers_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// failingDiscovery fails every call with err.
type failingDiscovery struct{ err error }

func (f failingDiscovery) Discover(context.Context, discovery.Request, time.Time) (int64, error) {
	return 0, f.err
}

func (f failingDiscovery) RunState(context.Context, int64) (discovery.RunState, error) {
	return discovery.RunState{}, f.err
}

func (f failingDiscovery) Daily(context.Context, time.Time) (discovery.Daily, error) {
	return discovery.Daily{}, f.err
}

func (f failingDiscovery) Another(context.Context, time.Time) (discovery.Daily, error) {
	return discovery.Daily{}, f.err
}

func (f failingDiscovery) Suggestions(context.Context, *discovery.Status, *int64) ([]discovery.Suggestion, error) {
	return nil, f.err
}

func (f failingDiscovery) Accept(context.Context, int64, int64) (*discovery.Suggestion, error) {
	return nil, f.err
}

func (f failingDiscovery) Reject(context.Context, int64, *discovery.Reason) (*discovery.Suggestion, error) {
	return nil, f.err
}

// DoD sfh: a discovery request that fails with a 5xx is logged server side
// (the daily popup hides errors from the user); a 4xx is not.
func TestDiscoveryHandler_Logs5xx(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := handlers.NewDiscoveryHandler(failingDiscovery{err: errors.New("disk I/O error")})
	w := httptest.NewRecorder()
	h.Daily(w, httptest.NewRequest(http.MethodGet, "/api/discovery/daily?date=2026-11-02", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, logs.String(), "level=ERROR")
	assert.Contains(t, logs.String(), "disk I/O error")

	logs.Reset()
	h = handlers.NewDiscoveryHandler(failingDiscovery{err: domain.ErrDiscoveryRunning})
	w = httptest.NewRecorder()
	h.Start(w, httptest.NewRequest(http.MethodPost, "/api/discovery", strings.NewReader(`{"mode":"daily"}`)))
	assert.Less(t, w.Code, 500)
	assert.NotContains(t, logs.String(), "level=ERROR")
}
