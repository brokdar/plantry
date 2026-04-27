package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestAIRepo_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRepo(db)

	title := "Plan dinner"
	c, err := repo.CreateConversation(ctx, nil, &title)
	require.NoError(t, err)
	assert.NotZero(t, c.ID)
	assert.Equal(t, "Plan dinner", *c.Title)
	assert.Nil(t, c.WeekID)
	assert.Empty(t, c.Messages)
	assert.False(t, c.CreatedAt.IsZero())

	got, err := repo.GetConversation(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
}

func TestAIRepo_GetMissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewAIRepo(testhelper.NewTestDB(t))

	_, err := repo.GetConversation(ctx, 999)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestAIRepo_AppendAndListMessages(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewAIRepo(testhelper.NewTestDB(t))

	c, err := repo.CreateConversation(ctx, nil, nil)
	require.NoError(t, err)

	userMsg := &agent.Message{
		ConversationID: c.ID,
		Role:           agent.RoleUser,
		Content:        json.RawMessage(`[{"type":"text","text":"plan tuesday"}]`),
	}
	require.NoError(t, repo.AppendMessage(ctx, userMsg))
	assert.NotZero(t, userMsg.ID)
	assert.False(t, userMsg.CreatedAt.IsZero())

	assistant := &agent.Message{
		ConversationID: c.ID,
		Role:           agent.RoleAssistant,
		Content:        json.RawMessage(`[{"type":"text","text":"ok"}]`),
	}
	require.NoError(t, repo.AppendMessage(ctx, assistant))

	msgs, err := repo.ListMessages(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, agent.RoleUser, msgs[0].Role)
	assert.Equal(t, agent.RoleAssistant, msgs[1].Role)
	assert.JSONEq(t, `[{"type":"text","text":"plan tuesday"}]`, string(msgs[0].Content))

	got, err := repo.GetConversation(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, got.Messages, 2)
}

func TestAIRepo_AppendMessageInvalidRole(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewAIRepo(testhelper.NewTestDB(t))
	c, err := repo.CreateConversation(ctx, nil, nil)
	require.NoError(t, err)

	err = repo.AppendMessage(ctx, &agent.Message{
		ConversationID: c.ID,
		Role:           "bogus",
		Content:        json.RawMessage(`[]`),
	})
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestAIRepo_AppendMessageUnknownConversation(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewAIRepo(testhelper.NewTestDB(t))

	err := repo.AppendMessage(ctx, &agent.Message{
		ConversationID: 9999,
		Role:           agent.RoleUser,
		Content:        json.RawMessage(`[]`),
	})
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestAIRepo_DeleteCascadesMessages(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewAIRepo(testhelper.NewTestDB(t))

	c, err := repo.CreateConversation(ctx, nil, nil)
	require.NoError(t, err)
	require.NoError(t, repo.AppendMessage(ctx, &agent.Message{
		ConversationID: c.ID,
		Role:           agent.RoleUser,
		Content:        json.RawMessage(`[]`),
	}))

	require.NoError(t, repo.DeleteConversation(ctx, c.ID))

	_, err = repo.GetConversation(ctx, c.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))

	err = repo.DeleteConversation(ctx, c.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestAIRepo_List_FiltersByWeek(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRepo(db)

	// weeks table is gone (migration 16); week_id on ai_conversations is now a
	// plain nullable INTEGER. Use an arbitrary sentinel value.
	weekID := int64(17)

	_, err := repo.CreateConversation(ctx, &weekID, nil)
	require.NoError(t, err)
	_, err = repo.CreateConversation(ctx, &weekID, nil)
	require.NoError(t, err)
	_, err = repo.CreateConversation(ctx, nil, nil)
	require.NoError(t, err)

	all, err := repo.ListConversations(ctx, agent.ListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), all.Total)
	assert.Len(t, all.Items, 3)

	byWeek, err := repo.ListConversations(ctx, agent.ListQuery{WeekID: &weekID})
	require.NoError(t, err)
	assert.Equal(t, int64(2), byWeek.Total)
	assert.Len(t, byWeek.Items, 2)
	for _, c := range byWeek.Items {
		require.NotNil(t, c.WeekID)
		assert.Equal(t, weekID, *c.WeekID)
	}
}

func TestAIRepo_UpdateTitleAndTouch(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewAIRepo(testhelper.NewTestDB(t))

	c, err := repo.CreateConversation(ctx, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, c.Title)

	newTitle := "Renamed"
	updated, err := repo.UpdateConversationTitle(ctx, c.ID, &newTitle)
	require.NoError(t, err)
	require.NotNil(t, updated.Title)
	assert.Equal(t, "Renamed", *updated.Title)

	require.NoError(t, repo.TouchConversation(ctx, c.ID))

	err = repo.TouchConversation(ctx, 9999)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}
