package agent

import "context"

// Repository is the persistence port for conversations + messages.
type Repository interface {
	CreateConversation(ctx context.Context, weekID *int64, title *string) (*Conversation, error)
	GetConversation(ctx context.Context, id int64) (*Conversation, error)
	UpdateConversationTitle(ctx context.Context, id int64, title *string) (*Conversation, error)
	TouchConversation(ctx context.Context, id int64) error
	DeleteConversation(ctx context.Context, id int64) error
	ListConversations(ctx context.Context, q ListQuery) (*ListResult, error)
	AppendMessage(ctx context.Context, m *Message) error
	ListMessages(ctx context.Context, conversationID int64) ([]Message, error)
}
