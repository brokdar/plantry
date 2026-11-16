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
	resp, _ := stream(t, c, llm.Request{Model: "claude-sonnet-5-5", Reasoning: true})
	require.Len(t, bodies, 1)
	assert.Equal(t, map[string]any{"type": "adaptive", "display": "summarized"}, bodies[0]["thinking"])
	assert.Equal(t, 380, resp.Usage.ReasoningTokens)
	assert.Equal(t, 420, resp.Usage.OutputTokens)
	assert.JSONEq(t, `{"type":"thinking","thinking":"Summary of thought.","signature":"SIG"}`, string(resp.Message.Content[0].Raw))
}

// Without Reasoning nothing changes: no thinking, the forced tool and the
// temperature go out as asked (lookups and the import fallback rely on it).
func TestStream_NoThinkingWithoutReasoning(t *testing.T) {
	var bodies []map[string]any
	c := thinkingServer(t, modelJSON(true, 64000), &bodies)
	stream(t, c, llm.Request{Model: "m", MaxTokens: 50})
	stream(t, c, llm.Request{Model: "m", ToolChoice: &llm.ToolChoice{Name: "submit_plan"}, Temperature: 0.4})
	require.Len(t, bodies, 2)
	assert.NotContains(t, bodies[0], "thinking")
	assert.NotContains(t, bodies[1], "thinking")
	assert.Equal(t, "tool", bodies[1]["tool_choice"].(map[string]any)["type"])
	assert.InDelta(t, 0.4, bodies[1]["temperature"], 1e-9)
}

// Reasoning over a forced tool and a temperature: thinking goes out, the
// tool_choice becomes auto with the system line, the temperature is dropped,
// and the decisions are reported as fallbacks.
func TestStream_ReasoningDowngradesForcedToolAndDropsTemperature(t *testing.T) {
	var bodies []map[string]any
	c := thinkingServer(t, modelJSON(true, 64000), &bodies)
	req := llm.Request{
		Model: "m", Reasoning: true, Temperature: 0.4,
		ToolChoice: &llm.ToolChoice{Name: "submit_plan"}, System: "Plan.",
	}
	stream(t, c, req)
	require.Len(t, bodies, 1)
	assert.Equal(t, map[string]any{"type": "adaptive", "display": "summarized"}, bodies[0]["thinking"])
	assert.Equal(t, "auto", bodies[0]["tool_choice"].(map[string]any)["type"])
	assert.NotContains(t, bodies[0], "temperature")
	sys := bodies[0]["system"].([]any)[0].(map[string]any)["text"].(string)
	assert.Contains(t, sys, "Answer by calling `submit_plan`.")

	eff := c.EffectiveParams(req)
	assert.True(t, eff.Thinking)
	assert.Equal(t, "auto", eff.ToolChoice)
	assert.Nil(t, eff.Temperature)
	assert.Contains(t, eff.Fallbacks, "thinking_over_forced_tool")
	assert.Contains(t, eff.Fallbacks, "temperature_dropped_for_thinking")
}

// Reasoning on a model without thinking support changes nothing.
func TestStream_ReasoningOnUnsupportedModel(t *testing.T) {
	var bodies []map[string]any
	c := thinkingServer(t, `{"id":"m","capabilities":{"thinking":{"supported":false}}}`, &bodies)
	stream(t, c, llm.Request{Model: "m2", Reasoning: true, Temperature: 0.4, ToolChoice: &llm.ToolChoice{Name: "x"}})
	assert.NotContains(t, bodies[0], "thinking")
	assert.Equal(t, "tool", bodies[0]["tool_choice"].(map[string]any)["type"])
	assert.InDelta(t, 0.4, bodies[0]["temperature"], 1e-9)
}
