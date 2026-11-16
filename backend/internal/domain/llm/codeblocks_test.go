package llm_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func server(raw string) llm.ContentBlock {
	return llm.ContentBlock{Type: llm.ContentTypeServer, Raw: json.RawMessage(raw)}
}

func text(role llm.Role, s string) llm.Message {
	return llm.Message{Role: role, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: s}}}
}

var (
	codeUse    = `{"type":"server_tool_use","id":"srv1","name":"code_execution","input":{}}`
	codeResult = `{"type":"code_execution_tool_result","tool_use_id":"srv1","content":{}}`
	searchUse  = `{"type":"server_tool_use","id":"srv2","name":"web_search","input":{}}`
	codeCall   = llm.ContentBlock{
		Type: llm.ContentTypeToolUse, ToolUseID: "tu1", ToolUseName: "find_foods",
		Caller: &llm.Caller{Type: llm.CallerCodeExecution, ToolID: "srv1"},
	}
	codeAnswer = llm.ContentBlock{Type: llm.ContentTypeToolResult, ToolResultID: "tu1"}
)

func TestCodeTurnOpen(t *testing.T) {
	paused := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{server(codeUse), codeCall}}
	answer := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{codeAnswer}}
	done := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{server(codeResult), {Type: llm.ContentTypeText, Text: "ok"}}}

	assert.True(t, llm.CodeTurnOpen([]llm.Message{paused}))
	assert.True(t, llm.CodeTurnOpen([]llm.Message{paused, answer}), "answered calls do not close the code")
	assert.False(t, llm.CodeTurnOpen([]llm.Message{paused, answer, done}))
	assert.False(t, llm.CodeTurnOpen([]llm.Message{{Role: llm.RoleAssistant, Content: []llm.ContentBlock{server(searchUse)}}}), "web search is not code")
}

func TestStripCodeBlocks(t *testing.T) {
	paused := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
		{Type: llm.ContentTypeText, Text: "thinking about it"}, server(codeUse), codeCall,
	}}
	answer := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{codeAnswer}}
	done := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{server(codeResult), {Type: llm.ContentTypeText, Text: "ok"}}}
	search := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{server(searchUse)}}

	got := llm.StripCodeBlocks([]llm.Message{text(llm.RoleUser, "q"), paused, answer, done, search, text(llm.RoleUser, "next")})

	require.Len(t, got, 5)
	assert.Equal(t, "thinking about it", got[1].Content[0].Text)
	assert.Len(t, got[1].Content, 1)
	assert.Equal(t, "ok", got[2].Content[0].Text)
	assert.Len(t, got[3].Content, 1, "web search blocks stay")
	assert.False(t, llm.HasCodeBlocks(got))
}

func TestStripCodeBlocks_DropsEmptyAndTrailingLostAssistant(t *testing.T) {
	onlyCode := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{server(codeUse), codeCall}}
	answer := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{codeAnswer}}
	got := llm.StripCodeBlocks([]llm.Message{text(llm.RoleUser, "q"), onlyCode, answer})
	require.Len(t, got, 1)

	partial := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "t"}, server(codeUse)}}
	got = llm.StripCodeBlocks([]llm.Message{text(llm.RoleUser, "q"), partial})
	require.Len(t, got, 1, "an assistant message that lost blocks must not end the conversation")
}
