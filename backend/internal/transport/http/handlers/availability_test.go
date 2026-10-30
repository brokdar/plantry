package handlers_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type availabilityEnv struct {
	db       *sql.DB
	router   http.Handler
	slotA    int64
	slotB    int64
	inactive int64
	preset   int64
}

func setupAvailabilityRouter(t *testing.T) *availabilityEnv {
	t.Helper()
	db := testhelper.NewTestDB(t)
	_, svc := testsvc.Plan(db)
	h := handlers.NewAvailabilityHandler(svc)
	r := chi.NewRouter()
	r.Route("/api/availability", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/defaults", h.GetDefaults)
		r.Put("/defaults", h.PutDefaults)
		r.Put("/{date}/{slotId}", h.PutOverride)
		r.Delete("/{date}/{slotId}", h.DeleteOverride)
	})
	env := &availabilityEnv{db: db, router: r}
	insert := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		require.NoError(t, err)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	env.slotA = insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.breakfast', 'Coffee', 0, 1)`)
	env.slotB = insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'Soup', 1, 1)`)
	env.inactive = insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.snack', 'Apple', 2, 0)`)
	env.preset = insert(`INSERT INTO presets (name) VALUES ('Oatmeal')`)
	return env
}

func (e *availabilityEnv) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Buffer
	if body != "" {
		rdr = bytes.NewBufferString(body)
	} else {
		rdr = &bytes.Buffer{}
	}
	resp := httptest.NewRecorder()
	e.router.ServeHTTP(resp, httptest.NewRequest(method, path, rdr))
	return resp
}

func decodeMap(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &m), resp.Body.String())
	return m
}

func TestAvailabilityHandler_Defaults(t *testing.T) {
	e := setupAvailabilityRouter(t)

	resp := e.do(t, http.MethodGet, "/api/availability/defaults", "")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	items := decodeMap(t, resp)["items"].([]any)
	assert.Len(t, items, 14, "7 weekdays × 2 active slots")
	first := items[0].(map[string]any)
	assert.Equal(t, map[string]any{"weekday": 0.0, "slot_id": float64(e.slotA), "mode": "plan", "preset_id": nil}, first)

	body := fmt.Sprintf(`{"items":[{"weekday":0,"slot_id":%d,"mode":"routine","preset_id":%d},{"weekday":6,"slot_id":%d,"mode":"none"},{"weekday":1,"slot_id":%d,"mode":"plan"}]}`,
		e.slotA, e.preset, e.slotB, e.slotA)
	resp = e.do(t, http.MethodPut, "/api/availability/defaults", body)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	items = decodeMap(t, resp)["items"].([]any)
	require.Len(t, items, 14)
	assert.Equal(t, "routine", items[0].(map[string]any)["mode"])
	assert.Equal(t, float64(e.preset), items[0].(map[string]any)["preset_id"])
	assert.Equal(t, "none", items[13].(map[string]any)["mode"])
}

