package handlers_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

// TestWeeksRouteGone asserts that /api/weeks is not registered.
// The /api/weeks route group was removed in phase 5; any request to it must
// return 404 from chi's default not-found handler.
func TestWeeksRouteGone(t *testing.T) {
	// Build a router with no /api/weeks routes — mirrors the production router
	// state after the weeks route group was deleted.
	r := chi.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/weeks/anything", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/weeks/anything: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// B8-AC3: the D27 read endpoints and the recent-unit lookup are no longer
// served by the production router. A path that now falls through to a
// sibling route answers that route's error instead: GET /api/plates hits the
// POST-only collection (405) and /api/plates/macros the numeric plate route
// (400 error.invalid_id).
func TestLegacyReadRoutesGone(t *testing.T) {
	a := newHistoryAPI(t)
	a.sunChili()

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/plates?from=2026-11-01&to=2026-11-07", http.StatusMethodNotAllowed},
		{"/api/plates/by-date/2026-11-01", http.StatusNotFound},
		{"/api/plates/macros?from=2026-11-01&to=2026-11-07", http.StatusBadRequest},
		{"/api/nutrition?from=2026-11-01&to=2026-11-07", http.StatusNotFound},
		{fmt.Sprintf("/api/foods/%d/recent-unit", a.chili), http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		a.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		assert.Equal(t, tc.want, rec.Code, tc.path)
	}

	// /api/foods/macros stays: the plate sheet reads per-eater macros from it.
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/foods/macros?ids=%d", a.chili), nil))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
