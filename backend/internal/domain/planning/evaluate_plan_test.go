package planning_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// evaluate_plan is a dry run: it is answered with the evaluation, is not an
// attempt and uses no repair; the plan that follows is submitted as usual.
func TestLLMChooser_EvaluatePlanLoop(t *testing.T) {
	in := run4Input()
	k := 2200.0
	in.State.Profile.KcalTarget = &k
	p := validWeek(t, in)
	eval := submit(t, "tu_e", p)
	eval.Message.Content[0].ToolName = "evaluate_plan"

	prop, reqs := chooseWith(t, in, eval, eval, submit(t, "tu_s", p))

	require.Len(t, reqs, 3, "two dry runs and the submit")
	assert.Zero(t, prop.RepairAttempts, "a dry run is not a repair")
	last := reqs[2].Messages[len(reqs[2].Messages)-1]
	require.Equal(t, llm.RoleUser, last.Role)
	require.Equal(t, llm.ContentTypeToolResult, last.Content[0].Type)
	assert.False(t, last.Content[0].ToolResultIsError)
	var ev struct {
		Days   []map[string]any `json:"days"`
		Issues []struct {
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
		} `json:"issues"`
	}
	require.NoError(t, json.Unmarshal(last.Content[0].ToolResultContent, &ev))
	assert.NotEmpty(t, ev.Days)
	var soft bool
	for _, is := range ev.Issues {
		soft = soft || (is.Rule == "S2" && is.Severity == "soft")
	}
	assert.True(t, soft, "the day kcal gap is reported as a soft issue: %s", last.Content[0].ToolResultContent)
}
