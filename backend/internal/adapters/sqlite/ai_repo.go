package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
)

// AIRepo implements agent.Repository backed by SQLite.
type AIRepo struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// NewAIRepo creates a SQLite-backed AI repository.
func NewAIRepo(db *sql.DB) *AIRepo {
	return &AIRepo{db: db, q: sqlcgen.New(db)}
}

func (r *AIRepo) CreateConversation(ctx context.Context, weekID *int64, title *string) (*agent.Conversation, error) {
	row, err := r.q.CreateConversation(ctx, sqlcgen.CreateConversationParams{
		WeekID: toNullInt64(weekID),
		Title:  toNullString(title),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return nil, fmt.Errorf("%w: week reference", domain.ErrInvalidInput)
		}
		return nil, err
	}
	c := &agent.Conversation{}
	mapConversationToDomain(&row, c)
	c.Messages = []agent.Message{}
	return c, nil
}

func (r *AIRepo) GetConversation(ctx context.Context, id int64) (*agent.Conversation, error) {
	row, err := r.q.GetConversation(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: conversation %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	c := &agent.Conversation{}
	mapConversationToDomain(&row, c)
	msgs, err := r.ListMessages(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Messages = msgs
	return c, nil
}

func (r *AIRepo) UpdateConversationTitle(ctx context.Context, id int64, title *string) (*agent.Conversation, error) {
	row, err := r.q.UpdateConversationTitle(ctx, sqlcgen.UpdateConversationTitleParams{
		Title: toNullString(title),
		ID:    id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: conversation %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	c := &agent.Conversation{}
	mapConversationToDomain(&row, c)
	msgs, err := r.ListMessages(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Messages = msgs
	return c, nil
}

func (r *AIRepo) TouchConversation(ctx context.Context, id int64) error {
	res, err := r.q.TouchConversation(ctx, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: conversation %d", domain.ErrNotFound, id)
	}
	return nil
}

func (r *AIRepo) DeleteConversation(ctx context.Context, id int64) error {
	res, err := r.q.DeleteConversation(ctx, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: conversation %d", domain.ErrNotFound, id)
	}
	return nil
}

func (r *AIRepo) ListConversations(ctx context.Context, q agent.ListQuery) (*agent.ListResult, error) {
	limit, offset := clampPagination(q.Limit, q.Offset, 25, 100)
	var (
		rows  []sqlcgen.AiConversation
		total int64
		err   error
	)
	if q.WeekID != nil {
		rows, err = r.q.ListConversationsByWeek(ctx, sqlcgen.ListConversationsByWeekParams{
			WeekID: toNullInt64(q.WeekID),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, err
		}
		total, err = r.q.CountConversationsByWeek(ctx, toNullInt64(q.WeekID))
		if err != nil {
			return nil, err
		}
	} else {
		rows, err = r.q.ListConversations(ctx, sqlcgen.ListConversationsParams{
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, err
		}
		total, err = r.q.CountConversations(ctx)
		if err != nil {
			return nil, err
		}
	}
	out := make([]agent.Conversation, len(rows))
	for i := range rows {
		mapConversationToDomain(&rows[i], &out[i])
	}
	return &agent.ListResult{Items: out, Total: total}, nil
}

func (r *AIRepo) AppendMessage(ctx context.Context, m *agent.Message) error {
	if !agent.ValidRole(m.Role) {
		return fmt.Errorf("%w: role %q", domain.ErrInvalidInput, m.Role)
	}
	if len(m.Content) == 0 {
		m.Content = json.RawMessage("[]")
	}
	row, err := r.q.AppendMessage(ctx, sqlcgen.AppendMessageParams{
		ConversationID: m.ConversationID,
		Role:           string(m.Role),
		Content:        string(m.Content),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: conversation %d", domain.ErrNotFound, m.ConversationID)
		}
		return err
	}
	mapMessageToDomain(&row, m)
	// Touch the parent so list ordering reflects recent activity.
	if _, err := r.q.TouchConversation(ctx, m.ConversationID); err != nil {
		return err
	}
	return nil
}

func (r *AIRepo) ListMessages(ctx context.Context, conversationID int64) ([]agent.Message, error) {
	rows, err := r.q.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	out := make([]agent.Message, len(rows))
	for i := range rows {
		mapMessageToDomain(&rows[i], &out[i])
	}
	return out, nil
}

func mapConversationToDomain(row *sqlcgen.AiConversation, c *agent.Conversation) {
	c.ID = row.ID
	if row.WeekID.Valid {
		v := row.WeekID.Int64
		c.WeekID = &v
	}
	if row.Title.Valid {
		v := row.Title.String
		c.Title = &v
	}
	c.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt) //nolint:errcheck // layout is controlled by our migration
	c.UpdatedAt, _ = time.Parse(timeLayout, row.UpdatedAt) //nolint:errcheck
}

func mapMessageToDomain(row *sqlcgen.AiMessage, m *agent.Message) {
	m.ID = row.ID
	m.ConversationID = row.ConversationID
	m.Role = agent.Role(row.Role)
	m.Content = json.RawMessage(row.Content)
	m.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt) //nolint:errcheck
}

func clampPagination(limit, offset, defLimit, maxLimit int) (int, int) {
	if limit <= 0 {
		limit = defLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
