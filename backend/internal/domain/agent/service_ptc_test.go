package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// ptcScript is a programmatic exchange: the code (server tool use srv1) calls
// find_foods, the reply pauses, and the final reply carries the code's result.
func ptcScript() *fake.Script {
	paused := fake.Turn{
		StopReason: "tool_use", ContainerID: "c1", ContainerExpiresAt: time.Now().Add(time.Hour),
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "server", Raw: json.RawMessage(`{"type":"server_tool_use","id":"srv1","name":"code_execution","input":{"code":"r = await find_foods({})\nprint('ok')"}}`)},
			{
				Type: "tool_use", ToolUseID: "tu1", ToolName: "find_foods", ToolInput: json.RawMessage(`{}`),
				Caller: &llm.Caller{Type: llm.CallerCodeExecution, ToolID: "srv1"},
			},
		}},
		Usage: fake.ScriptedUsage{InputTokens: 100, OutputTokens: 10},
	}
	final := fake.Turn{
		StopReason: "end_turn", ContainerID: "c1", ContainerExpiresAt: time.Now().Add(time.Hour),
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "server", Raw: json.RawMessage(`{"type":"code_execution_tool_result","tool_use_id":"srv1","content":{"type":"code_execution_result","stdout":"ok\n","stderr":"","return_code":0,"content":[]}}`)},
			{Type: "text", Text: "Done."},
		}},
		Usage: fake.ScriptedUsage{InputTokens: 120, OutputTokens: 12},
	}
	return &fake.Script{Turns: []fake.Turn{paused, final}}
}

func TestChat_ProgrammaticRunIsLogged(t *testing.T) {
	svc, db := newChatService(t, ptcScript())
	_, err := chat(context.Background(), svc, "plan", nil)
	require.NoError(t, err)

	var runID int64
	var container sql.NullString
	require.NoError(t, db.QueryRow(`SELECT id, container_id FROM ai_runs`).Scan(&runID, &container))
	assert.Equal(t, "c1", container.String)

	rows, err := db.Query(`SELECT type, name, caller, caller_tool_id, args_json, result_json, request_json
		FROM ai_run_steps WHERE run_id = ? ORDER BY seq`, runID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	type step struct {
		typ, name                             string
		caller, callerTool, args, result, req sql.NullString
	}
	var steps []step
	for rows.Next() {
		var s step
		require.NoError(t, rows.Scan(&s.typ, &s.name, &s.caller, &s.callerTool, &s.args, &s.result, &s.req))
		steps = append(steps, s)
	}
	require.NoError(t, rows.Err())

	var tool, code *step
	var llmCalls []step
	for i := range steps {
		switch {
		case steps[i].typ == "tool_call" && steps[i].name == "find_foods":
			tool = &steps[i]
		case steps[i].typ == "code_execution":
			code = &steps[i]
		case steps[i].typ == "llm_call":
			llmCalls = append(llmCalls, steps[i])
		}
	}
	require.NotNil(t, tool)
	assert.Equal(t, "code_execution", tool.caller.String)
	assert.Equal(t, "srv1", tool.callerTool.String)
	assert.NotEmpty(t, tool.result.String, "the code-called result is only stored on its step")

	require.NotNil(t, code)
	assert.Contains(t, code.args.String, "find_foods")
	assert.Contains(t, code.result.String, `"stdout":"ok\n"`)
	assert.Contains(t, code.result.String, `"container_id":"c1"`)
	assert.Contains(t, code.result.String, `"return_code":0`)

	assert.Len(t, llmCalls, 2)
}

func TestChat_ContainerSavedAndReusedNextTurn(t *testing.T) {
	svc, db := newChatService(t, ptcScript())
	var convID int64
	_, err := chat(context.Background(), svc, "plan", func(e llm.Event) {
		if p, ok := e.Payload.(llm.ConversationReadyPayload); ok {
			convID = p.ConversationID
		}
	})
	require.NoError(t, err)

	var id sql.NullString
	require.NoError(t, db.QueryRow(`SELECT container_id FROM ai_conversations WHERE id = ?`, convID).Scan(&id))
	assert.Equal(t, "c1", id.String)
}

func TestChat_ProgrammaticToolsOffSendsNoCodeTool(t *testing.T) {
	script := &fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{InputTokens: 1, OutputTokens: 1})}}
	client := fake.NewFromScript(script)
	svc, db := newChatServiceWithClient(t, client)
	require.NoError(t, testsvc.Settings(db).Set(context.Background(), settings.KeyAIProgrammaticTools, "false"))
	_, err := chat(context.Background(), svc, "hi", nil)
	require.NoError(t, err)

	reqs := client.Requests()
	require.Len(t, reqs, 1)
	assert.False(t, reqs[0].CodeExecution)
	for _, tl := range reqs[0].Tools {
		assert.Empty(t, tl.AllowedCallers, tl.Name)
	}

	// On (the default) it opts in.
	client2 := fake.NewFromScript(script)
	svc2, _ := newChatServiceWithClient(t, client2)
	_, err = chat(context.Background(), svc2, "hi", nil)
	require.NoError(t, err)
	assert.True(t, client2.Requests()[0].CodeExecution)
}