func TestAvailabilityHandler_PutDefaults_Errors(t *testing.T) {
	e := setupAvailabilityRouter(t)
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantKey    string
	}{
		{"malformed json", `{"items":[`, http.StatusBadRequest, "error.invalid_body"},
		{"missing items", `{}`, http.StatusBadRequest, "error.invalid_body"},
		{"weekday out of range", fmt.Sprintf(`{"items":[{"weekday":7,"slot_id":%d,"mode":"none"}]}`, e.slotA), http.StatusBadRequest, "error.invalid_body"},
		{"duplicate", fmt.Sprintf(`{"items":[{"weekday":1,"slot_id":%d,"mode":"none"},{"weekday":1,"slot_id":%d,"mode":"none"}]}`, e.slotA, e.slotA), http.StatusBadRequest, "error.invalid_body"},
		{"routine without preset", fmt.Sprintf(`{"items":[{"weekday":1,"slot_id":%d,"mode":"routine"}]}`, e.slotA), http.StatusBadRequest, "error.availability.preset_required"},
		{"none with preset", fmt.Sprintf(`{"items":[{"weekday":1,"slot_id":%d,"mode":"none","preset_id":%d}]}`, e.slotA, e.preset), http.StatusBadRequest, "error.availability.preset_not_allowed"},
		{"unknown slot", `{"items":[{"weekday":1,"slot_id":9999,"mode":"none"}]}`, http.StatusBadRequest, "error.availability.slot_unknown"},
		{"unknown preset", fmt.Sprintf(`{"items":[{"weekday":1,"slot_id":%d,"mode":"routine","preset_id":9999}]}`, e.slotA), http.StatusBadRequest, "error.availability.preset_unknown"},
		{"inactive slot", fmt.Sprintf(`{"items":[{"weekday":1,"slot_id":%d,"mode":"none"}]}`, e.inactive), http.StatusBadRequest, "error.availability.slot_inactive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := e.do(t, http.MethodPut, "/api/availability/defaults", tt.body)
			require.Equal(t, tt.wantStatus, resp.Code, resp.Body.String())
			assert.Equal(t, tt.wantKey, decodeMap(t, resp)["message_key"])
		})
	}
}

func TestAvailabilityHandler_List(t *testing.T) {
	e := setupAvailabilityRouter(t)
	resp := e.do(t, http.MethodPut, fmt.Sprintf("/api/availability/2026-10-27/%d", e.slotB), `{"mode":"none","note":"away"}`)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	resp = e.do(t, http.MethodGet, "/api/availability?from=2026-10-26&to=2026-11-01", "")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	items := decodeMap(t, resp)["items"].([]any)
	require.Len(t, items, 14)
	assert.Equal(t, map[string]any{
		"date": "2026-10-26", "slot_id": float64(e.slotA), "mode": "plan", "source": "fallback",
		"preset_id": nil, "preset_name": nil, "note": nil,
	}, items[0])
	assert.Equal(t, map[string]any{
		"date": "2026-10-27", "slot_id": float64(e.slotB), "mode": "none", "source": "override",
		"preset_id": nil, "preset_name": nil, "note": "away",
	}, items[3])
}

func TestAvailabilityHandler_List_BadRange(t *testing.T) {
	e := setupAvailabilityRouter(t)
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"missing from", "?to=2026-10-26", http.StatusBadRequest},
		{"missing to", "?from=2026-10-26", http.StatusBadRequest},
		{"bad date", "?from=2026-13-01&to=2026-10-26", http.StatusBadRequest},
		{"to before from", "?from=2026-10-27&to=2026-10-26", http.StatusBadRequest},
		{"63 days", "?from=2026-01-01&to=2026-03-04", http.StatusBadRequest},
		{"62 days", "?from=2026-01-01&to=2026-03-03", http.StatusOK},
		{"single day", "?from=2026-10-26&to=2026-10-26", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := e.do(t, http.MethodGet, "/api/availability"+tt.query, "")
			assert.Equal(t, tt.want, resp.Code, resp.Body.String())
		})
	}
}

func TestAvailabilityHandler_PutOverride_Routine(t *testing.T) {
	e := setupAvailabilityRouter(t)
	resp := e.do(t, http.MethodPut, fmt.Sprintf("/api/availability/2026-10-26/%d", e.slotA), fmt.Sprintf(`{"mode":"routine","preset_id":%d}`, e.preset))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decodeMap(t, resp)
	assert.Equal(t, map[string]any{
		"date": "2026-10-26", "slot_id": float64(e.slotA), "mode": "routine", "source": "override",
		"preset_id": float64(e.preset), "preset_name": "Oatmeal", "note": nil,
	}, body["cell"])
	_, has := body["removed_plate"]
	assert.False(t, has)
}

