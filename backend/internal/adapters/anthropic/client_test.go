package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func drainEvents(ch <-chan llm.Event) []llm.Event {
	out := []llm.Event{}
	for e := range ch {
		out = append(out, e)
	}
	return out
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func serveFixture(t *testing.T, path, contentType string) http.HandlerFunc {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func TestStream_TextOnly(t *testing.T) {
	ts := newTestServer(t, serveFixture(t, "testdata/stream_text.sse", "text/event-stream"))
	c := anthropic.New("test-key", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 64)
	resp, err := c.Stream(context.Background(), llm.Request{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "hi"}}},
		},
	}, out)
	require.NoError(t, err)
	require.NotNil(t, resp)

	events := drainEvents(out)
	require.NotEmpty(t, events)
	assert.Equal(t, llm.EventMessageStart, events[0].Type)
	assert.Equal(t, llm.StopReasonEndTurn, resp.StopReason)
	assert.Equal(t, 7, resp.Usage.OutputTokens)
	assert.Equal(t, 42, resp.Usage.InputTokens)
	assert.Equal(t, 100, resp.Usage.CacheReadTokens)

	require.Len(t, resp.Message.Content, 1)
	assert.Equal(t, llm.ContentTypeText, resp.Message.Content[0].Type)
	assert.Equal(t, "Hello world", resp.Message.Content[0].Text)

	// Confirm the deltas were emitted in order.
	var deltas []string
	for _, e := range events {
		if e.Type == llm.EventAssistantDelta {
			deltas = append(deltas, e.Payload.(llm.AssistantDeltaPayload).Text)
		}
	}
	assert.Equal(t, []string{"Hello", " world"}, deltas)
}

func TestStream_ToolUseAssembled(t *testing.T) {
	ts := newTestServer(t, serveFixture(t, "testdata/stream_tool_use.sse", "text/event-stream"))
	c := anthropic.New("test-key", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 64)
	resp, err := c.Stream(context.Background(), llm.Request{
		Model:    "claude-sonnet-4-6",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "plan"}}}},
	}, out)
	require.NoError(t, err)

	events := drainEvents(out)
	var toolCallStarts, toolCallDeltas int
	for _, e := range events {
		switch e.Type {
		case llm.EventToolCallStart:
			toolCallStarts++
		case llm.EventToolCallDelta:
			toolCallDeltas++
		}
	}
	assert.Equal(t, 1, toolCallStarts)
	assert.Equal(t, 2, toolCallDeltas)
	assert.Equal(t, llm.StopReasonToolUse, resp.StopReason)

	// Response contains text + tool_use, with accumulated JSON parsed.
	require.Len(t, resp.Message.Content, 2)
	assert.Equal(t, llm.ContentTypeText, resp.Message.Content[0].Type)
	assert.Equal(t, "Let me check", resp.Message.Content[0].Text)
	tool := resp.Message.Content[1]
	assert.Equal(t, llm.ContentTypeToolUse, tool.Type)
	assert.Equal(t, "toolu_abc", tool.ToolUseID)
	assert.Equal(t, "list_slots", tool.ToolUseName)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(tool.ToolUseInput, &parsed))
	assert.Equal(t, true, parsed["active_only"])
}

func TestStream_ParallelToolUses(t *testing.T) {
	ts := newTestServer(t, serveFixture(t, "testdata/stream_parallel_tools.sse", "text/event-stream"))
	c := anthropic.New("test-key", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 64)
	resp, err := c.Stream(context.Background(), llm.Request{Model: "claude-sonnet-4-6"}, out)
	require.NoError(t, err)
	_ = drainEvents(out)

	require.Len(t, resp.Message.Content, 2)
	assert.Equal(t, "toolu_a", resp.Message.Content[0].ToolUseID)
	assert.Equal(t, "list_slots", resp.Message.Content[0].ToolUseName)
	assert.Equal(t, "toolu_b", resp.Message.Content[1].ToolUseID)
	assert.Equal(t, "get_profile", resp.Message.Content[1].ToolUseName)
	assert.Equal(t, llm.StopReasonToolUse, resp.StopReason)
}

