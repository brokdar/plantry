package anthropic_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

var update = flag.Bool("update", false, "rewrite golden files")

// captureServer records every request body and answers with the SSE fixture.
func captureServer(t *testing.T, fixture string) (*anthropic.Client, func() [][]byte) {
	t.Helper()
	sse, err := os.ReadFile(fixture)
	require.NoError(t, err)
	var mu sync.Mutex
	var bodies [][]byte
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		bodies = append(bodies, b)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	return c, func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		return append([][]byte(nil), bodies...)
	}
}

func stream(t *testing.T, c *anthropic.Client, req llm.Request) (*llm.Response, []llm.Event) {
	t.Helper()
	out := make(chan llm.Event, 256)
	resp, err := c.Stream(context.Background(), req, out)
	require.NoError(t, err)
	return resp, drainEvents(out)
}

func baseRequest() llm.Request {
	return llm.Request{
		Model:  "claude-opus-5-5",
		System: "you are test",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "find recipes"}}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				{Type: llm.ContentTypeText, Text: "ok"},
				{Type: llm.ContentTypeToolUse, ToolUseID: "tu_1", ToolUseName: "list_slots", ToolUseInput: json.RawMessage(`{"a":1}`)},
			}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				{Type: llm.ContentTypeToolResult, ToolResultID: "tu_1", ToolResultContent: json.RawMessage(`{"id":42}`)},
			}},
		},
		Tools: []llm.Tool{
			{Name: "list_slots", Description: "list slots", Schema: json.RawMessage(`{"type":"object"}`)},
		},
		Temperature: 0.4,
		MaxTokens:   2048,
	}
}

// B1-AC1: without WebSearch the body is byte-identical to the pre-B1 encoder
// (golden recorded before the change); with it the server tool follows the
// custom tools.
func TestStream_WebSearchToolEncoding(t *testing.T) {
	t.Run("without search the body is unchanged", func(t *testing.T) {
		c, bodies := captureServer(t, "testdata/stream_text.sse")
		stream(t, c, baseRequest())
		golden := "testdata/request_no_search.golden.json"
		if *update {
			require.NoError(t, os.WriteFile(golden, append(bodies()[0], '\n'), 0o644))
		}
		want, err := os.ReadFile(golden)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSuffix(string(want), "\n"), string(bodies()[0]))
	})

	t.Run("with search the server tool is added next to custom tools", func(t *testing.T) {
		c, bodies := captureServer(t, "testdata/stream_text.sse")
		req := baseRequest()
		req.WebSearch = &llm.WebSearch{MaxUses: 5}
		stream(t, c, req)

		var body struct {
			Tools []json.RawMessage `json:"tools"`
		}
		require.NoError(t, json.Unmarshal(bodies()[0], &body))
		require.Len(t, body.Tools, 2)
		assert.Contains(t, string(body.Tools[0]), `"name":"list_slots"`)
		assert.JSONEq(t, `{"type":"web_search_20250305","name":"web_search","max_uses":5}`, string(body.Tools[1]))
	})

	t.Run("search alone sends only the server tool", func(t *testing.T) {
		c, bodies := captureServer(t, "testdata/stream_text.sse")
		stream(t, c, llm.Request{Model: "claude-opus-5-5", WebSearch: &llm.WebSearch{MaxUses: 3}})
		assert.Contains(t, string(bodies()[0]), `"tools":[{"type":"web_search_20250305","name":"web_search","max_uses":3}]`)
	})
}

