package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// ChatRuns matches ids exactly: a change under run 12 is not one under run 1
// (nor 2), and a run id anywhere in the array is found.
func TestAIRunRepo_ChatRunsMatchExactIDs(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	for _, id := range []int{1, 2, 12, 21} {
		_, err := db.Exec(`INSERT INTO ai_runs (id, kind, started_at) VALUES (?, 'chat', '2026-11-01 10:00:00')`, id)
		require.NoError(t, err)
	}
	for _, c := range []struct {
		run int
		ids string
	}{{1, "[1]"}, {2, "[2, 12]"}, {21, "[21,1]"}} {
		_, err := db.Exec(`INSERT INTO ai_changes (run_id, step_seq, tool, text_key, params_json, run_ids_json, dates_json, plate_ids_json, session_ids_json)
			VALUES (?, 1, 't', 'k', '{}', ?, '[]', '[]', '[]')`, c.run, c.ids)
		require.NoError(t, err)
	}

	got, err := repo.ChatRuns(ctx, []int64{12})
	require.NoError(t, err)
	assert.Equal(t, []int64{2}, got, "12 is in change 2 only, not in 1 or 21")
	got, err = repo.ChatRuns(ctx, []int64{1})
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 21}, got)
	got, err = repo.ChatRuns(ctx, []int64{1, 12})
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2, 21}, got)
	got, err = repo.ChatRuns(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
