package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Anthropic rejects a tool_result whose content is a JSON object or array
// (400 "Found an object, but tool_result content must either be a string or
// a list of content blocks"), so the wire content is always a string.
func TestEncodeContent_ToolResultWireShape(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"object", `{"foods":[{"id":1}],"total":1}`, `{"foods":[{"id":1}],"total":1}`},
		{"array", `[1,2,3]`, `[1,2,3]`},
		{"string", `"plain text"`, `plain text`},
		{"empty", ``, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := encodeContent([]llm.ContentBlock{{
				Type: llm.ContentTypeToolResult, ToolResultID: "tu_1",
				ToolResultContent: json.RawMessage(tt.in),
			}})
			raw, err := json.Marshal(blocks)
			if err != nil {
				t.Fatal(err)
			}
			var wire []struct {
				Type    string `json:"type"`
				Content any    `json:"content"`
			}
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			if len(wire) != 1 || wire[0].Type != "tool_result" {
				t.Fatalf("wire = %s", raw)
			}
			got, ok := wire[0].Content.(string)
			if !ok {
				t.Fatalf("content is %T, want string: %s", wire[0].Content, raw)
			}
			if got != tt.want {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
		})
	}
}
