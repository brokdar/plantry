package handlers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// planAPI is the real router over a test DB with the plate, session and
// plan-write handlers. Slots lunch (sort 1) and dinner (sort 2), Chili and
// Curry (composed), household portions 2.
type planAPI struct {
	t             *testing.T
	db            *sql.DB
	router        http.Handler
	lunch, dinner int64
	chili, curry  int64
}

func newPlanAPI(t *testing.T) *planAPI {
	t.Helper()
	db := testhelper.NewTestDB(t)
	plates := testsvc.Plates(db)
	a := &planAPI{t: t, db: db}
	a.router = transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Plates:       handlers.NewPlateHandler(plates),
		CookSessions: handlers.NewCookSessionHandler(plates),
		PlanWrite:    handlers.NewPlanWriteHandler(plates),
	})
	a.lunch = a.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'sun', 1, 1)`)
	a.dinner = a.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`)
	a.chili = a.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Chili', 'composed', 'main', 4)`)
	a.curry = a.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Curry', 'composed', 'main', 2)`)
	_, err := db.Exec(`UPDATE user_profile SET household_portions = 2 WHERE id = 1`)
	require.NoError(t, err)
	return a
}

func (a *planAPI) insert(q string) int64 {
	a.t.Helper()
	res, err := a.db.Exec(q)
	require.NoError(a.t, err)
	id, err := res.LastInsertId()
	require.NoError(a.t, err)
	return id
}

func (a *planAPI) do(method, path, body string) (int, map[string]any) {
	a.t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)
	out := map[string]any{}
	if w.Body.Len() > 0 {
		require.NoError(a.t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	}
	return w.Code, out
}

