package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Slice 3b: the day regenerate's preview and the run's undo.
func TestPlanGenerateHTTP_RegeneratePreviewAndRunUndo(t *testing.T) {
	a := newGenAPI(t)
	_, gen := a.do(http.MethodPost, "/api/plan/generate"+genToday, `{"start":"2026-11-07"}`)
	a.drain()
	_, run := a.do(http.MethodGet, fmt.Sprintf("/api/plan/runs/%.0f", gen["run_id"]), "")
	assert.NotContains(t, run, "undo", "generate has no undo")

	const preview = "/api/plan/regenerate/preview?start=2026-11-07&today=2026-11-06"
	for _, tc := range []struct {
		name, query string
		key         string
	}{
		{"scope missing", "&date=2026-11-10", "error.plan.invalid_scope"},
		{"other scope", "&scope=plate&date=2026-11-10", "error.plan.invalid_scope"},
		{"date missing", "&scope=day", "error.plan.invalid_scope"},
		{"date outside the window", "&scope=day&date=2026-11-14", "error.plan.invalid_scope"},
		{"past date", "&scope=day&date=2026-11-05", "error.plan.invalid_scope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := a.do(http.MethodGet, preview+tc.query, "")
			assert.Equal(t, http.StatusBadRequest, code, body)
			assert.Equal(t, tc.key, body["message_key"])
		})
	}
	code, body := a.do(http.MethodGet, "/api/plan/regenerate/preview?scope=day&date=2026-11-10&start=2026-11-08&today=2026-11-06", "")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "error.plan.invalid_start", body["message_key"])

	code, pv := a.do(http.MethodGet, preview+"&scope=day&date=2026-11-10", "")
	require.Equal(t, http.StatusOK, code, pv)
	assert.Len(t, pv["replaced"], 2, "Tuesday's lunch and dinner")
	assert.IsType(t, []any{}, pv["also_changed"])
	assert.Equal(t, []any{}, pv["confirmed_affected"])

	code, body = a.do(http.MethodPost, "/api/plan/regenerate"+genToday, `{"start":"2026-11-07","scope":{"kind":"day","date":"2026-11-10"}}`)
	require.Equal(t, http.StatusAccepted, code, body)
	runPath := fmt.Sprintf("/api/plan/runs/%.0f", body["run_id"])
	_, run = a.do(http.MethodGet, runPath, "")
	require.Equal(t, "running", run["status"])
	assert.NotContains(t, run, "undo", "no undo while the run runs")

	a.drain()

	_, run = a.do(http.MethodGet, runPath, "")
	require.Contains(t, []any{"ok", "partial"}, run["status"], run)
	undo, ok := run["undo"].(map[string]any)
	require.True(t, ok, "a finished day regenerate carries its undo: %v", run)
	assert.Positive(t, undo["first_event_id"])
	assert.GreaterOrEqual(t, undo["last_event_id"], undo["first_event_id"])
}
