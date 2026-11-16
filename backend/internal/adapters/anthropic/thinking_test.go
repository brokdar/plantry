package anthropic_test

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func thinkingServer(t *testing.T, modelBody string, bodies *[]map[string]any) *anthropic.Client {
	t.Helper()
	sse, err := os.ReadFile("testdata/stream_thinking_summary.sse")
	require.NoError(t, err)
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(modelBody))
			return
		}
		var m map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&m))
		*bodies = append(*bodies, m)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
	})
	return anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
}

// A model that supports thinking gets adaptive thinking with a summarized
// display, and thinking_tokens from the final message_delta become
// ReasoningTokens; the summary block is kept verbatim.
func TestStream_AdaptiveThinkingSummarized(t *testing.T) {
	var bodies []map[string]any
	c := thinkingServer(t, modelJSON(true, 64000), &bodies)
	resp, _ := stream(t, c, llm.Request{Model: "claude-sonnet-5-5"})
	require.Len(t, bodies, 1)
	assert.Equal(t, map[string]any{"type": "adaptive", "display": "summarized"}, bodies[0]["thinking"])
	assert.Equal(t, 380, resp.Usage.ReasoningTokens)
	assert.Equal(t, 420, resp.Usage.OutputTokens)
	assert.JSONEq(t, `{"type":"thinking","thinking":"Summary of thought.","signature":"SIG"}`, string(resp.Message.Content[0].Raw))
}

// A forced tool (allowed only where thinking can be off), a temperature and
// models without thinking support send no thinking parameter.
func TestStream_NoThinkingWhenForcedOrUnsupported(t *testing.T) {
	var bodies []map[string]any
	c := thinkingServer(t, modelJSON(true, 64000), &bodies)
	stream(t, c, llm.Request{Model: "m", ToolChoice: &llm.ToolChoice{Name: "submit_plan"}})
	require.Len(t, bodies, 1)
	assert.Equal(t, "tool", bodies[0]["tool_choice"].(map[string]any)["type"])
	assert.NotContains(t, bodies[0], "thinking")

	bodies = nil
	c = thinkingServer(t, `{"id":"m","capabilities":{"thinking":{"supported":false}}}`, &bodies)
	stream(t, c, llm.Request{Model: "m2"})
	assert.NotContains(t, bodies[0], "thinking")

	bodies = nil
	c = thinkingServer(t, modelJSON(true, 64000), &bodies)
	stream(t, c, llm.Request{Model: "m3", Temperature: 0.4})
	assert.NotContains(t, bodies[0], "thinking")
}
