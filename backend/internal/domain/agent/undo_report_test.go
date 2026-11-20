package agent_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E030: the report of an undo has nothing new or worse and no day status: the
// plan is back where it was before the undone change.
func TestUndoReport_NothingNewAfterAnUndo(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	ctx, conv := e.chat()
	_, p := e.laterMealSession()
	away := fmt.Sprintf(`{"op":"away","cells":[%s]}`, cellJSON(p.DateString(), e.slotID("dinner")))
	e.change(ctx, "apply_changes", `{"ops":[`+away+`]}`)
	res := e.undoLast(ctx, conv, `{}`)
	rep, ok := res["report"].(map[string]any)
	require.True(t, ok, "an undo carries a report")
	assert.Contains(t, rep["note"], "Undone")
	assert.NotContains(t, rep, "new_warnings")
	assert.NotContains(t, rep, "worse")
	for _, d := range rep["days"].([]any) {
		assert.NotContains(t, d, "status")
	}
}
