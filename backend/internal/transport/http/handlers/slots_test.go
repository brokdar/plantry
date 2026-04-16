package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func setupSlotRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewSlotRepo(db)
	svc := slot.NewService(repo)
	h := handlers.NewSlotHandler(svc)
	r := chi.NewRouter()
	r.Route("/api/settings/slots", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
	return r
}

func TestSlotHandler_CreateAndList(t *testing.T) {
	r := setupSlotRouter(t)

	body := `{"name_key":"slot.breakfast","icon":"Coffee","sort_order":1,"active":true}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/settings/slots", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	assert.Equal(t, "slot.breakfast", created["name_key"])

	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/settings/slots", nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var listed map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))
	items, ok := listed["items"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 1)
}

func TestSlotHandler_Create_InvalidBody(t *testing.T) {
	r := setupSlotRouter(t)
	body := `{"name_key":"","icon":"Coffee"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/settings/slots", bytes.NewBufferString(body)))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestSlotHandler_Update(t *testing.T) {
	r := setupSlotRouter(t)
	id := createSlot(t, r, "slot.lunch", "Sun", 2, true)
	body := `{"name_key":"slot.lunch","icon":"Soup","sort_order":3,"active":true}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/settings/slots/%d", id), bytes.NewBufferString(body)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
}

func TestSlotHandler_Delete_NotFound(t *testing.T) {
	r := setupSlotRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/settings/slots/9999", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func createSlot(t *testing.T, r http.Handler, nameKey, icon string, sort int, active bool) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name_key": nameKey, "icon": icon, "sort_order": sort, "active": active,
	})
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/settings/slots", bytes.NewReader(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return int64(got["id"].(float64))
}
