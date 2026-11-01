package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Phase 3 B8 (§3.2): mid-week re-plan endpoints. Today is Wed 2026-11-11 of
// the window Sat 11-07 … Fri 11-13; a confirmed Thursday lunch is marked No
// plate.

const replanToday = "?today=2026-11-11"

func TestPlanReplanHTTP(t *testing.T) {
	a := newGenAPI(t)
	var lunch, food int64
	require.NoError(t, a.db.QueryRow(`SELECT id FROM time_slots WHERE name_key = 'slot.lunch'`).Scan(&lunch))
	require.NoError(t, a.db.QueryRow(`SELECT id FROM foods WHERE name = 'Main 1'`).Scan(&food))
	code, body := a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2026-11-12","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, lunch, food))
	require.Equal(t, http.StatusCreated, code, body)
	pid := body["id"].(float64)

	code, pv := a.do(http.MethodGet, fmt.Sprintf("/api/plan/replan/preview?from=2026-11-11&to=2026-11-13&none=2026-11-12:%d&today=2026-11-11", lunch), "")
	require.Equal(t, http.StatusOK, code, pv)
	assert.Equal(t, []any{map[string]any{"date": "2026-11-12", "slot_id": float64(lunch)}}, pv["none_cells"])
	assert.Equal(t, []any{map[string]any{"plate_id": pid, "date": "2026-11-12", "slot_id": float64(lunch), "food_name": "Main 1"}}, pv["confirmed_affected"])
	assert.Equal(t, []any{}, pv["spare"])

	req := fmt.Sprintf(`{"from":"2026-11-11","to":"2026-11-13","none_cells":[{"date":"2026-11-12","slot_id":%d}],"spare":"freeze","regenerate":true`, lunch)
	code, body = a.do(http.MethodPost, "/api/plan/replan"+replanToday, req+`,"unlock_confirmed":false}`)
	require.Equal(t, http.StatusConflict, code, body)
	assert.Equal(t, "error.plan.confirmed_affected", body["message_key"])
	assert.Equal(t, pv["confirmed_affected"], body["confirmed_affected"])
	assert.Empty(t, a.queued, "no run started")

	code, body = a.do(http.MethodPost, "/api/plan/replan"+replanToday, req+`,"unlock_confirmed":true}`)
	require.Equal(t, http.StatusOK, code, body)
	removed := body["removed"].(map[string]any)
	assert.Len(t, removed["plates"], 1)
	require.IsType(t, float64(0), body["run_id"])
	a.drain()
	_, run := a.do(http.MethodGet, fmt.Sprintf("/api/plan/runs/%.0f", body["run_id"]), "")
	assert.Equal(t, "regenerate", run["kind"])
	assert.Equal(t, "ok", run["status"], run)
	var n int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM plates WHERE date < '2026-11-11'`).Scan(&n))
	assert.Zero(t, n, "nothing before from")
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM plates WHERE date = '2026-11-12' AND slot_id = ?`, lunch).Scan(&n))
	assert.Zero(t, n, "the No-plate cell stays empty")

	code, body = a.do(http.MethodPost, "/api/plan/replan"+replanToday, `{"from":"2026-11-12","to":"2026-11-13","spare":"keep"}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Nil(t, body["run_id"])
	assert.Equal(t, map[string]any{"plates": []any{}, "sessions": []any{}}, body["removed"])
}

func TestPlanReplanHTTP_Errors(t *testing.T) {
	a := newGenAPI(t)
	tests := []struct {
		name, method, path, body string
		status                   int
		key                      string
	}{
		{"preview from in the past", http.MethodGet, "/api/plan/replan/preview?from=2026-11-10&to=2026-11-13&today=2026-11-11", "", http.StatusBadRequest, "error.plan.replan_past"},
		{"replan from in the past", http.MethodPost, "/api/plan/replan" + replanToday, `{"from":"2026-11-10","to":"2026-11-13","spare":"keep"}`, http.StatusBadRequest, "error.plan.replan_past"},
		{"preview without dates", http.MethodGet, "/api/plan/replan/preview", "", http.StatusBadRequest, "error.invalid_body"},
		{"preview bad none cell", http.MethodGet, "/api/plan/replan/preview?from=2026-11-11&to=2026-11-13&none=2026-11-12", "", http.StatusBadRequest, "error.invalid_body"},
		{"preview range over two windows", http.MethodGet, "/api/plan/replan/preview?from=2026-11-11&to=2026-11-14&today=2026-11-11", "", http.StatusBadRequest, "error.invalid_body"},
		{"malformed body", http.MethodPost, "/api/plan/replan", `{`, http.StatusBadRequest, "error.invalid_body"},
		{"bad none cell date", http.MethodPost, "/api/plan/replan" + replanToday, `{"from":"2026-11-11","to":"2026-11-13","spare":"keep","none_cells":[{"date":"x","slot_id":1}]}`, http.StatusBadRequest, "error.invalid_body"},
		{"unknown spare mode", http.MethodPost, "/api/plan/replan" + replanToday, `{"from":"2026-11-11","to":"2026-11-13","spare":"toss"}`, http.StatusBadRequest, "error.invalid_body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := a.do(tt.method, tt.path, tt.body)
			assert.Equal(t, tt.status, code, body)
			assert.Equal(t, tt.key, body["message_key"])
		})
	}
}