// fixtureBlock returns the compacted content_block of the SSE event whose
// data line contains marker.
func fixtureBlock(t *testing.T, fixture, marker string) string {
	t.Helper()
	raw, err := os.ReadFile(fixture)
	require.NoError(t, err)
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		var ev struct {
			ContentBlock json.RawMessage `json:"content_block"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev))
		var buf bytes.Buffer
		require.NoError(t, json.Compact(&buf, ev.ContentBlock))
		return buf.String()
	}
	t.Fatalf("marker %q not in %s", marker, fixture)
	return ""
}

// B1-AC2: the search fixture yields results, the search count, the final text
// and server blocks that re-encode byte-identical.
func TestStream_WebSearchTurn(t *testing.T) {
	const fixture = "testdata/stream_web_search.sse"
	c, bodies := captureServer(t, fixture)
	req := llm.Request{
		Model:     "claude-opus-5-5",
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "curry"}}}},
		WebSearch: &llm.WebSearch{MaxUses: 5},
	}
	resp, events := stream(t, c, req)

	assert.Equal(t, llm.StopReasonEndTurn, resp.StopReason)
	assert.Equal(t, 2, resp.Usage.WebSearchRequests)
	assert.Equal(t, 210, resp.Usage.OutputTokens)
	assert.Equal(t, []llm.SearchResult{
		{URL: "https://example.com/green-curry", Title: "Green Curry"},
		{URL: "https://example.org/red-curry", Title: "Red Curry"},
		{URL: "https://example.net/massaman", Title: "Massaman Curry"},
	}, resp.SearchResults)

	content := resp.Message.Content
	require.Len(t, content, 6)
	assert.Equal(t, llm.ContentTypeText, content[0].Type)
	assert.Equal(t, "Searching for recipes.", content[0].Text)
	for _, i := range []int{1, 2, 3, 4} {
		assert.Equal(t, llm.ContentTypeServer, content[i].Type, "block %d", i)
	}
	final := content[5]
	assert.Equal(t, llm.ContentTypeText, final.Type)
	assert.Equal(t, `{"candidates":[]}`, final.Text)
	assert.Equal(t, []string{"https://example.com/green-curry"}, final.Citations)

	// The result blocks are the fixture's blocks verbatim; the tool-use blocks
	// carry the streamed input.
	assert.Equal(t, fixtureBlock(t, fixture, `"tool_use_id":"srvtoolu_01A"`), string(content[2].Raw))
	assert.Equal(t, fixtureBlock(t, fixture, `"tool_use_id":"srvtoolu_01B"`), string(content[4].Raw))
	assert.Contains(t, string(content[2].Raw), `"encrypted_content":"EqgfCioIARgBIiQ3YTAw+/="`)
	assert.JSONEq(t, `{"type":"server_tool_use","id":"srvtoolu_01A","name":"web_search","input":{"query":"thai curry recipe"}}`, string(content[1].Raw))

	// Server tool blocks never surface as client tool calls.
	for _, e := range events {
		assert.NotEqual(t, llm.EventToolCallStart, e.Type)
		assert.NotEqual(t, llm.EventToolCallDelta, e.Type)
	}

	// Resending the turn replays every server block byte for byte.
	req.Messages = append(req.Messages, resp.Message,
		llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "more"}}})
	stream(t, c, req)
	second := string(bodies()[1])
	for _, i := range []int{1, 2, 3, 4} {
		assert.Contains(t, second, string(content[i].Raw), "block %d", i)
	}
	assert.NotContains(t, second, `"citations"`)
}

// B1-AC3: pause_turn surfaces; resending messages plus the returned blocks
// keeps the trailing server_tool_use and adds no user text.
func TestStream_PauseTurn(t *testing.T) {
	c, bodies := captureServer(t, "testdata/stream_pause_turn.sse")
	req := llm.Request{
		Model:     "claude-opus-5-5",
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "ramen"}}}},
		WebSearch: &llm.WebSearch{MaxUses: 5},
	}
	resp, _ := stream(t, c, req)
	assert.Equal(t, llm.StopReasonPauseTurn, resp.StopReason)
	assert.Equal(t, 1, resp.Usage.WebSearchRequests)
	require.Len(t, resp.Message.Content, 3)
	trailing := resp.Message.Content[2]
	require.Equal(t, llm.ContentTypeServer, trailing.Type)

	req.Messages = append(req.Messages, resp.Message)
	stream(t, c, req)

	var body struct {
		Messages []struct {
			Role    string            `json:"role"`
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(bodies()[1], &body))
	require.Len(t, body.Messages, 2)
	last := body.Messages[1]
	assert.Equal(t, "assistant", last.Role)
	require.Len(t, last.Content, 3)
	assert.Equal(t, string(trailing.Raw), string(last.Content[2]))
	assert.JSONEq(t, `{"type":"server_tool_use","id":"srvtoolu_02B","name":"web_search","input":{"query":"tonkotsu"}}`, string(last.Content[2]))
}

// B1-AC5: unknown block types neither fail the stream nor emit tool deltas
// without an ID.
func TestStream_UnknownBlockTypes(t *testing.T) {
	sse := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"model":"claude-opus-5-5","usage":{"input_tokens":5,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"mystery_block","payload":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"mystery_delta","value":7}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":"done"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":1}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		``,
	}, "\n")
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sse)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	resp, events := stream(t, c, llm.Request{Model: "claude-opus-5-5"})

	assert.Equal(t, llm.StopReasonEndTurn, resp.StopReason)
	for _, e := range events {
		if e.Type == llm.EventToolCallDelta {
			assert.NotEmpty(t, e.Payload.(llm.ToolCallDeltaPayload).ID, "tool delta without ID")
		}
	}
	var text strings.Builder
	for _, b := range resp.Message.Content {
		text.WriteString(b.Text)
	}
	assert.Equal(t, "done", text.String())
}

// modelsServer answers GET /v1/models/{id} with body for each model and
// counts the calls.
func modelsServer(t *testing.T, status int, body string) (*anthropic.Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.True(t, strings.HasPrefix(r.URL.Path, "/v1/models/"), r.URL.Path)
		assert.Equal(t, "k", r.Header.Get("x-api-key"))
		assert.NotEmpty(t, r.Header.Get("anthropic-version"))
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	return anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client())), &calls
}

// B1-AC4: capability detection from the Models API, cached per model for 24 h.
func TestWebSearchCapability(t *testing.T) {
	var _ llm.Capabilities = (*anthropic.Client)(nil)

	tests := []struct {
		name    string
		status  int
		body    string
		want    bool
		wantErr bool
	}{
		{"supported", 200, `{"id":"m","capabilities":{"server_tools":{"supported":true,"web_search":{"supported":true}}}}`, true, false},
		{"unsupported", 200, `{"id":"m","capabilities":{"server_tools":{"supported":true,"web_search":{"supported":false}}}}`, false, false},
		{"missing field", 200, `{"id":"m","capabilities":{"image_input":{"supported":true}}}`, false, false},
		{"null capabilities", 200, `{"id":"m","capabilities":null}`, false, false},
		{"not found", 404, `{"type":"error","error":{"type":"not_found_error","message":"model"}}`, false, true},
		{"bad json", 200, `{`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, calls := modelsServer(t, tt.status, tt.body)
			got, err := c.WebSearch(context.Background(), "claude-opus-5-5")
			assert.Equal(t, tt.want, got)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			// Cached, including failures: no second call within 24 h.
			got, err = c.WebSearch(context.Background(), "claude-opus-5-5")
			assert.Equal(t, tt.want, got)
			assert.NoError(t, err)
			assert.Equal(t, int32(1), calls.Load())
		})
	}

	t.Run("one call per model per 24h", func(t *testing.T) {
		c, calls := modelsServer(t, 200, `{"capabilities":{"server_tools":{"web_search":{"supported":true}}}}`)
		now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
		anthropic.SetNow(c, func() time.Time { return now })

		for range 3 {
			ok, err := c.WebSearch(context.Background(), "claude-opus-5-5")
			require.NoError(t, err)
			assert.True(t, ok)
		}
		assert.Equal(t, int32(1), calls.Load())

		_, _ = c.WebSearch(context.Background(), "claude-sonnet-5-5")
		assert.Equal(t, int32(2), calls.Load(), "each model is checked once")

		now = now.Add(24*time.Hour - time.Second)
		_, _ = c.WebSearch(context.Background(), "claude-opus-5-5")
		assert.Equal(t, int32(2), calls.Load())

		now = now.Add(2 * time.Second)
		_, _ = c.WebSearch(context.Background(), "claude-opus-5-5")
		assert.Equal(t, int32(3), calls.Load(), "re-checked after 24 h")
	})
}

// B1-AC4 / D3: a 400 on a search request re-runs the turn without search and
// makes WebSearch report false for 24 h without asking the Models API.
func TestStream_WebSearchRejectedFallsBack(t *testing.T) {
	disabled, err := os.ReadFile("testdata/error_web_search_disabled.json")
	require.NoError(t, err)
	sse, err := os.ReadFile("testdata/stream_text.sse")
	require.NoError(t, err)

	var messageBodies []string
	var modelCalls atomic.Int32
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/models/") {
			modelCalls.Add(1)
			_, _ = io.WriteString(w, `{"capabilities":{"server_tools":{"web_search":{"supported":true}}}}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		messageBodies = append(messageBodies, string(b))
		if strings.Contains(string(b), "web_search_20250305") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(disabled)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	anthropic.SetNow(c, func() time.Time { return now })

	req := llm.Request{
		Model:     "claude-opus-5-5",
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "hi"}}}},
		WebSearch: &llm.WebSearch{MaxUses: 5},
	}
	resp, _ := stream(t, c, req)
	assert.Equal(t, "Hello world", resp.Message.Content[0].Text)
	require.Len(t, messageBodies, 2)
	assert.Contains(t, messageBodies[0], "web_search_20250305")
	assert.NotContains(t, messageBodies[1], "web_search")

	ok, err := c.WebSearch(context.Background(), "claude-opus-5-5")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, int32(0), modelCalls.Load())

	now = now.Add(23 * time.Hour)
	ok, _ = c.WebSearch(context.Background(), "claude-opus-5-5")
	assert.False(t, ok)
	assert.Equal(t, int32(0), modelCalls.Load())

	now = now.Add(2 * time.Hour)
	ok, _ = c.WebSearch(context.Background(), "claude-opus-5-5")
	assert.True(t, ok)
	assert.Equal(t, int32(1), modelCalls.Load())
}

// A 400 on a request without search is returned as is, not retried.
func TestStream_BadRequestWithoutSearchNotRetried(t *testing.T) {
	var calls atomic.Int32
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	out := make(chan llm.Event, 8)
	_, err := c.Stream(context.Background(), llm.Request{Model: "claude-opus-5-5"}, out)
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}
