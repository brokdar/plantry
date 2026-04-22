package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func setupProfileRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewProfileRepo(db)
	svc := profile.NewService(repo)
	h := handlers.NewProfileHandler(svc)
	r := chi.NewRouter()
	r.Route("/api/profile", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Put("/", h.Update)
	})
	return r
}

func TestProfileHandler_Get_Default(t *testing.T) {
	r := setupProfileRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "en", body["locale"])
	assert.Nil(t, body["kcal_target"])
}

func TestProfileHandler_Update_Valid(t *testing.T) {
	r := setupProfileRouter(t)

	payload := `{"kcal_target":1800,"protein_pct":35,"fat_pct":30,"carbs_pct":35,"dietary_restrictions":["vegetarian"],"locale":"de"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, float64(1800), body["kcal_target"])
	assert.Equal(t, "de", body["locale"])
}

func TestProfileHandler_Update_InvalidMacroSum(t *testing.T) {
	r := setupProfileRouter(t)

	payload := `{"protein_pct":50,"fat_pct":30,"carbs_pct":30}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusBadRequest, resp.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "error.profile.invalid_macros", body["message_key"])
}

func TestProfileHandler_Update_InvalidKcal(t *testing.T) {
	r := setupProfileRouter(t)

	payload := `{"kcal_target":0}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestProfileHandler_Update_MalformedJSON(t *testing.T) {
	r := setupProfileRouter(t)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(`{bad json`)))
	require.Equal(t, http.StatusBadRequest, resp.Code)
}
