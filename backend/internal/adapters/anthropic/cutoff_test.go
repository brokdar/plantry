package anthropic_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// A tool_use cut at max_tokens ends as a JSON string holding the partial
// input; replaying it must send an object, or the API answers 400.
const cutSSE = `event: message_start
data: {"type":"message_start","message":{"model":"claude-opus-5-5","usage":{"input_tokens":5,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_cut","name":"evaluate_plan","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"sessions\": [{\"key\": \"a"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}

`

func TestStream_CutToolInputReplaysAsObject(t *testing.T) {
	c, bodies := ptcServer(t, supportsCode, func(_ string, w http.ResponseWriter) { sse(w, cutSSE) })
	req := ptcRequest()
	req.CodeExecution = false
	resp, _ := stream(t, c, req)
	require.Equal(t, llm.StopReasonMaxTokens, resp.StopReason)
	use := resp.Message.Content[0]
	assert.Equal(t, `"{\"sessions\": [{\"key\": \"a"`, string(use.ToolUseInput), "the reply keeps the cut text")

	req.Messages = append(req.Messages, resp.Message, llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{
		{Type: llm.ContentTypeToolResult, ToolResultID: "toolu_cut", ToolResultContent: json.RawMessage(`"cut"`), ToolResultIsError: true},
	}})
	stream(t, c, req)

	var body struct {
		Messages []struct {
			Content []struct {
				Type  string          `json:"type"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"messages"`
	}
	require.Len(t, bodies(), 2)
	require.NoError(t, json.Unmarshal([]byte(bodies()[1]), &body))
	var found bool
	for _, m := range body.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_use" {
				found = true
				assert.Equal(t, `{}`, string(b.Input))
			}
		}
	}
	assert.True(t, found)
}
