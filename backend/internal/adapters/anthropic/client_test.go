package anthropic_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		if r.Method == http.MethodGet { // the capability lookup: unknown model
			w.WriteHeader(http.StatusNotFound)
			return
		}
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
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
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

// modelJSON is a GET /v1/models/{id} body whose thinking.types.disabled
// support is canThink (false: always-thinking) and whose max_tokens is max.
func modelJSON(disabledOK bool, max int) string {
	return fmt.Sprintf(`{"id":"m","max_tokens":%d,"capabilities":{"thinking":{"supported":true,"types":{"disabled":{"supported":%t}}}}}`, max, disabledOK)
}

// The request rules follow the model's capabilities from the Models API, not
// a table: a model that cannot turn thinking off cannot take a forced
// tool_choice, an unknown or unreachable model gets `auto`, and temperature
// is sent whenever the caller sets one (a 400 retry covers models that
// refuse it).
func TestStream_ModelRequestRules(t *testing.T) {
	tools := []llm.Tool{{Name: "submit_plan", Description: "submit", Schema: json.RawMessage(`{"type":"object"}`)}}
	forced := llm.Request{
		Model: "any-model", System: "be brief", Tools: tools, Temperature: 0.4,
		ToolChoice: &llm.ToolChoice{Name: "submit_plan"},
	}
	const (
		forcedJSON  = `"tool_choice":{"type":"tool","name":"submit_plan"}`
		autoJSON    = `"tool_choice":{"type":"auto"}`
		systemLine  = "be brief\\n\\nAnswer by calling `submit_plan`."
		temperature = `"temperature"`
	)
	tests := []struct {
		name    string
		status  int
		model   string
		req     llm.Request
		present []string
		absent  []string
	}{
		{
			"thinking can be disabled: forced", 200, modelJSON(true, 64000), forced,
			[]string{forcedJSON, temperature},
			[]string{autoJSON},
		},
		{
			"always thinking: auto plus the system line", 200, modelJSON(false, 64000), forced,
			[]string{autoJSON, systemLine, temperature},
			[]string{forcedJSON},
		},
		{
			"no thinking support: forced", 200, `{"id":"m","capabilities":{"thinking":{"supported":false}}}`, forced,
			[]string{forcedJSON},
			[]string{autoJSON},
		},
		{
			"no capabilities in the answer: auto", 200, `{"id":"m"}`, forced,
			[]string{autoJSON, systemLine},
			[]string{forcedJSON},
		},
		{
			"unknown model (404): auto", 404, `{"type":"error","error":{"type":"not_found_error","message":"x"}}`, forced,
			[]string{autoJSON, systemLine},
			[]string{forcedJSON},
		},
		{
			"models API down (500): auto", 500, `{}`, forced,
			[]string{autoJSON, systemLine},
			[]string{forcedJSON},
		},
		{
			"no tool choice stays unset", 200, modelJSON(true, 64000),
			llm.Request{Model: "m", Tools: tools},
			[]string{`"max_tokens":4096`},
			[]string{`"tool_choice"`, temperature, "Answer by calling"},
		},
		{
			"max_tokens above the model's cap is clamped", 200, modelJSON(true, 8000),
			func() llm.Request { r := forced; r.MaxTokens = 16000; return r }(),
			[]string{`"max_tokens":8000`},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw []byte
			ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					assert.Equal(t, "/v1/models/"+tt.req.Model, r.URL.Path)
					w.WriteHeader(tt.status)
					_, _ = io.WriteString(w, tt.model)
					return
				}
				var err error
				raw, err = io.ReadAll(r.Body)
				require.NoError(t, err)
				w.Header().Set("Content-Type", "text/event-stream")
				b, _ := os.ReadFile("testdata/stream_text.sse")
				_, _ = w.Write(b)
			})
			c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
			out := make(chan llm.Event, 16)
			_, err := c.Stream(context.Background(), tt.req, out)
			require.NoError(t, err)
			_ = drainEvents(out)
			for _, s := range tt.present {
				assert.Contains(t, string(raw), s)
			}
			for _, s := range tt.absent {
				assert.NotContains(t, string(raw), s)
			}
		})
	}
}

// The Models API list is paginated and doubles as the API key check.
func TestListModels(t *testing.T) {
	page1 := `{"data":[{"id":"a","display_name":"A","lifecycle":"active","created_at":"2026-01-01T00:00:00Z"}],"has_more":true,"last_id":"a"}`
	page2 := `{"data":[{"id":"b","display_name":"B","lifecycle":"deprecated"}],"has_more":false,"last_id":"b"}`
	var queries []string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "k", r.Header.Get("x-api-key"))
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("after_id") == "a" {
			_, _ = io.WriteString(w, page2)
			return
		}
		_, _ = io.WriteString(w, page1)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	got, err := c.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "b", got[1].ID)
	assert.Equal(t, "deprecated", got[1].Lifecycle)
	assert.Equal(t, []string{"limit=1000", "limit=1000&after_id=a"}, queries)
}

func TestListModels_RejectedKey(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	})
	c := anthropic.New("bad", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	_, err := c.ListModels(context.Background())
	var apiErr *anthropic.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.Status)
	assert.Equal(t, "authentication_error", apiErr.ErrorType)
}

