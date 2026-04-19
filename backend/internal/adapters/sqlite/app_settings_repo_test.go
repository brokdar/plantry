package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func newAppSettingsRepo(t *testing.T) *sqlite.AppSettingsRepo {
	t.Helper()
	return sqlite.NewAppSettingsRepo(testhelper.NewTestDB(t))
}

func TestAppSettingsRepo_Get_Missing(t *testing.T) {
	repo := newAppSettingsRepo(t)
	_, ok, err := repo.Get(context.Background(), "ai.provider")
	require.NoError(t, err)
	assert.False(t, ok, "missing key should return (_, false, nil)")
}

func TestAppSettingsRepo_UpsertGet(t *testing.T) {
	repo := newAppSettingsRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, settings.Row{
		Key: "ai.provider", Value: "openai", Encrypted: false,
	}))

	row, ok, err := repo.Get(ctx, "ai.provider")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "openai", row.Value)
	assert.False(t, row.Encrypted)
}

func TestAppSettingsRepo_UpsertReplaces(t *testing.T) {
	repo := newAppSettingsRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, settings.Row{Key: "ai.model", Value: "gpt-4"}))
	require.NoError(t, repo.Upsert(ctx, settings.Row{Key: "ai.model", Value: "claude-opus-4-7"}))

	row, ok, err := repo.Get(ctx, "ai.model")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "claude-opus-4-7", row.Value)
}

func TestAppSettingsRepo_Delete(t *testing.T) {
	repo := newAppSettingsRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, settings.Row{Key: "ai.provider", Value: "fake"}))
	require.NoError(t, repo.Delete(ctx, "ai.provider"))

	_, ok, err := repo.Get(ctx, "ai.provider")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAppSettingsRepo_Delete_Idempotent(t *testing.T) {
	repo := newAppSettingsRepo(t)
	// Deleting a row that never existed must not error — callers rely on this
	// so that "clear override" is always safe.
	require.NoError(t, repo.Delete(context.Background(), "ai.provider"))
}

func TestAppSettingsRepo_List_Ordered(t *testing.T) {
	repo := newAppSettingsRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, settings.Row{Key: "ai.provider", Value: "openai"}))
	require.NoError(t, repo.Upsert(ctx, settings.Row{Key: "ai.api_key", Value: "secret", Encrypted: true}))
	require.NoError(t, repo.Upsert(ctx, settings.Row{Key: "ai.model", Value: "gpt-4"}))

	rows, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	// List returns in key-ascending order (see ListSettings query).
	assert.Equal(t, "ai.api_key", rows[0].Key)
	assert.True(t, rows[0].Encrypted)
	assert.Equal(t, "ai.model", rows[1].Key)
	assert.Equal(t, "ai.provider", rows[2].Key)
}
