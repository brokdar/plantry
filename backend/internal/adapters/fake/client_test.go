package fake

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func endTurnScript(usage ScriptedUsage) *Script {
	return &Script{Turns: []Turn{{
		StopReason: "end_turn",
		Message:    ScriptedMessage{Content: []ScriptedBlock{{Type: "text", Text: "ok"}}},
		Usage:      usage,
	}}}
}

func stream(t *testing.T, c *Client, req llm.Request) *llm.Response {
	t.Helper()
	out := make(chan llm.Event, 16)
	resp, err := c.Stream(context.Background(), req, out)
	require.NoError(t, err)
	return resp
}

// The existing script format already carries tool_use turns with JSON input,
// so a forced submit_plan call and its repair are scripted without changes.
func TestClient_ScriptedForcedToolCalls(t *testing.T) {
	raw := `{"turns":[
		{"stop_reason":"tool_use","message":{"content":[{"type":"tool_use","tool_use_id":"tu_1","tool_name":"submit_plan","tool_input":{"plates":[],"notes":"first"}}]}},
		{"stop_reason":"tool_use","message":{"content":[{"type":"tool_use","tool_use_id":"tu_2","tool_name":"submit_plan","tool_input":{"plates":[],"notes":"repaired"}}]}}
	]}`
	var s Script
	require.NoError(t, json.Unmarshal([]byte(raw), &s))
	c := NewFromScript(&s)

	tools := []llm.Tool{{Name: "submit_plan", Schema: json.RawMessage(`{"type":"object"}`)}}
	choice := &llm.ToolChoice{Name: "submit_plan"}
	user := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "plan"}}}

	first := stream(t, c, llm.Request{Messages: []llm.Message{user}, Tools: tools, ToolChoice: choice, Temperature: 0.4})
	assert.Equal(t, llm.StopReasonToolUse, first.StopReason)
	require.Len(t, first.Message.Content, 1)
	assert.Equal(t, "submit_plan", first.Message.Content[0].ToolUseName)
	assert.JSONEq(t, `{"plates":[],"notes":"first"}`, string(first.Message.Content[0].ToolUseInput))

	repair := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{
		Type: llm.ContentTypeToolResult, ToolResultID: "tu_1", ToolResultContent: json.RawMessage(`"violations: H1"`), ToolResultIsError: true,
	}}}
	second := stream(t, c, llm.Request{Messages: []llm.Message{user, first.Message, repair}, Tools: tools, ToolChoice: choice})
	assert.JSONEq(t, `{"plates":[],"notes":"repaired"}`, string(second.Message.Content[0].ToolUseInput))

	reqs := c.Requests()
	require.Len(t, reqs, 2)
	for _, r := range reqs {
		require.NotNil(t, r.ToolChoice)
		assert.Equal(t, "submit_plan", r.ToolChoice.Name)
	}
	assert.InDelta(t, 0.4, reqs[0].Temperature, 1e-9)
}

func TestClient_RecordsRequests(t *testing.T) {
	t.Parallel()

	c := NewFromScript(endTurnScript(ScriptedUsage{}))

	stream(t, c, llm.Request{Model: "m1", System: "sys one"})
	_, err := c.Complete(context.Background(), llm.Request{Model: "m2", System: "sys two"})
	require.NoError(t, err)

	reqs := c.Requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "m1", reqs[0].Model)
	assert.Equal(t, "sys one", reqs[0].System)
	assert.Equal(t, "m2", reqs[1].Model)
	assert.Equal(t, "sys two", reqs[1].System)

	// The returned slice is a copy: mutating it must not affect the client.
	reqs[0].Model = "mutated"
	assert.Equal(t, "m1", c.Requests()[0].Model)
}

func TestClient_RecordsConcurrentRequests(t *testing.T) {
	t.Parallel()

	c := NewFromScript(endTurnScript(ScriptedUsage{}))

	const n = 20
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Complete(context.Background(), llm.Request{Model: "m"})
			assert.NoError(t, err)
			_ = c.Requests()
		}()
	}
	wg.Wait()

	assert.Len(t, c.Requests(), n)
}

func TestClient_UsageIncludesCacheTokens(t *testing.T) {
	t.Parallel()

	var s Script
	require.NoError(t, json.Unmarshal([]byte(`{"turns":[{
		"stop_reason":"end_turn",
		"message":{"content":[{"type":"text","text":"ok"}]},
		"usage":{"input_tokens":10,"output_tokens":20,"cache_read_tokens":30,"cache_write_tokens":40}
	}]}`), &s))
	c := NewFromScript(&s)

	resp := stream(t, c, llm.Request{})

	assert.Equal(t, llm.Usage{
		InputTokens: 10, OutputTokens: 20, CacheReadTokens: 30, CacheWriteTokens: 40,
	}, resp.Usage)
}

func TestClient_AbandonedSessionDoesNotLeakIntoNewSession(t *testing.T) {
	c := NewFromScript(&Script{Turns: []Turn{
		{StopReason: "tool_use", Message: ScriptedMessage{Content: []ScriptedBlock{{Type: "tool_use", ToolUseID: "t1", ToolName: "x"}}}},
		{StopReason: "end_turn", Message: ScriptedMessage{Content: []ScriptedBlock{{Type: "text", Text: "done"}}}},
	}})
	tools := []llm.Tool{{Name: "x"}}
	user := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "hi"}}}

	// Session 1 takes the tool_use turn and is abandoned before the tool result.
	assert.Equal(t, llm.StopReasonToolUse, stream(t, c, llm.Request{Tools: tools, Messages: []llm.Message{user}}).StopReason)

	// Session 2 starts with a fresh user message and must get turn 0 again.
	resp := stream(t, c, llm.Request{Tools: tools, Messages: []llm.Message{user}})
	assert.Equal(t, llm.StopReasonToolUse, resp.StopReason)

	// Its tool-result round trip advances to the end turn.
	result := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeToolResult, ToolResultID: "t1"}}}
	resp = stream(t, c, llm.Request{Tools: tools, Messages: []llm.Message{user, resp.Message, result}})
	assert.Equal(t, llm.StopReasonEndTurn, resp.StopReason)
}
