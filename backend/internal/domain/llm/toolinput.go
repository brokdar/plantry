package llm

import "encoding/json"

// ObjectInput returns the tool_use input to replay to a provider: the input
// itself when it is a JSON object, `{}` otherwise. An input cut at the output
// limit arrives as a JSON string holding the partial text, which providers
// reject on replay.
func ObjectInput(raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return json.RawMessage("{}")
	}
	return raw
}

// ReplayMessage returns m with every tool_use input replaced by ObjectInput,
// the form kept in the conversation history. m is not modified.
func ReplayMessage(m Message) Message {
	content := make([]ContentBlock, len(m.Content))
	copy(content, m.Content)
	for i := range content {
		if content[i].Type == ContentTypeToolUse {
			content[i].ToolUseInput = ObjectInput(content[i].ToolUseInput)
		}
	}
	m.Content = content
	return m
}
