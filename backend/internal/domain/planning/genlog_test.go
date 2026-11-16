package planning_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// Every generation (heuristic runs too) logs its stage transitions with the
// run id and the elapsed time, and a final line with the outcome.
func TestService_LogsStageTransitions(t *testing.T) {
	e := newSvcEnv(t)
	var buf bytes.Buffer
	e.deps.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
	e.svc = planning.NewService(e.deps)

	id := e.generate(e.req())
	e.ok(id)

	var stages []string
	var done, saving, checking map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), line)
		if m["msg"] != "plan.run.stage" {
			continue
		}
		assert.EqualValues(t, id, m["run_id"])
		assert.Contains(t, m, "elapsed_ms")
		st, _ := m["stage"].(string)
		stages = append(stages, st)
		switch st {
		case "done":
			done = m
		case "saving":
			saving = m
		case "checking":
			checking = m
		}
	}
	assert.Equal(t, []string{"resolving", "choosing", "checking", "saving", "done"}, stages)
	require.NotNil(t, checking)
	assert.Contains(t, checking, "violations")
	assert.Contains(t, checking, "by_rule")
	require.NotNil(t, saving)
	assert.Positive(t, saving["plates"])
	assert.Contains(t, saving, "proposal_sessions")
	assert.Contains(t, saving, "routine_plates")
	require.NotNil(t, done)
	assert.Contains(t, []any{"ok", "partial"}, done["status"])
	assert.NotEmpty(t, done["score"])
}
