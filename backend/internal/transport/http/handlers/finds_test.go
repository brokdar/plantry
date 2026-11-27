package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// The finds API: request (Chef on only), list by state, read, discard with
// a reason, and a second decision refused.
func TestFindsHandler(t *testing.T) {
	t.Parallel()
	db := testhelper.NewTestDB(t)
	queued := 0
	svc := finds.NewService(finds.Deps{
		Store:   sqlite.NewFindRepo(db),
		Enqueue: func(context.Context, int64, bool) error { queued++; return nil },
	})
	on := false
	h := handlers.NewFindsHandler(svc, func(context.Context) (bool, error) { return on, nil })
	r := chi.NewRouter()
	r.Get("/api/finds", h.List)
	r.Post("/api/finds", h.Create)
	r.Get("/api/finds/{id}", h.Get)
	r.Post("/api/finds/{id}/keep", h.Keep)
	r.Post("/api/finds/{id}/discard", h.Discard)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	w := do(http.MethodPost, "/api/finds", `{"dish":"pho"}`)
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "error.chef.off")

	on = true
	w = do(http.MethodPost, "/api/finds", `{"dish":"pho"}`)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"state":"searching"`)
	require.Equal(t, 1, queued)

	w = do(http.MethodGet, "/api/finds?state=searching", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"dish":"pho"`)
	require.Equal(t, http.StatusBadRequest, do(http.MethodGet, "/api/finds?state=nope", "").Code)
	require.Equal(t, http.StatusOK, do(http.MethodGet, "/api/finds/1", "").Code)
	require.Equal(t, http.StatusNotFound, do(http.MethodGet, "/api/finds/99", "").Code)

	// A searching find cannot be decided yet.
	require.Equal(t, http.StatusConflict, do(http.MethodPost, "/api/finds/1/discard", `{"reason":"taste"}`).Code)
	_, err := sqlite.NewFindRepo(db).Finish(context.Background(), 1, finds.Outcome{State: finds.StateReady})
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, do(http.MethodPost, "/api/finds/1/discard", `{"reason":"meh"}`).Code)
	w = do(http.MethodPost, "/api/finds/1/discard", `{"reason":"too_long"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"state":"discarded"`)
	require.Equal(t, http.StatusConflict, do(http.MethodPost, "/api/finds/1/keep", `{"food_id":3}`).Code)
	require.Equal(t, http.StatusBadRequest, do(http.MethodPost, "/api/finds/1/keep", `{}`).Code)
}

// Without an AI provider a find is refused up front, not left searching.
func TestFindsHandler_NoProvider(t *testing.T) {
	t.Parallel()
	svc := finds.NewService(finds.Deps{
		Store:   sqlite.NewFindRepo(testhelper.NewTestDB(t)),
		LLM:     llm.StaticResolver(nil, ""),
		Enqueue: func(context.Context, int64, bool) error { return nil },
	})
	h := handlers.NewFindsHandler(svc, func(context.Context) (bool, error) { return true, nil })
	w := httptest.NewRecorder()
	h.Create(w, httptest.NewRequest(http.MethodPost, "/api/finds", strings.NewReader(`{"dish":"pho"}`)))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "error.ai.provider_missing")
}
