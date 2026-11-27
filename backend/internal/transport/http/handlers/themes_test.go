package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/theme"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// The themes routes set, read, limit and remove a window's themes and
// accept or decline its suggestion.
func TestThemeHandler(t *testing.T) {
	t.Parallel()
	svc := theme.NewService(theme.Deps{
		Store:    sqlite.NewThemeRepo(testhelper.NewTestDB(t)),
		ChefOn:   func(context.Context) (bool, error) { return true, nil },
		Library:  func(context.Context, theme.Theme) (int, error) { return 0, nil },
		Find:     func(context.Context, string, int) error { return nil },
		Cuisines: func(context.Context) ([]string, error) { return []string{"thai"}, nil },
		Now:      func() time.Time { return time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC) },
	})
	h := handlers.NewThemeHandler(svc)
	r := chi.NewRouter()
	r.Route("/api/windows/{start}/themes", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Post("/", h.Post)
		r.Delete("/{id}", h.Delete)
		r.Post("/suggestion/accept", h.Accept)
		r.Post("/suggestion/decline", h.Decline)
	})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	const base = "/api/windows/2026-11-28/themes"

	w := do(http.MethodGet, base, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"themes":[],"again":[],"suggestion":{"kind":"cuisine","value":"thai","state":"open"}}`, w.Body.String())
	w = do(http.MethodPost, base+"/suggestion/accept", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"finding":2`)

	w = do(http.MethodPost, base, `{"kind":"mood","value":"One-pot"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res theme.Result
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, theme.SourceUser, res.Theme.Source)
	w = do(http.MethodPost, base, `{"kind":"cuisine","value":"greek"}`)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "error.theme.limit")
	w = do(http.MethodPost, base, `{"kind":"cuisine","value":"greek","source":"suggestion"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = do(http.MethodDelete, base+"/"+strconv.FormatInt(res.Theme.ID, 10), "")
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	w = do(http.MethodDelete, base+"/"+strconv.FormatInt(res.Theme.ID, 10), "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = do(http.MethodPost, base+"/suggestion/decline", "")
	assert.Equal(t, http.StatusNotFound, w.Code, "decided already")
	w = do(http.MethodGet, "/api/windows/bad/themes", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
