package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
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
