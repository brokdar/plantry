package anthropic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// pausedSSE is a reply that paused on a tool call made by code: the code's
// server tool use, a tool_use whose caller is that code, and a container.
const pausedSSE = `event: message_start
data: {"type":"message_start","message":{"model":"claude-opus-5-5","container":{"id":"container_1","expires_at":"2026-11-16T20:00:00Z"},"usage":{"input_tokens":5,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_1","name":"code_execution","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"code\":\"print(1)\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"find_foods","input":{},"caller":{"type":"code_execution_20260120","tool_id":"srvtoolu_1"}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}

`

func ptcRequest() llm.Request {
	return llm.Request{
		Model:    "claude-opus-5-5",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "hi"}}}},
		Tools: []llm.Tool{
			{
				Name: "find_foods", Description: "d", Schema: json.RawMessage(`{"type":"object"}`),
				AllowedCallers: []string{llm.CallerDirect, llm.CallerCodeExecution},
			},
			{Name: "confirm", Description: "d", Schema: json.RawMessage(`{"type":"object"}`)},
		},
		CodeExecution: true,
		Container:     "container_0",
	}
}

// ptcServer answers the capability lookup with caps and each message request
// with reply(body); it records the message bodies.
func ptcServer(t *testing.T, caps string, reply func(body string, w http.ResponseWriter)) (*anthropic.Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/models/") {
			_, _ = io.WriteString(w, caps)
			return
		}
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		reply(string(b), w)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	return c, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

func sse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, body)
}

const supportsCode = `{"capabilities":{"code_execution":{"supported":true}}}`

func TestStream_ProgrammaticRequestShapeAndParsing(t *testing.T) {
	c, bodies := ptcServer(t, supportsCode, func(_ string, w http.ResponseWriter) { sse(w, pausedSSE) })
	resp, _ := stream(t, c, ptcRequest())

	var body struct {
		Container string           `json:"container"`
		Tools     []map[string]any `json:"tools"`
	}
	require.Len(t, bodies(), 1)
	require.NoError(t, json.Unmarshal([]byte(bodies()[0]), &body))
	assert.Equal(t, "container_0", body.Container)
	require.Len(t, body.Tools, 3)
	assert.Equal(t, "code_execution_20260120", body.Tools[0]["type"])
	assert.Equal(t, "code_execution", body.Tools[0]["name"])
	assert.Equal(t, []any{"direct", "code_execution_20260120"}, body.Tools[1]["allowed_callers"])
	assert.NotContains(t, body.Tools[2], "allowed_callers", "a tool without callers stays direct-only")

	eff := c.EffectiveParams(ptcRequest())
	assert.True(t, eff.CodeExecution)
	assert.Equal(t, []string{"find_foods"}, eff.CodeTools)
	assert.Equal(t, "container_0", eff.Container)

	// Response: container, server block verbatim, the tool call's caller.
	assert.Equal(t, "container_1", resp.ContainerID)
	assert.Equal(t, 2026, resp.ContainerExpiresAt.Year())
	require.Len(t, resp.Message.Content, 2)
	assert.Equal(t, llm.ContentTypeServer, resp.Message.Content[0].Type)
	assert.JSONEq(t, `{"type":"server_tool_use","id":"srvtoolu_1","name":"code_execution","input":{"code":"print(1)"}}`, string(resp.Message.Content[0].Raw))
	call := resp.Message.Content[1]
	assert.Equal(t, llm.ContentTypeToolUse, call.Type)
	assert.True(t, call.Caller.FromCode())
	assert.Equal(t, "srvtoolu_1", call.Caller.ToolID)
	assert.Equal(t, llm.StopReasonToolUse, resp.StopReason)
}

func TestStream_ProgrammaticContinuationReplaysCallerAndContainer(t *testing.T) {
	c, bodies := ptcServer(t, supportsCode, func(_ string, w http.ResponseWriter) { sse(w, pausedSSE) })
	first, _ := stream(t, c, ptcRequest())

	req := ptcRequest()
	req.Container = first.ContainerID
	req.Messages = append(req.Messages, first.Message, llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{
		Type: llm.ContentTypeToolResult, ToolResultID: "toolu_1", ToolResultContent: json.RawMessage(`"{}"`),
	}}})
	stream(t, c, req)

	var body struct {
		Container string `json:"container"`
		Messages  []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	require.Len(t, bodies(), 2)
	require.NoError(t, json.Unmarshal([]byte(bodies()[1]), &body))
	assert.Equal(t, "container_1", body.Container)
	require.Len(t, body.Messages, 3)
	assistant := body.Messages[1].Content
	assert.Equal(t, "server_tool_use", assistant[0]["type"])
	assert.Equal(t, map[string]any{"type": "code_execution_20260120", "tool_id": "srvtoolu_1"}, assistant[1]["caller"])
	last := body.Messages[2].Content
	require.Len(t, last, 1)
	assert.Equal(t, "tool_result", last[0]["type"], "the answer to programmatic calls holds tool_result blocks only")
}

func TestStream_ProgrammaticUnsupportedModelSendsNoCodeTool(t *testing.T) {
	c, bodies := ptcServer(t, `{"capabilities":{"code_execution":{"supported":false}}}`, func(_ string, w http.ResponseWriter) { sse(w, pausedSSE) })
	stream(t, c, ptcRequest())

	b := bodies()[0]
	assert.NotContains(t, b, "code_execution_20260120")
	assert.NotContains(t, b, "allowed_callers")
	assert.NotContains(t, b, `"container"`)
	eff := c.EffectiveParams(ptcRequest())
	assert.False(t, eff.CodeExecution)
	assert.Contains(t, eff.Fallbacks, "code_execution_unsupported_by_model")
}

func TestStream_ProgrammaticRejectedRetriesOnceWithout(t *testing.T) {
	c, bodies := ptcServer(t, supportsCode, func(body string, w http.ResponseWriter) {
		if strings.Contains(body, "allowed_callers") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"tools.1.allowed_callers: code execution is not enabled for this organization"}}`)
			return
		}
		sse(w, pausedSSE)
	})
	var attempts []llm.Attempt
	ctx := llm.WithAttemptObserver(context.Background(), func(a llm.Attempt) { attempts = append(attempts, a) })
	out := make(chan llm.Event, 64)
	_, err := c.Stream(ctx, ptcRequest(), out)
	require.NoError(t, err)

	require.Len(t, bodies(), 2)
	assert.Contains(t, bodies()[0], "code_execution_20260120")
	assert.NotContains(t, bodies()[1], "code_execution_20260120")
	assert.NotContains(t, bodies()[1], "allowed_callers")
	require.Len(t, attempts, 1)
	assert.Equal(t, "code_execution_rejected_400", attempts[0].Reason)
	assert.True(t, attempts[0].Sent.CodeExecution)

	eff := c.EffectiveParams(ptcRequest())
	assert.False(t, eff.CodeExecution, "remembered per model")
	assert.Contains(t, eff.Fallbacks, "code_execution_rejected_by_model")
}

func TestStream_ProgrammaticNotSentWithForcedToolChoice(t *testing.T) {
	c, bodies := ptcServer(t, supportsCode, func(_ string, w http.ResponseWriter) { sse(w, pausedSSE) })
	req := ptcRequest()
	req.ToolChoice = &llm.ToolChoice{Name: "confirm"}
	stream(t, c, req)
	assert.NotContains(t, bodies()[0], "code_execution_20260120")
	assert.Contains(t, c.EffectiveParams(req).Fallbacks, "code_execution_with_tool_choice")
}
