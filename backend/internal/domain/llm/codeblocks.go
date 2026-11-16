package llm

import (
	"encoding/json"
	"strings"
)

// serverBlock is the part of a provider server block these helpers read.
type serverBlock struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id"`
}

func (b ContentBlock) server() serverBlock {
	var s serverBlock
	if b.Type == ContentTypeServer {
		_ = json.Unmarshal(b.Raw, &s)
	}
	return s
}

// IsCodeBlock reports whether b belongs to code execution: the server_tool_use
// holding the code, its result block, or a tool call made from that code.
// Web search blocks are not code blocks.
func (b ContentBlock) IsCodeBlock() bool {
	if b.Caller.FromCode() {
		return true
	}
	s := b.server()
	return strings.Contains(s.Type, "code_execution_tool_result") ||
		(s.Type == "server_tool_use" && strings.HasSuffix(s.Name, "code_execution"))
}

// HasCodeBlocks reports whether msgs replay code execution blocks.
func HasCodeBlocks(msgs []Message) bool {
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.IsCodeBlock() {
				return true
			}
		}
	}
	return false
}

// CodeTurnOpen reports whether msgs end inside a code execution turn: a code
// block was issued and its result block has not arrived. Such a turn cannot be
// followed by anything but tool results, so a conversation must not be stored
// or continued in that state.
func CodeTurnOpen(msgs []Message) bool {
	issued := map[string]bool{}
	for _, m := range msgs {
		for _, b := range m.Content {
			s := b.server()
			switch {
			case s.Type == "server_tool_use" && strings.HasSuffix(s.Name, "code_execution"):
				issued[s.ID] = true
			case strings.Contains(s.Type, "code_execution_tool_result"):
				delete(issued, s.ToolUseID)
			}
		}
	}
	return len(issued) > 0
}

// StripCodeBlocks returns msgs without code execution: the server blocks, the
// tool calls made from code and the results that answered them. A message left
// without content (or an assistant message left with thinking only) is
// dropped, as is an assistant message that lost blocks and now ends the
// conversation. Used when the request cannot carry the code execution tool
// (setting off, capability gone) or to close a code turn that was cut short.
func StripCodeBlocks(msgs []Message) []Message {
	codeCalls := map[string]bool{}
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Type == ContentTypeToolUse && b.Caller.FromCode() {
				codeCalls[b.ToolUseID] = true
			}
		}
	}
	out := make([]Message, 0, len(msgs))
	lost := false
	for i, m := range msgs {
		kept := make([]ContentBlock, 0, len(m.Content))
		for _, b := range m.Content {
			if b.IsCodeBlock() || (b.Type == ContentTypeToolResult && codeCalls[b.ToolResultID]) {
				continue
			}
			kept = append(kept, b)
		}
		lost = len(kept) < len(m.Content)
		if len(kept) == 0 || (m.Role == RoleAssistant && onlyThinking(kept)) {
			continue
		}
		if m.Role == RoleAssistant && lost && i == len(msgs)-1 {
			continue
		}
		out = append(out, Message{Role: m.Role, Content: kept})
	}
	return out
}

func onlyThinking(blocks []ContentBlock) bool {
	for _, b := range blocks {
		if t := b.server().Type; t != "thinking" && t != "redacted_thinking" {
			return false
		}
	}
	return true
}
