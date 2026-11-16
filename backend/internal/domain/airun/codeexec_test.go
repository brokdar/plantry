package airun_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// reporting is a fake client that says what it sent.
type reporting struct {
	llm.Client
	eff llm.Effective
}

func (r reporting) EffectiveParams(llm.Request) llm.Effective { return r.eff }

func TestMeter_RecordsCodeExecutionStepsAndSentParams(t *testing.T) {
	paused := fake.Turn{
		StopReason: "tool_use", ContainerID: "c1",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "server", Raw: json.RawMessage(`{"type":"server_tool_use","id":"srv1","name":"code_execution","input":{"code":"print(2)"}}`)},
		}},
	}
	done := fake.Turn{
		StopReason: "end_turn", ContainerID: "c1",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "server", Raw: json.RawMessage(`{"type":"code_execution_tool_result","tool_use_id":"srv1","content":{"type":"code_execution_result","stdout":"2\n","stderr":"warn","return_code":3,"content":[]}}`)},
			{Type: "text", Text: "ok"},
		}},
	}
	inner := reporting{
		Client: fake.NewFromScript(&fake.Script{Turns: []fake.Turn{paused, done}}),
		eff:    llm.Effective{CodeExecution: true, CodeTools: []string{"find_foods"}, Container: "c0"},
	}
	store := newMemStore()
	rec, logs := newRecorderWithClock(t, store, steppingClock(time.Second))
	client := rec.Meter(inner, "fake")

	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	req := llm.Request{Model: "m", CodeExecution: true}
	_, err := drainStream(ctx, client, req)
	require.NoError(t, err)
	// The tool the code called, then the continuation that carries the result.
	run.ToolStep(llm.WithCaller(ctx, &llm.Caller{Type: llm.CallerCodeExecution, ToolID: "srv1"}), "find_foods", json.RawMessage(`{}`), json.RawMessage(`{"items":[]}`), false, time.Millisecond)
	_, err = drainStream(ctx, client, req)
	require.NoError(t, err)
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, steps, _ := store.snapshot()
	require.Len(t, finishes, 1)
	assert.Equal(t, "c1", finishes[0].ContainerID)

	var tool, code *airun.Step
	var calls []airun.Step
	for i := range steps {
		switch steps[i].Type {
		case airun.StepToolCall:
			tool = &steps[i]
		case airun.StepCodeExecution:
			code = &steps[i]
		case airun.StepLLMCall:
			calls = append(calls, steps[i])
		}
	}
	require.NotNil(t, tool)
	assert.Equal(t, airun.CallerCode, tool.Caller)
	assert.Equal(t, "srv1", tool.CallerToolID)

	require.NotNil(t, code)
	assert.Equal(t, "print(2)", code.ArgsJSON)
	assert.True(t, code.IsError, "a non-zero return code is an error")
	assert.Positive(t, code.DurationMs)
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(code.ResultJSON), &res))
	assert.Equal(t, "2\n", res["stdout"])
	assert.Equal(t, "warn", res["stderr"])
	assert.EqualValues(t, 3, res["return_code"])
	assert.Equal(t, "c1", res["container_id"])

	require.Len(t, calls, 2)
	var rb struct {
		Sent struct {
			CodeExecution bool     `json:"code_execution"`
			CodeTools     []string `json:"code_tools"`
			Container     string   `json:"container"`
		} `json:"sent"`
	}
	require.NoError(t, json.Unmarshal([]byte(calls[0].RequestJSON), &rb))
	assert.True(t, rb.Sent.CodeExecution)
	assert.Equal(t, []string{"find_foods"}, rb.Sent.CodeTools)
	assert.Equal(t, "c0", rb.Sent.Container)
	assert.Contains(t, calls[0].ResponseJSON, `"container_id":"c1"`)

	out := logs.String()
	assert.Contains(t, out, `"caller":"code_execution"`)
	assert.Contains(t, out, `"container_id":"c1"`)
	assert.Contains(t, out, "ai.code.execution")
}
