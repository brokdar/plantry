package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/openai"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// A tool call whose input was cut (a JSON string holding partial text) is
// replayed with `{}` as its arguments, which must be a valid JSON object.
func TestStream_CutToolInputReplaysAsObjectArguments(t *testing.T) {
	var body []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(ts.Close)
	c := openai.New("k", openai.WithBaseURL(ts.URL), openai.WithHTTPClient(ts.Client()))

	req := llm.Request{Model: "gpt-5.4-mini", Messages: []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "hi"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{
			Type: llm.ContentTypeToolUse, ToolUseID: "call_1", ToolUseName: "evaluate_plan",
			ToolUseInput: json.RawMessage(`"{\"sessions\": [{"`),
		}}},
		{Role: llm.RoleUser, Content: []llm.ContentBlock{{
			Type: llm.ContentTypeToolResult, ToolResultID: "call_1", ToolResultContent: json.RawMessage(`"cut"`), ToolResultIsError: true,
		}}},
	}}
	out := make(chan llm.Event, 64)
	_, err := c.Stream(context.Background(), req, out)
	require.NoError(t, err)
	drainEvents(out)

	var sent struct {
		Messages []struct {
			ToolCalls []struct {
				Function struct {
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &sent))
	var found bool
	for _, m := range sent.Messages {
		for _, tc := range m.ToolCalls {
			found = true
			assert.Equal(t, `{}`, tc.Function.Arguments)
		}
	}
	assert.True(t, found)
}