// sunChili creates the Sun dinner Chili plate and returns its id and session.
func (a *planAPI) sunChili() (plateID, sessionID int64) {
	a.t.Helper()
	code, body := a.do(http.MethodPost, "/api/plates",
		fmt.Sprintf(`{"date":"2026-11-01","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, a.dinner, a.chili))
	require.Equal(a.t, http.StatusCreated, code, body)
	comp := body["components"].([]any)[0].(map[string]any)
	return int64(body["id"].(float64)), int64(comp["cook_session_id"].(float64))
}

func TestPlatesHTTP_CreateWithEatersAndComponents(t *testing.T) {
	a := newPlanAPI(t)
	code, body := a.do(http.MethodPost, "/api/plates",
		fmt.Sprintf(`{"date":"2026-11-01","slot_id":%d,"eaters":3.5,"note":"guests","components":[{"food_id":%d}]}`, a.dinner, a.chili))
	require.Equal(t, http.StatusCreated, code, body)
	assert.EqualValues(t, 3.5, body["eaters"])
	assert.Equal(t, "guests", body["note"])
	assert.Nil(t, body["feedback"])
	assert.NotEmpty(t, body["updated_at"])
	comp := body["components"].([]any)[0].(map[string]any)
	assert.EqualValues(t, 1, comp["portions"], "portions default to 1")
	assert.Equal(t, "Chili", comp["food_name"])
	assert.Equal(t, "composed", comp["kind"])
	assert.Equal(t, "main", comp["role"])

	sid := int64(comp["cook_session_id"].(float64))
	code, sess := a.do(http.MethodGet, fmt.Sprintf("/api/cook-sessions/%d", sid), "")
	require.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 3.5, sess["portions"])

	code, body = a.do(http.MethodPut, fmt.Sprintf("/api/plates/%d", int64(body["id"].(float64))), `{"eaters":2}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.EqualValues(t, 2, body["eaters"])
	_, sess = a.do(http.MethodGet, fmt.Sprintf("/api/cook-sessions/%d", sid), "")
	assert.EqualValues(t, 2, sess["portions"], "D11 follows the eaters")

	code, body = a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2026-11-02","slot_id":%d,"eaters":0}`, a.dinner))
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "error.invalid_body", body["message_key"])
}

func TestCookSessionsHTTP_GetShape(t *testing.T) {
	a := newPlanAPI(t)
	plateID, sid := a.sunChili()

	code, body := a.do(http.MethodGet, fmt.Sprintf("/api/cook-sessions/%d", sid), "")
	require.Equal(t, http.StatusOK, code, body)
	assert.EqualValues(t, sid, body["id"])
	assert.EqualValues(t, a.chili, body["food_id"])
	assert.Equal(t, "Chili", body["food_name"])
	assert.Equal(t, "2026-11-01", body["cook_date"])
	assert.EqualValues(t, a.dinner, body["cook_slot_id"])
	assert.EqualValues(t, 2, body["portions"])
	assert.EqualValues(t, 0, body["frozen_portions"])
	assert.Nil(t, body["cooked_at"])
	assert.Nil(t, body["note"])
	assert.EqualValues(t, 3, body["keep_days"])
	assert.Equal(t, "2026-11-04", body["use_by"])
	assert.EqualValues(t, 2, body["consumed"])
	assert.EqualValues(t, 0, body["frozen_consumed"])
	assert.EqualValues(t, 0, body["surplus"])
	assert.EqualValues(t, 1, body["meals"])
	assert.Equal(t, []any{}, body["warnings"])
	links := body["links"].([]any)
	require.Len(t, links, 1)
	link := links[0].(map[string]any)
	assert.EqualValues(t, plateID, link["plate_id"])
	assert.NotNil(t, link["component_id"])
	assert.Equal(t, "2026-11-01", link["date"])
	assert.EqualValues(t, a.dinner, link["slot_id"])
	assert.EqualValues(t, 2, link["portions"])
	assert.Equal(t, "fresh", link["kind"])
	assert.EqualValues(t, 1, link["position"])
}

func TestCookSessionsHTTP_Edits(t *testing.T) {
	a := newPlanAPI(t)
	_, sid := a.sunChili()
	path := fmt.Sprintf("/api/cook-sessions/%d", sid)

	code, body := a.do(http.MethodPut, path, `{"portions":5,"frozen_portions":0,"note":"spicy"}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.EqualValues(t, 5, body["portions"])
	assert.Equal(t, "spicy", body["note"])
	assert.EqualValues(t, 3, body["surplus"])

	code, body = a.do(http.MethodPut, path, `{"portions":5,"frozen_portions":2}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.session.freezing_disabled", body["message_key"])

	code, body = a.do(http.MethodPost, path+"/cooked", `{"cooked":true}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.NotNil(t, body["cooked_at"])
	code, body = a.do(http.MethodPost, path+"/cooked", `{"cooked":false}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Nil(t, body["cooked_at"])
}

func TestCookSessionsHTTP_Leftovers(t *testing.T) {
	a := newPlanAPI(t)
	_, sid := a.sunChili()
	path := fmt.Sprintf("/api/cook-sessions/%d", sid)

	code, body := a.do(http.MethodGet, path+"/leftover-cells?count=2", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []any{
		map[string]any{"date": "2026-11-02", "slot_id": float64(a.lunch)},
		map[string]any{"date": "2026-11-02", "slot_id": float64(a.dinner)},
	}, body["cells"])

	code, body = a.do(http.MethodPost, path+"/leftovers",
		fmt.Sprintf(`{"cells":[{"date":"2026-11-02","slot_id":%d},{"date":"2026-11-02","slot_id":%d}]}`, a.lunch, a.dinner))
	require.Equal(t, http.StatusOK, code, body)
	assert.EqualValues(t, 6, body["portions"])
	assert.EqualValues(t, 3, body["meals"])

	code, body = a.do(http.MethodPost, path+"/leftovers", fmt.Sprintf(`{"cells":[{"date":"2026-11-01","slot_id":%d}]}`, a.lunch))
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.session.before_cook", body["message_key"])
}

func TestCookSessionsHTTP_LinkErrors(t *testing.T) {
	a := newPlanAPI(t)
	_, sid := a.sunChili()
	code, body := a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2026-11-02","slot_id":%d}`, a.lunch))
	require.Equal(t, http.StatusCreated, code)
	monLunch := int64(body["id"].(float64))
	comps := fmt.Sprintf("/api/plates/%d/components", monLunch)

	code, body = a.do(http.MethodPost, comps, fmt.Sprintf(`{"food_id":%d,"cook_session_id":%d}`, a.curry, sid))
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.session.food_mismatch", body["message_key"])

	code, body = a.do(http.MethodPost, comps, fmt.Sprintf(`{"food_id":%d,"cook_session_id":999}`, a.chili))
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.session.not_found", body["message_key"])

	code, body = a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2026-11-06","slot_id":%d,"components":[{"food_id":%d,"cook_session_id":%d}]}`, a.lunch, a.chili, sid))
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.session.past_keep_days", body["message_key"])

	code, body = a.do(http.MethodPost, comps, fmt.Sprintf(`{"food_id":%d,"cook_session_id":%d}`, a.chili, sid))
	require.Equal(t, http.StatusCreated, code, body)
	comp := body["components"].([]any)[0].(map[string]any)
	assert.EqualValues(t, sid, comp["cook_session_id"])
}

func TestCookSessionsHTTP_BadRequests(t *testing.T) {
	a := newPlanAPI(t)
	_, sid := a.sunChili()
	path := fmt.Sprintf("/api/cook-sessions/%d", sid)
	tests := []struct {
		name, method, path, body string
		status                   int
		key                      string
	}{
		{"non-numeric id", http.MethodGet, "/api/cook-sessions/abc", "", http.StatusBadRequest, "error.invalid_id"},
		{"unknown session", http.MethodGet, "/api/cook-sessions/999", "", http.StatusNotFound, "error.not_found"},
		{"put without portions", http.MethodPut, path, `{"frozen_portions":0}`, http.StatusBadRequest, "error.invalid_body"},
		{"put malformed", http.MethodPut, path, `{`, http.StatusBadRequest, "error.invalid_body"},
		{"put zero portions", http.MethodPut, path, `{"portions":0}`, http.StatusBadRequest, "error.invalid_body"},
		{"cooked without flag", http.MethodPost, path + "/cooked", `{}`, http.StatusBadRequest, "error.invalid_body"},
		{"cells without count", http.MethodGet, path + "/leftover-cells", "", http.StatusBadRequest, "error.invalid_body"},
		{"cells with zero count", http.MethodGet, path + "/leftover-cells?count=0", "", http.StatusBadRequest, "error.invalid_body"},
		{"leftovers bad date", http.MethodPost, path + "/leftovers", `{"cells":[{"date":"02.11.","slot_id":1}]}`, http.StatusBadRequest, "error.invalid_body"},
		{"leftovers empty", http.MethodPost, path + "/leftovers", `{"cells":[]}`, http.StatusBadRequest, "error.invalid_body"},
		{"delete unknown", http.MethodDelete, "/api/cook-sessions/999", "", http.StatusNotFound, "error.not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := a.do(tt.method, tt.path, tt.body)
			assert.Equal(t, tt.status, code)
			assert.Equal(t, tt.key, body["message_key"])
		})
	}
}

func TestCookSessionsHTTP_DeleteAndRestore(t *testing.T) {
	a := newPlanAPI(t)
	_, sid := a.sunChili()

	code, body := a.do(http.MethodDelete, fmt.Sprintf("/api/cook-sessions/%d", sid), "")
	require.Equal(t, http.StatusOK, code, body)
	removed := body["removed"].(map[string]any)
	require.Len(t, removed["plates"].([]any), 1)
	require.Len(t, removed["sessions"].([]any), 1)

	snapshot, err := json.Marshal(removed)
	require.NoError(t, err)
	code, body = a.do(http.MethodPost, "/api/plan/restore", string(snapshot))
	require.Equal(t, http.StatusOK, code, body)
	assert.Len(t, body["plate_ids"].([]any), 1)
}

// DELETE /api/plates/{id} answers 200 {removed}; restoring it twice hits the
// now-occupied cell (B1-AC11 over HTTP).
func TestPlanWriteHTTP_DeletePlateAndRestore(t *testing.T) {
	a := newPlanAPI(t)
	plateID, _ := a.sunChili()

	code, body := a.do(http.MethodDelete, fmt.Sprintf("/api/plates/%d", plateID), "")
	require.Equal(t, http.StatusOK, code, body)
	removed := body["removed"].(map[string]any)
	plates := removed["plates"].([]any)
	require.Len(t, plates, 1)
	p := plates[0].(map[string]any)
	assert.Equal(t, "2026-11-01", p["date"])
	assert.EqualValues(t, 2, p["eaters"])
	comp := p["components"].([]any)[0].(map[string]any)
	assert.Equal(t, "s1", comp["session"].(map[string]any)["ref"])

	snapshot, err := json.Marshal(removed)
	require.NoError(t, err)
	code, body = a.do(http.MethodPost, "/api/plan/restore", string(snapshot))
	require.Equal(t, http.StatusOK, code, body)
	code, body = a.do(http.MethodPost, "/api/plan/restore", string(snapshot))
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.plan.cell_occupied", body["message_key"])
	var n int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM cook_sessions`).Scan(&n))
	assert.Equal(t, 1, n, "the failed restore wrote nothing")
}

func TestPlanWriteHTTP_ClearRange(t *testing.T) {
	a := newPlanAPI(t)
	a.sunChili()

	code, body := a.do(http.MethodDelete, "/api/plan?from=2026-11-01&to=2026-11-07", "")
	require.Equal(t, http.StatusOK, code, body)
	removed := body["removed"].(map[string]any)
	assert.Len(t, removed["plates"].([]any), 1)
	assert.Len(t, removed["sessions"].([]any), 1)

	code, body = a.do(http.MethodDelete, "/api/plan?from=2026-11-01&to=2026-11-07", "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"plates": []any{}, "sessions": []any{}}, body["removed"], "an empty range removes nothing")

	for _, q := range []string{"", "?from=2026-11-01", "?from=x&to=2026-11-07", "?from=2026-11-07&to=2026-11-01"} {
		code, body = a.do(http.MethodDelete, "/api/plan"+q, "")
		assert.Equal(t, http.StatusBadRequest, code, q)
		assert.Equal(t, "error.invalid_body", body["message_key"], q)
	}
}

func TestPlanWriteHTTP_RestoreErrors(t *testing.T) {
	a := newPlanAPI(t)
	code, body := a.do(http.MethodPost, "/api/plan/restore", `{`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "error.invalid_body", body["message_key"])

	snap := fmt.Sprintf(`{"plates":[{"date":"2026-11-02","slot_id":%d,"eaters":2,"components":[{"food_id":%d,"portions":1,"session":{"id":999}}]}],"sessions":[]}`, a.lunch, a.chili)
	code, body = a.do(http.MethodPost, "/api/plan/restore", snap)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.session.not_found", body["message_key"])
}
