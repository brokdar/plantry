// Package agent is the AI agent aggregate: conversations, messages, and the
// tool-calling loop that ties an LLM to Plantry's domain services.
package agent

import (
	"encoding/json"
	"time"
)

// Role mirrors llm.Role but lives in this package to keep the AI aggregate
// self-contained at the domain layer. The value 'error' is agent-only: it
// records a stream failure in the transcript.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleError     Role = "error"
)

var validRoles = map[Role]bool{
	RoleSystem:    true,
	RoleUser:      true,
	RoleAssistant: true,
	RoleTool:      true,
	RoleError:     true,
}

// ValidRole reports whether r is one of the persisted roles.
func ValidRole(r Role) bool { return validRoles[r] }

// Conversation aggregates an AI chat session, optionally tied to a week.
type Conversation struct {
	ID        int64
	WeekID    *int64
	Title     *string
	Messages  []Message
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message is one persisted turn. Content is a JSON-encoded array of
// provider-neutral llm.ContentBlock values so that switching providers later
// does not invalidate history.
type Message struct {
	ID             int64
	ConversationID int64
	Role           Role
	Content        json.RawMessage
	CreatedAt      time.Time
}

// ListQuery filters + paginates a conversation list.
type ListQuery struct {
	WeekID *int64
	Limit  int
	Offset int
}

// ListResult pages conversations with a total count.
type ListResult struct {
	Items []Conversation
	Total int64
}
