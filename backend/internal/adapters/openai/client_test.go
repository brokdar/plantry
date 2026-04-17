package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/openai"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func drainEvents(ch <-chan llm.Event) []llm.Event {
	out := []llm.Event{}
	for e := range ch {
		out = append(out, e)
	}
	return out
}

func serve(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(fixture)
	require.NoError(t, err)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(body)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestStream_TextOnly(t *testing.T) {
	ts := serve(t, "testdata/stream_text.sse")
	c := openai.New("k", openai.WithBaseURL(ts.URL), openai.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 64)
	resp, err := c.Stream(context.Background(), llm.Request{Model: "gpt-5.4-mini"}, out)
	require.NoError(t, err)
	events := drainEvents(out)

	var deltas []string
	for _, e := range events {
		if e.Type == llm.EventAssistantDelta {
			deltas = append(deltas, e.Payload.(llm.AssistantDeltaPayload).Text)
		}
	}
	assert.Equal(t, []string{"Hello", " world"}, deltas)
	assert.Equal(t, llm.StopReasonEndTurn, resp.StopReason)
	assert.Equal(t, 42, resp.Usage.InputTokens)
	assert.Equal(t, 7, resp.Usage.OutputTokens)
	assert.Equal(t, 30, resp.Usage.CacheReadTokens)

	require.Len(t, resp.Message.Content, 1)
	assert.Equal(t, "Hello world", resp.Message.Content[0].Text)
}

func TestStream_ToolUseAssembled(t *testing.T) {
	ts := serve(t, "testdata/stream_tool_use.sse")
	c := openai.New("k", openai.WithBaseURL(ts.URL), openai.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 64)
	resp, err := c.Stream(context.Background(), llm.Request{Model: "gpt-5.4-mini"}, out)
	require.NoError(t, err)
	events := drainEvents(out)

	var starts, deltas int
	for _, e := range events {
		switch e.Type {
		case llm.EventToolCallStart:
			starts++
		case llm.EventToolCallDelta:
			deltas++
		}
	}
	assert.Equal(t, 1, starts)
	assert.Equal(t, 2, deltas)
	assert.Equal(t, llm.StopReasonToolUse, resp.StopReason)

	require.Len(t, resp.Message.Content, 1)
	tc := resp.Message.Content[0]
	assert.Equal(t, llm.ContentTypeToolUse, tc.Type)
	assert.Equal(t, "call_abc", tc.ToolUseID)
	assert.Equal(t, "list_slots", tc.ToolUseName)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(tc.ToolUseInput, &parsed))
	assert.Equal(t, true, parsed["active_only"])
}

func TestStream_ParallelToolCalls(t *testing.T) {
	ts := serve(t, "testdata/stream_parallel_tools.sse")
	c := openai.New("k", openai.WithBaseURL(ts.URL), openai.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 64)
	resp, err := c.Stream(context.Background(), llm.Request{Model: "gpt-5.4-mini"}, out)
	require.NoError(t, err)
	_ = drainEvents(out)

	require.Len(t, resp.Message.Content, 2)
	assert.Equal(t, "call_a", resp.Message.Content[0].ToolUseID)
	assert.Equal(t, "list_slots", resp.Message.Content[0].ToolUseName)
	assert.Equal(t, "call_b", resp.Message.Content[1].ToolUseID)
	assert.Equal(t, "get_profile", resp.Message.Content[1].ToolUseName)
	assert.Equal(t, llm.StopReasonToolUse, resp.StopReason)
}

func TestStream_RateLimitError(t *testing.T) {
	body, err := os.ReadFile("testdata/error_rate_limit.json")
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(body)
	}))
	defer ts.Close()
	c := openai.New("k", openai.WithBaseURL(ts.URL), openai.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 8)
	_, err = c.Stream(context.Background(), llm.Request{Model: "gpt-5.4-mini"}, out)
	require.Error(t, err)

	apiErr, ok := err.(*openai.APIError)
	require.True(t, ok, "expected *openai.APIError, got %T", err)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.Status)
	assert.Equal(t, "rate_limit_error", apiErr.ErrorType)
}

func TestStream_RequestEncoding(t *testing.T) {
	var received struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content,omitempty"`
			ToolCallID string `json:"tool_call_id,omitempty"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"messages"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer k", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.Header().Set("Content-Type", "text/event-stream")
		b, _ := os.ReadFile("testdata/stream_text.sse")
		_, _ = w.Write(b)
	}))
	defer ts.Close()
	c := openai.New("k", openai.WithBaseURL(ts.URL), openai.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 64)
	_, err := c.Stream(context.Background(), llm.Request{
		Model:  "gpt-5.4-mini",
		System: "you are test",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "plan"}}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				{Type: llm.ContentTypeText, Text: "ok"},
				{
					Type: llm.ContentTypeToolUse, ToolUseID: "call_1", ToolUseName: "list_slots",
					ToolUseInput: json.RawMessage(`{}`),
				},
			}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				{
					Type: llm.ContentTypeToolResult, ToolResultID: "call_1",
					ToolResultContent: json.RawMessage(`{"ok":true}`),
				},
			}},
		},
		Tools: []llm.Tool{
			{Name: "list_slots", Description: "d", Schema: json.RawMessage(`{"type":"object"}`)},
		},
	}, out)
	require.NoError(t, err)
	_ = drainEvents(out)

	assert.Equal(t, "gpt-5.4-mini", received.Model)
	assert.True(t, received.Stream)

	// System first, then user, then assistant (with tool_calls), then tool.
	require.GreaterOrEqual(t, len(received.Messages), 4)
	assert.Equal(t, "system", received.Messages[0].Role)
	assert.Equal(t, "user", received.Messages[1].Role)
	assert.Equal(t, "assistant", received.Messages[2].Role)
	require.Len(t, received.Messages[2].ToolCalls, 1)
	assert.Equal(t, "call_1", received.Messages[2].ToolCalls[0].ID)
	assert.Equal(t, "list_slots", received.Messages[2].ToolCalls[0].Function.Name)
	assert.Equal(t, "tool", received.Messages[3].Role)
	assert.Equal(t, "call_1", received.Messages[3].ToolCallID)

	require.Len(t, received.Tools, 1)
	assert.Equal(t, "function", received.Tools[0].Type)
	assert.Equal(t, "list_slots", received.Tools[0].Function.Name)
}
