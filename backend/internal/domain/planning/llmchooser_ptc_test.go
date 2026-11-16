package planning_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// programmaticEnv is newLLMEnv with the chooser opted into programmatic tool
// calling (the setting on).
func programmaticEnv(t *testing.T, on bool) *llmEnv {
	t.Helper()
	e := newLLMEnv(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(e.db), slog.New(slog.DiscardHandler), time.Now)
	t.Cleanup(rec.Wait)
	e.deps.RunLog = planning.RecorderLog{Recorder: rec}
	e.deps.Choosers[airun.ChooserLLM] = planning.NewLLMChooser(llm.StaticResolver(rec.Meter(e.client, "fake"), "fake-model")).
		WithProgrammatic(func(context.Context) bool { return on })
	e.svc = planning.NewService(e.deps)
	return e
}

// codeEvaluateTurn is a reply that paused on evaluate_plan calls made by code.
func codeEvaluateTurn(t *testing.T, plan any, n int) fake.Turn {
	t.Helper()
	raw, err := json.Marshal(plan)
	require.NoError(t, err)
	blocks := []fake.ScriptedBlock{{
		Type: "server",
		Raw:  json.RawMessage(`{"type":"server_tool_use","id":"srv1","name":"code_execution","input":{"code":"for d in drafts: await evaluate_plan(d)"}}`),
	}}
	for i := range n {
		blocks = append(blocks, fake.ScriptedBlock{
			Type: "tool_use", ToolUseID: "tu" + string(rune('a'+i)), ToolName: "evaluate_plan", ToolInput: raw,
			Caller: &llm.Caller{Type: llm.CallerCodeExecution, ToolID: "srv1"},
		})
	}
	return fake.Turn{
		StopReason: "tool_use", ContainerID: "c1", ContainerExpiresAt: time.Now().Add(time.Hour),
		Message: fake.ScriptedMessage{Content: blocks},
		Usage:   fake.ScriptedUsage{InputTokens: 1000, OutputTokens: 200},
	}
}

func TestLLMChooser_ProgrammaticEvaluations(t *testing.T) {
	e := programmaticEnv(t, true)
	plan := e.validPlan()
	id := e.gen(codeEvaluateTurn(t, plan, 2), submit(t, "sub", plan))

	assert.Equal(t, "ok", e.run(id).Status)
	reqs := e.client.Requests()
	require.Len(t, reqs, 2)

	assert.True(t, reqs[0].CodeExecution)
	var evalCallers, submitCallers []string
	for _, tl := range reqs[0].Tools {
		switch tl.Name {
		case "evaluate_plan":
			evalCallers = tl.AllowedCallers
		case "submit_plan":
			submitCallers = tl.AllowedCallers
		}
	}
	assert.Equal(t, []string{llm.CallerDirect, llm.CallerCodeExecution}, evalCallers)
	assert.Empty(t, submitCallers, "writes stay direct")

	assert.Equal(t, "c1", reqs[1].Container, "the continuation carries the container")
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	require.Len(t, last.Content, 2, "only tool_result blocks answer the programmatic calls")
	for _, b := range last.Content {
		assert.Equal(t, llm.ContentTypeToolResult, b.Type)
		assert.False(t, b.ToolResultIsError)
		assert.True(t, json.Valid(b.ToolResultContent))
		assert.Contains(t, string(b.ToolResultContent), `"days"`)
	}

	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM ai_run_steps WHERE run_id = ? AND name = 'plan.evaluation' AND caller = 'code_execution' AND caller_tool_id = 'srv1'`, id).Scan(&n))
	assert.Equal(t, 2, n, "each code-called evaluation is a step with its caller")
	var container string
	require.NoError(t, e.db.QueryRow(`SELECT container_id FROM ai_runs WHERE id = ?`, id).Scan(&container))
	assert.Equal(t, "c1", container)
}

func TestLLMChooser_ProgrammaticOffSendsNoCodeTool(t *testing.T) {
	e := programmaticEnv(t, false)
	id := e.gen(submit(t, "sub", e.validPlan()))
	assert.Equal(t, "ok", e.run(id).Status)

	req := e.client.Requests()[0]
	assert.False(t, req.CodeExecution)
	for _, tl := range req.Tools {
		assert.Empty(t, tl.AllowedCallers, tl.Name)
	}
}
