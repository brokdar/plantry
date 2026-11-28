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

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/libreview"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// The review routes serve the month's card (null when off), dismiss a
// finding or the card, and group a duplicates finding's recipes as variants.
func TestLibraryReviewHandler(t *testing.T) {
	t.Parallel()
	db := testhelper.NewTestDB(t)
	exec := func(q string, args ...any) {
		t.Helper()
		_, err := db.Exec(q, args...)
		require.NoError(t, err, q)
	}
	for _, name := range []string{"Chili", "chili"} {
		exec(`INSERT INTO foods (name, kind, role, reference_portions, prep_minutes) VALUES (?, 'composed', 'main', 1, 10)`, name)
		exec(`INSERT INTO food_instructions (food_id, step_number, text) SELECT id, 1, 'cook' FROM foods WHERE name = ?`, name)
	}
	on := false
	h := handlers.NewLibraryReviewHandler(libreview.NewService(libreview.Deps{
		Store: sqlite.NewLibraryReviewRepo(db), Tx: sqlite.NewTxRunner(db), Now: func() time.Time { return time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC) },
		Enabled: func(context.Context) (bool, error) { return on, nil },
	}))
	r := chi.NewRouter()
	r.Get("/api/library-review", h.Get)
	r.Post("/api/library-review/dismiss", h.Dismiss)
	r.Post("/api/library-review/variants", h.MakeVariants)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	card := func() *libreview.Card {
		w := do(http.MethodGet, "/api/library-review", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var out struct {
			Card *libreview.Card `json:"card"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out.Card
	}

	assert.Nil(t, card(), "off")
	on = true
	c := card()
	require.NotNil(t, c)
	assert.Equal(t, "2026-11", c.Month)
	require.Len(t, c.Findings, 1)
	assert.Equal(t, "dup:1-2", c.Findings[0].Key)

	assert.Equal(t, http.StatusNotFound, do(http.MethodPost, "/api/library-review/variants", `{"key":"dup:9-10"}`).Code)
	assert.Equal(t, http.StatusBadRequest, do(http.MethodPost, "/api/library-review/variants", `{}`).Code)
	assert.Equal(t, http.StatusNoContent, do(http.MethodPost, "/api/library-review/variants", `{"key":"dup:1-2"}`).Code)
	var g1, g2 int64
	require.NoError(t, db.QueryRow(`SELECT variant_group_id FROM foods WHERE id = 1`).Scan(&g1))
	require.NoError(t, db.QueryRow(`SELECT variant_group_id FROM foods WHERE id = 2`).Scan(&g2))
	assert.NotZero(t, g1)
	assert.Equal(t, g1, g2)
	assert.Nil(t, card(), "the only finding was acted on")
	assert.Equal(t, http.StatusNotFound, do(http.MethodPost, "/api/library-review/dismiss", `{"key":"nope"}`).Code)
	assert.Equal(t, http.StatusNoContent, do(http.MethodPost, "/api/library-review/dismiss", `{}`).Code)
}
