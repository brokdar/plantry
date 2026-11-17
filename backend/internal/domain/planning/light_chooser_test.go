package planning_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withLight adds a `light` list to a plan's submit_plan input.
func withLightInput(t *testing.T, p *jsonPlan, light ...map[string]string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	m["light"] = light
	out, err := json.Marshal(m)
	require.NoError(t, err)
	return out
}

// A light request stated in the first submit_plan stands for the whole run:
// the repair that does not repeat it is still evaluated under it, and every
// plan.evaluation step of the run records it.
func TestLLMChooser_LightRequestStandsForTheRun(t *testing.T) {
	e := newLLMEnv(t)
	mon := e.start.AddDate(0, 0, 2).Format(time.DateOnly)
	first := submitRaw("tu_1", withLightInput(t, e.h2Plan(), map[string]string{"date": mon, "scope": "day"}))
	id := e.gen(first, submit(t, "tu_2", e.validPlan()))
	require.Equal(t, "ok", e.run(id).Status)

	rows, err := e.db.Query(`SELECT args_json, result_json FROM ai_run_steps WHERE run_id = ? AND name = 'plan.evaluation' ORDER BY seq`, id)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var args, result string
		require.NoError(t, rows.Scan(&args, &result))
		assert.Contains(t, result, `"light":[{"date":"`+mon+`","scope":"day"}]`, args)
		n++
	}
	assert.GreaterOrEqual(t, n, 3, "two attempts and the saved plan")
}

func TestLLMChooser_LightRequestIsValidated(t *testing.T) {
	e := newLLMEnv(t)
	bad := submitRaw("tu_1", withLightInput(t, e.validPlan(), map[string]string{"date": "2026-11-09", "scope": "meal"}))
	id := e.gen(bad, submit(t, "tu_2", e.validPlan()))
	assert.Equal(t, "ok", e.run(id).Status)
	assert.Equal(t, 1, e.run(id).RepairAttempts, "a meal request without a slot is sent back like any unreadable reply")
}