func TestStream_RateLimitError(t *testing.T) {
	body, err := os.ReadFile("testdata/error_rate_limit.json")
	require.NoError(t, err)

	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(body)
	})
	c := anthropic.New("test-key", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 8)
	_, err = c.Stream(context.Background(), llm.Request{Model: "claude-sonnet-4-6"}, out)
	require.Error(t, err)

	apiErr, ok := err.(*anthropic.APIError)
	require.True(t, ok, "expected *anthropic.APIError, got %T", err)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.Status)
	assert.Equal(t, "rate_limit_error", apiErr.ErrorType)
	assert.Contains(t, apiErr.Message, "rate-limited")
}

func TestStream_RequestEncoding(t *testing.T) {
	var received struct {
		Model     string          `json:"model"`
		MaxTokens int             `json:"max_tokens"`
		Stream    bool            `json:"stream"`
		System    json.RawMessage `json:"system"`
		Messages  []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string          `json:"type"`
				Text      string          `json:"text,omitempty"`
				ID        string          `json:"id,omitempty"`
				Name      string          `json:"name,omitempty"`
				Input     json.RawMessage `json:"input,omitempty"`
				ToolUseID string          `json:"tool_use_id,omitempty"`
				Content   json.RawMessage `json:"content,omitempty"`
				IsError   bool            `json:"is_error,omitempty"`
			} `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Name         string          `json:"name"`
			CacheControl json.RawMessage `json:"cache_control"`
		} `json:"tools"`
	}
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))

		w.Header().Set("Content-Type", "text/event-stream")
		body, _ := os.ReadFile("testdata/stream_text.sse")
		_, _ = w.Write(body)
	})
	c := anthropic.New("test-key", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	toolInput := json.RawMessage(`{"week_id":1,"day":0,"slot_id":2}`)
	toolResultOut := json.RawMessage(`{"id":42}`)

	out := make(chan llm.Event, 64)
	_, err := c.Stream(context.Background(), llm.Request{
		Model:  "claude-sonnet-4-6",
		System: "you are test",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "plan"}}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				{Type: llm.ContentTypeText, Text: "ok"},
				{Type: llm.ContentTypeToolUse, ToolUseID: "tu_1", ToolUseName: "create_plate", ToolUseInput: toolInput},
			}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				{Type: llm.ContentTypeText, Text: "note-after-result"},
				{Type: llm.ContentTypeToolResult, ToolResultID: "tu_1", ToolResultContent: toolResultOut},
			}},
		},
		Tools: []llm.Tool{
			{Name: "list_slots", Description: "list slots", Schema: json.RawMessage(`{"type":"object"}`)},
			{Name: "create_plate", Description: "create", Schema: json.RawMessage(`{"type":"object"}`)},
		},
	}, out)
	require.NoError(t, err)
	_ = drainEvents(out)

	assert.Equal(t, "claude-sonnet-4-6", received.Model)
	assert.Equal(t, 4096, received.MaxTokens)
	assert.True(t, received.Stream)
	require.Len(t, received.Messages, 3)

	// The tool_result block must be first in the user-role message that follows
	// a tool_use (Anthropic strict requirement).
	lastMsg := received.Messages[2]
	require.NotEmpty(t, lastMsg.Content)
	assert.Equal(t, "tool_result", lastMsg.Content[0].Type)
	assert.Equal(t, "tu_1", lastMsg.Content[0].ToolUseID)

	// Cache control must be set on the last tool for prompt caching.
	require.Len(t, received.Tools, 2)
	assert.NotNil(t, received.Tools[1].CacheControl)

	// System block must be present and non-empty.
	assert.NotEmpty(t, string(received.System))
}

func TestStream_DefaultsMaxTokensWhenZero(t *testing.T) {
	var body reqBodyPeek
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("Content-Type", "text/event-stream")
		b, _ := os.ReadFile("testdata/stream_text.sse")
		_, _ = w.Write(b)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	out := make(chan llm.Event, 16)
	_, err := c.Stream(context.Background(), llm.Request{Model: "claude-sonnet-4-6"}, out)
	require.NoError(t, err)
	_ = drainEvents(out)
	assert.Equal(t, anthropic.DefaultMaxTokens, body.MaxTokens)
}

type reqBodyPeek struct {
	MaxTokens int `json:"max_tokens"`
}
