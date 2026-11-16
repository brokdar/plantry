package llm_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func TestObjectInput(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		``:                    `{}`,
		`{}`:                  `{}`,
		`{"a":1}`:             `{"a":1}`,
		`"{\"sessions\": [{"`: `{}`,
		`[1]`:                 `{}`,
		`null`:                `{}`,
		`{"sessions": [{"a":`: `{}`,
	} {
		assert.Equal(t, want, string(llm.ObjectInput(json.RawMessage(in))), in)
	}
}

func TestReplayMessage_LeavesTheOriginal(t *testing.T) {
	t.Parallel()
	cut := json.RawMessage(`"{\"sessions\": [{"`)
	m := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
		{Type: llm.ContentTypeText, Text: "x"},
		{Type: llm.ContentTypeToolUse, ToolUseID: "t", ToolUseInput: cut},
	}}
	got := llm.ReplayMessage(m)
	assert.Equal(t, `{}`, string(got.Content[1].ToolUseInput))
	assert.Equal(t, string(cut), string(m.Content[1].ToolUseInput))
}