func TestAvailabilityHandler_PutOverride_Errors(t *testing.T) {
	e := setupAvailabilityRouter(t)
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantKey    string
	}{
		{"malformed json", fmt.Sprintf("/api/availability/2026-10-26/%d", e.slotA), `{"mode":`, http.StatusBadRequest, "error.invalid_body"},
		{"non-numeric slot", "/api/availability/2026-10-26/abc", `{"mode":"none"}`, http.StatusBadRequest, "error.invalid_id"},
		{"bad date", fmt.Sprintf("/api/availability/2026-02-30/%d", e.slotA), `{"mode":"none"}`, http.StatusBadRequest, "error.invalid_body"},
		{"bad mode", fmt.Sprintf("/api/availability/2026-10-26/%d", e.slotA), `{"mode":"away"}`, http.StatusBadRequest, "error.invalid_body"},
		{"unknown slot", "/api/availability/2026-10-26/9999", `{"mode":"none"}`, http.StatusNotFound, "error.not_found"},
		{"unknown preset", fmt.Sprintf("/api/availability/2026-10-26/%d", e.slotA), `{"mode":"routine","preset_id":9999}`, http.StatusBadRequest, "error.availability.preset_unknown"},
		{"routine without preset", fmt.Sprintf("/api/availability/2026-10-26/%d", e.slotA), `{"mode":"routine"}`, http.StatusBadRequest, "error.availability.preset_required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := e.do(t, http.MethodPut, tt.path, tt.body)
			require.Equal(t, tt.wantStatus, resp.Code, resp.Body.String())
			assert.Equal(t, tt.wantKey, decodeMap(t, resp)["message_key"])
		})
	}
}

func TestAvailabilityHandler_PutOverride_CellHasPlate(t *testing.T) {
	e := setupAvailabilityRouter(t)
	res, err := e.db.Exec(`INSERT INTO plates (slot_id, date, note) VALUES (?, '2026-10-26', 'curry night')`, e.slotB)
	require.NoError(t, err)
	plateID, err := res.LastInsertId()
	require.NoError(t, err)
	path := fmt.Sprintf("/api/availability/2026-10-26/%d", e.slotB)

	resp := e.do(t, http.MethodPut, path, `{"mode":"none"}`)
	require.Equal(t, http.StatusConflict, resp.Code, resp.Body.String())
	assert.Equal(t, "error.availability.cell_has_plate", decodeMap(t, resp)["message_key"])

	resp = e.do(t, http.MethodPut, path, `{"mode":"none","note":"invited","remove_plate":true}`)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decodeMap(t, resp)
	assert.Equal(t, "none", body["cell"].(map[string]any)["mode"])
	assert.Equal(t, "invited", body["cell"].(map[string]any)["note"])
	removed, ok := body["removed_plate"].(map[string]any)
	require.True(t, ok, "removed_plate present")
	assert.Equal(t, float64(plateID), removed["id"])
	assert.Equal(t, "2026-10-26", removed["date"])
	assert.Equal(t, "curry night", removed["note"])
	assert.Equal(t, []any{}, removed["components"])

	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM plates`).Scan(&n))
	assert.Equal(t, 0, n)
}

func TestAvailabilityHandler_DeleteOverride(t *testing.T) {
	e := setupAvailabilityRouter(t)
	path := fmt.Sprintf("/api/availability/2026-10-26/%d", e.slotA)
	resp := e.do(t, http.MethodPut, path, `{"mode":"none"}`)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	resp = e.do(t, http.MethodDelete, path, "")
	assert.Equal(t, http.StatusNoContent, resp.Code)
	resp = e.do(t, http.MethodDelete, path, "")
	assert.Equal(t, http.StatusNoContent, resp.Code, "idempotent")

	resp = e.do(t, http.MethodGet, "/api/availability?from=2026-10-26&to=2026-10-26", "")
	require.Equal(t, http.StatusOK, resp.Code)
	items := decodeMap(t, resp)["items"].([]any)
	assert.Equal(t, "fallback", items[0].(map[string]any)["source"])

	resp = e.do(t, http.MethodDelete, "/api/availability/2026-10-26/abc", "")
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	resp = e.do(t, http.MethodDelete, fmt.Sprintf("/api/availability/not-a-date/%d", e.slotA), "")
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}