// The served model (message_start) is on the response.
func TestStream_ServedModel(t *testing.T) {
	c, _ := captureServer(t, "testdata/stream_text.sse")
	resp, _ := stream(t, c, llm.Request{Model: "claude-sonnet-4-6"})
	assert.NotEmpty(t, resp.Model)
}

// B8: thinking blocks (signature included) replay verbatim.
func TestStream_ThinkingBlocksReplayVerbatim(t *testing.T) {
	c, bodies := captureServer(t, "testdata/stream_thinking.sse")
	req := llm.Request{
		Model:    "claude-opus-5-5",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "plan"}}}},
	}
	resp, _ := stream(t, c, req)
	require.Len(t, resp.Message.Content, 3)
	assert.JSONEq(t, `{"type":"thinking","thinking":"Let me plan.","signature":"EqQBCgIYAhIM"}`, string(resp.Message.Content[0].Raw))
	assert.JSONEq(t, `{"type":"redacted_thinking","data":"EmwKAhgB"}`, string(resp.Message.Content[1].Raw))
	assert.Equal(t, "Hello", resp.Message.Content[2].Text)

	req.Messages = append(req.Messages, resp.Message)
	stream(t, c, req)
	var body struct {
		Messages []struct {
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(bodies()[1], &body))
	last := body.Messages[1].Content
	require.Len(t, last, 3)
	assert.JSONEq(t, `{"type":"thinking","thinking":"Let me plan.","signature":"EqQBCgIYAhIM"}`, string(last[0]))
	assert.JSONEq(t, `{"type":"redacted_thinking","data":"EmwKAhgB"}`, string(last[1]))
	assert.JSONEq(t, `{"type":"text","text":"Hello","cache_control":{"type":"ephemeral","ttl":"5m"}}`, string(last[2]))
}

type reqBodyPeek struct {
	MaxTokens int `json:"max_tokens"`
}

// DoD S8: a stream cut before message_stop is an error, not a reply.
func TestStream_TruncatedStreamIsError(t *testing.T) {
	full, err := os.ReadFile("testdata/stream_text.sse")
	require.NoError(t, err)
	i := bytes.Index(full, []byte("event: content_block_stop"))
	require.Positive(t, i)
	cut := full[:i]
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(cut)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	resp, err := c.Stream(context.Background(), llm.Request{Model: "m"}, make(chan llm.Event, 64))
	require.ErrorIs(t, err, anthropic.ErrStreamIncomplete)
	require.NotNil(t, resp, "the partial reply and its billed usage come back with the error")
	assert.True(t, resp.Partial)
	assert.Equal(t, "Hello world", resp.Message.Content[0].Text)
	assert.Equal(t, 42, resp.Usage.InputTokens)
}

// The request-id response header is on the response of a successful call.
func TestStream_RequestIDOnSuccess(t *testing.T) {
	body, err := os.ReadFile("testdata/stream_text.sse")
	require.NoError(t, err)
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("request-id", "req_011OK")
		_, _ = w.Write(body)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	out := make(chan llm.Event, 64)
	resp, err := c.Stream(context.Background(), llm.Request{Model: "claude-sonnet-4-6"}, out)
	require.NoError(t, err)
	_ = drainEvents(out)
	assert.Equal(t, "req_011OK", resp.RequestID)
}

// The effective parameters name the cache breakpoints the request carries.
func TestEffectiveParams_CacheBreakpoints(t *testing.T) {
	c := anthropic.New("k")
	e := c.EffectiveParams(llm.Request{Model: "m", System: "sys", Tools: []llm.Tool{{Name: "t"}}})
	assert.Equal(t, []string{"system", "tools"}, e.CacheBreakpoints)
	assert.Empty(t, c.EffectiveParams(llm.Request{Model: "m"}).CacheBreakpoints)
}

// The request puts a breakpoint on the last block of the last message, next to
// system and tools (3 of the 4 allowed), so the resent history is a cache read.
func TestStream_CachesConversationTail(t *testing.T) {
	fixture := serveFixture(t, "testdata/stream_text.sse", "text/event-stream")
	var body []byte
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		fixture(w, r)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	req := llm.Request{
		Model: "claude-sonnet-4-6", MaxTokens: 1024, System: "sys",
		Tools: []llm.Tool{{Name: "t", Schema: json.RawMessage(`{"type":"object"}`)}},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "hi"}}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeToolUse, ToolUseID: "tu_1", ToolUseName: "t", ToolUseInput: json.RawMessage(`{}`)}}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				{Type: llm.ContentTypeToolResult, ToolResultID: "tu_1", ToolResultContent: json.RawMessage(`"ok"`)},
				{Type: llm.ContentTypeText, Text: "go on"},
			}},
		},
	}
	out := make(chan llm.Event, 64)
	_, err := c.Stream(context.Background(), req, out)
	require.NoError(t, err)
	_ = drainEvents(out)

	var got struct {
		Messages []struct {
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	marked := 0
	for i, m := range got.Messages {
		for j, b := range m.Content {
			if _, ok := b["cache_control"]; ok {
				marked++
				assert.Equal(t, 2, i)
				assert.Equal(t, len(m.Content)-1, j)
			}
		}
	}
	assert.Equal(t, 1, marked)
	assert.Equal(t, 3, bytes.Count(body, []byte(`"cache_control"`)))
	assert.Equal(t, []string{"system", "tools", "messages"}, c.EffectiveParams(req).CacheBreakpoints)
}
