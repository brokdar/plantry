package sqlite_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// DoD walk F4 / AC-4.2: a chat run's edits include the rows a regenerate it
// started wrote (generator events under the regenerate run, linked through
// ai_changes), counted once with its own agent edits.
func TestAIRunRepo_EditsIncludeLinkedRegenerateRun(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	start := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	chat := createRun(t, repo, airun.KindChat, start)
	regen := createRun(t, repo, airun.KindRegenerate, start)
	otherChat := createRun(t, repo, airun.KindChat, start)

	plateEvent(t, db, 1, "update", "agent", &chat) // the chat's own edit of plate 1
	var before int64
	require.NoError(t, db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM plate_events`).Scan(&before))
	plateEvent(t, db, 1, "update", "generator", &regen) // the regenerate touches plates 1 and 2
	plateEvent(t, db, 2, "update", "generator", &regen)
	var last int64
	require.NoError(t, db.QueryRow(`SELECT MAX(id) FROM plate_events`).Scan(&last))
	plateEvent(t, db, 3, "update", "generator", &regen) // after the change: not part of it
	_, err := db.Exec(`INSERT INTO ai_changes (run_id, step_seq, tool, text_key, params_json, run_ids_json, dates_json, plate_ids_json, session_ids_json, first_event_id, last_event_id)
		VALUES (?, 1, 'regenerate', 'k', '{}', ?, '[]', '[]', '[]', ?, ?)`, chat, fmt.Sprintf("[%d,%d]", chat, regen), before+1, last)
	require.NoError(t, err)
	plateEvent(t, db, 9, "update", "agent", &otherChat)

	got, err := repo.Edits(ctx, chat, otherChat)
	require.NoError(t, err)
	assert.Equal(t, 2, got[chat], "plates 1 (once) and 2")
	assert.Equal(t, 1, got[otherChat])
	_, ok := got[regen]
	assert.False(t, ok, "the generator run itself has no agent edits")
}
