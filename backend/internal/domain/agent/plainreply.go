package agent

import (
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plainwords"
)

// plainReply is the last line of defence for the words a household reads (see
// plainwords.Reply, shared with the planner's saved note).
func plainReply(s string) string { return plainwords.Reply(s) }

// plainReplyMessage rewrites the text blocks of the model's final message.
func plainReplyMessage(m *llm.Message) {
	for i := range m.Content {
		if m.Content[i].Type == llm.ContentTypeText {
			m.Content[i].Text = plainReply(m.Content[i].Text)
		}
	}
}
