package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A sole {"params": {...}} wrapper is unwrapped for every tool (audit B7).
func TestExecute_UnwrapsASoleParamsObject(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	out, _, _, err := e.ts.Execute(e.ctx, "get_plan", json.RawMessage(`{"params":{"from":"2026-11-07","to":"2026-11-08"}}`))
	require.NoError(t, err)
	var res struct {
		Cells []struct {
			Date string `json:"date"`
		} `json:"cells"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	require.NotEmpty(t, res.Cells)
	for _, c := range res.Cells {
		assert.LessOrEqual(t, c.Date, "2026-11-08", "the range inside params applied")
	}

	_, _, _, err = e.ts.Execute(e.ctx, "get_plan", json.RawMessage(`{"params":{"from":"2026-11-07"},"to":"2026-11-08"}`))
	assert.Error(t, err, "params next to other fields is not a wrapper")
}
