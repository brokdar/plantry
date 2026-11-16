package planning_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// A call really cut at max_tokens holds partial JSON as a string. The run
// keeps going: the next request replays the call with an object input, the
// answered step keeps the raw text and is an error step, and the run note
// appears once however often replies are cut.
func TestLLMChooser_CutInputReplaysAsObjectAndIsLogged(t *testing.T) {
	e := newLLMEnv(t)
	cut := fake.Turn{StopReason: "max_tokens", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
		{Type: "tool_use", ToolUseID: "tu_cut", ToolName: "evaluate_plan", ToolInput: json.RawMessage(`"{\"sessions\": [{\"key\": \"a"`)},
	}}}
	id := e.gen(cut, cut, submit(t, "tu_1", e.validPlan()))

	row := e.run(id)
	assert.Equal(t, "ok", row.Status)
	assert.Equal(t, 1, strings.Count(row.Notes, "cut off at the output limit"))

	reqs := e.client.Requests()
	require.Len(t, reqs, 3)
	for _, req := range reqs[1:] {
		var replayed bool
		for _, m := range req.Messages {
			for _, b := range m.Content {
				if b.Type == llm.ContentTypeToolUse && b.ToolUseID == "tu_cut" {
					replayed = true
					assert.JSONEq(t, `{}`, string(b.ToolUseInput))
				}
			}
		}
		assert.True(t, replayed)
	}

	rows, err := e.db.Query(`SELECT args_json, result_json, is_error FROM ai_run_steps WHERE run_id = ? AND type = 'tool_call' AND name = 'evaluate_plan' ORDER BY seq`, id)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var args, result string
		var isErr bool
		require.NoError(t, rows.Scan(&args, &result, &isErr))
		n++
		assert.True(t, isErr)
		assert.Equal(t, `"{\"sessions\": [{\"key\": \"a"`, args)
		assert.Contains(t, result, `"error":"Your reply was cut off`)
	}
	assert.Equal(t, 2, n)
}
