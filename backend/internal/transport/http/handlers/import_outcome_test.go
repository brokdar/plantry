package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func TestImportCheckOutcome_StoresKeptUndoneTouchAndFlags(t *testing.T) {
	t.Parallel()
	db := testhelper.NewTestDB(t)
	r := chi.NewRouter()
	r.Post("/api/import/check/outcome", handlers.NewImportHandler(nil).WithOutcomes(sqlite.NewOutcomeRepo(db)).CheckOutcome)
	post := func(body string) int {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/import/check/outcome", bytes.NewBufferString(body)))
		return rec.Code
	}

	require.Equal(t, http.StatusNoContent, post(`{"run_id":7,"kept":["times","role"],"undone":["keep"],"touch":"skipped","flags":2}`))
	// A resave replaces the row.
	require.Equal(t, http.StatusNoContent, post(`{"run_id":7,"kept":["times"],"undone":[],"touch":"added","flags":2}`))

	var kept, undone, touch string
	var flags, n int
	require.NoError(t, db.QueryRow(`SELECT kept, undone, touch, flags FROM import_check_outcomes WHERE run_id = 7`).Scan(&kept, &undone, &touch, &flags))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM import_check_outcomes`).Scan(&n))
	assert.Equal(t, `["times"]`, kept)
	assert.Equal(t, `[]`, undone)
	assert.Equal(t, "added", touch)
	assert.Equal(t, 2, flags)
	assert.Equal(t, 1, n)

	assert.Equal(t, http.StatusBadRequest, post(`{"run_id":0,"touch":"none"}`))
	assert.Equal(t, http.StatusBadRequest, post(`{"run_id":7,"touch":"maybe"}`))
}
