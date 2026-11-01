package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// Phase 4 B5: the conversation lists its changes; POST
// /api/ai/changes/{id}/undo undoes one (D13). The undo paths themselves are
// covered in domain/agent/changes_test.go.

func TestAIChanges_ConversationAndUndo(t *testing.T) {
	db := testhelper.NewTestDB(t)
	h, _ := newAIHandlerWithPlan(t, db, agent.PlanContext{
		Build: testsvc.PlanningDeps(db).Builder.Build, Summary: planning.RenderPlanSummary,
	})
	mustExecDB := func(q string, args ...any) {
		t.Helper()
		_, err := db.Exec(q, args...)
		require.NoError(t, err, q)
	}
	mustExecDB(`INSERT INTO ai_conversations (id, title) VALUES (5, 'Chili')`)
	mustExecDB(`INSERT INTO ai_runs (id, kind, started_at, conversation_id) VALUES (9, 'chat', '2026-11-01 10:00:00', 5)`)
	mustExecDB(`INSERT INTO ai_changes (id, run_id, step_seq, tool, text_key, params_json, run_ids_json, dates_json, plate_ids_json, session_ids_json)
		VALUES (1, 9, 2, 'set_plate_eaters', 'set_plate_eaters', '{"name":"Chili","eaters":3}', '[9]', '["2026-11-07"]', '[4]', '[]')`)
	mustExecDB(`INSERT INTO ai_changes (id, run_id, step_seq, tool, text_key, params_json, run_ids_json, dates_json, plate_ids_json, session_ids_json, undone_at)
		VALUES (2, 9, 3, 'confirm', 'confirm', '{"count":1}', '[9]', '[]', '[]', '[]', '2026-11-01 10:05:00')`)

	r := chi.NewRouter()
	r.Get("/api/ai/conversations/{id}", h.GetConversation)
	r.Post("/api/ai/changes/{id}/undo", h.UndoChange)
	do := func(method, url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, url, nil))
		return w
	}

	// B5-AC5
	w := do(http.MethodGet, "/api/ai/conversations/5")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var conv struct {
		Changes []map[string]any `json:"changes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &conv))
	require.Len(t, conv.Changes, 2)
	first := conv.Changes[0]
	assert.Equal(t, 1.0, first["id"])
	assert.Equal(t, 9.0, first["run_id"])
	assert.Equal(t, "set_plate_eaters", first["text_key"])
	assert.Equal(t, map[string]any{"name": "Chili", "eaters": 3.0}, first["params"])
	assert.Equal(t, []any{"2026-11-07"}, first["dates"])
	assert.Equal(t, []any{4.0}, first["plate_ids"])
	assert.Equal(t, []any{}, first["session_ids"])
	assert.Equal(t, false, first["undoable"], "no event, no cell: nothing to undo")
	assert.Nil(t, first["undone_at"])
	assert.Contains(t, first, "undone_at")
	assert.Equal(t, "2026-11-01T10:05:00Z", conv.Changes[1]["undone_at"])

	// Undo: the shape, then the second undo, an unknown and a bad id.
	w = do(http.MethodPost, "/api/ai/changes/1/undo")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"plate_ids":[],"session_ids":[]}`, w.Body.String())
	w = do(http.MethodPost, "/api/ai/changes/1/undo")
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "error.ai.change_undone", messageKey(t, w))
	w = do(http.MethodPost, "/api/ai/changes/77/undo")
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = do(http.MethodPost, "/api/ai/changes/abc/undo")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = do(http.MethodGet, "/api/ai/conversations/5")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &conv))
	assert.NotNil(t, conv.Changes[0]["undone_at"])
}
