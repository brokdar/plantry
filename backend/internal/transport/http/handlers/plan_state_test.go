package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Phase 3 §3.1: state endpoints and the state fields of plates and sessions.

func ids(v any) []int64 {
	out := []int64{}
	for _, x := range v.([]any) {
		out = append(out, int64(x.(float64)))
	}
	return out
}

func TestPlanStateHTTP_ConfirmUnlockAndFields(t *testing.T) {
	a := newPlanAPI(t)
	pid, sid := a.sunChili()

	_, p := a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", pid), "")
	assert.Equal(t, "confirmed", p["state"], "the user's plate is confirmed")
	assert.Contains(t, p, "ai_run_id")
	assert.Nil(t, p["ai_run_id"])
	assert.Contains(t, p, "reason")
	assert.Nil(t, p["reason"])
	_, s := a.do(http.MethodGet, fmt.Sprintf("/api/cook-sessions/%d", sid), "")
	assert.Equal(t, "confirmed", s["state"])
	assert.Contains(t, s, "ai_run_id")
	assert.Contains(t, s, "reason")

	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/unlock", pid), "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []int64{pid}, ids(body["plate_ids"]))
	assert.Equal(t, []int64{sid}, ids(body["session_ids"]))
	_, p = a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", pid), "")
	assert.Equal(t, "draft", p["state"])

	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/confirm", sid), "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []int64{pid}, ids(body["plate_ids"]))
	_, s = a.do(http.MethodGet, fmt.Sprintf("/api/cook-sessions/%d", sid), "")
	assert.Equal(t, "confirmed", s["state"])

	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/unlock", sid), "")
	require.Equal(t, http.StatusOK, code, body)
	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/confirm", pid), "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []int64{sid}, ids(body["session_ids"]))

	// The plan read model carries the state too.
	code, plan := a.do(http.MethodGet, "/api/plan?start=2026-10-26&today=2026-10-31", "")
	require.Equal(t, http.StatusOK, code, plan)
	assert.Equal(t, "confirmed", plan["sessions"].([]any)[0].(map[string]any)["state"])
}

func TestPlanStateHTTP_AcceptUnacceptDiscardRestore(t *testing.T) {
	a := newPlanAPI(t)
	pid, sid := a.sunChili()
	window := `{"from":"2026-11-01","to":"2026-11-07"}`

	code, body := a.do(http.MethodPost, "/api/plan/accept", window)
	require.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "error.plan.nothing_to_accept", body["message_key"])
	code, body = a.do(http.MethodPost, "/api/plan/discard", window)
	require.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "error.plan.nothing_to_accept", body["message_key"])

	a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/unlock", pid), "")
	code, body = a.do(http.MethodPost, "/api/plan/unaccept", fmt.Sprintf(`{"plate_ids":[%d],"session_ids":[%d]}`, pid, sid))
	require.Equal(t, http.StatusConflict, code, "drafts can't be un-accepted")
	assert.Equal(t, "error.plan.state_mismatch", body["message_key"])

	code, body = a.do(http.MethodPost, "/api/plan/accept", window)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []int64{pid}, ids(body["plate_ids"]))
	assert.Equal(t, []int64{sid}, ids(body["session_ids"]))

	code, body = a.do(http.MethodPost, "/api/plan/unaccept", fmt.Sprintf(`{"plate_ids":[%d],"session_ids":[%d]}`, pid, sid))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []int64{pid}, ids(body["plate_ids"]))
	_, p := a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", pid), "")
	assert.Equal(t, "draft", p["state"])

	code, body = a.do(http.MethodPost, "/api/plan/discard", window)
	require.Equal(t, http.StatusOK, code, body)
	removed := body["removed"].(map[string]any)
	plates := removed["plates"].([]any)
	require.Len(t, plates, 1)
	assert.Equal(t, "draft", plates[0].(map[string]any)["state"])
	assert.Equal(t, "draft", removed["sessions"].([]any)[0].(map[string]any)["state"])
	code, _ = a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", pid), "")
	assert.Equal(t, http.StatusNotFound, code)

	snap, err := json.Marshal(removed)
	require.NoError(t, err)
	code, body = a.do(http.MethodPost, "/api/plan/restore", string(snap))
	require.Equal(t, http.StatusOK, code, body)
	restored := ids(body["plate_ids"])
	_, p = a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", restored[0]), "")
	assert.Equal(t, "draft", p["state"], "undo of a discard brings back a draft")
}

func TestPlanStateHTTP_BadRequests(t *testing.T) {
	a := newPlanAPI(t)
	tests := []struct {
		name, method, path, body string
		status                   int
		key                      string
	}{
		{"accept malformed", http.MethodPost, "/api/plan/accept", `{`, http.StatusBadRequest, "error.invalid_body"},
		{"accept bad date", http.MethodPost, "/api/plan/accept", `{"from":"x","to":"2026-11-07"}`, http.StatusBadRequest, "error.invalid_body"},
		{"accept reversed", http.MethodPost, "/api/plan/accept", `{"from":"2026-11-07","to":"2026-11-01"}`, http.StatusBadRequest, "error.invalid_body"},
		{"discard missing to", http.MethodPost, "/api/plan/discard", `{"from":"2026-11-01"}`, http.StatusBadRequest, "error.invalid_body"},
		{"unaccept malformed", http.MethodPost, "/api/plan/unaccept", `[]`, http.StatusBadRequest, "error.invalid_body"},
		{"unaccept empty", http.MethodPost, "/api/plan/unaccept", `{"plate_ids":[],"session_ids":[]}`, http.StatusBadRequest, "error.invalid_body"},
		{"unaccept unknown", http.MethodPost, "/api/plan/unaccept", `{"plate_ids":[999]}`, http.StatusConflict, "error.plan.state_mismatch"},
		{"confirm unknown plate", http.MethodPost, "/api/plates/999/confirm", "", http.StatusNotFound, "error.not_found"},
		{"unlock unknown session", http.MethodPost, "/api/cook-sessions/999/unlock", "", http.StatusNotFound, "error.not_found"},
		{"confirm non-numeric", http.MethodPost, "/api/plates/abc/confirm", "", http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := a.do(tt.method, tt.path, tt.body)
			assert.Equal(t, tt.status, code, body)
			if tt.key != "" {
				assert.Equal(t, tt.key, body["message_key"])
			}
		})
	}
}
