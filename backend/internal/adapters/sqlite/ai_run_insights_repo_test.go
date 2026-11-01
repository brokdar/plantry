package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func createRun(t *testing.T, repo *sqlite.AIRunRepo, kind airun.Kind, started time.Time) int64 {
	t.Helper()
	id, err := repo.CreateRun(context.Background(), airun.RunStart{Kind: kind, StartedAt: started})
	require.NoError(t, err)
	return id
}

// plateEvent inserts one plate_events row; the audit trail has no FKs.
func plateEvent(t *testing.T, db *sql.DB, plateID int64, action, actor string, runID *int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO plate_events (plate_id, date, slot_id, action, actor, ai_run_id) VALUES (?, '2026-11-02', 1, ?, ?, ?)`,
		plateID, action, actor, runID)
	require.NoError(t, err)
}

func TestAIRunRepo_CorrectionsD24(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	start := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	run := createRun(t, repo, airun.KindGenerate, start)
	other := createRun(t, repo, airun.KindGenerate, start)
	untouched := createRun(t, repo, airun.KindChat, start)

	// run generates plates 1-4; the user edits plate 2 before accepting.
	for p := int64(1); p <= 4; p++ {
		plateEvent(t, db, p, "create", "generator", &run)
	}
	plateEvent(t, db, 2, "update", "user", nil)
	// Another run's agent edit is a correction (B6-AC3); the generator's own
	// updates and its own run's agent edits are not.
	plateEvent(t, db, 3, "update", "agent", &untouched)
	plateEvent(t, db, 4, "update", "generator", &run)
	plateEvent(t, db, 4, "update", "agent", &run)
	for p := int64(1); p <= 4; p++ {
		plateEvent(t, db, p, "accept", "user", &run)
	}
	// After accept, edits don't count.
	plateEvent(t, db, 1, "update", "user", nil)
	plateEvent(t, db, 3, "delete", "user", nil)

	// other: plate 5 is confirmed then edited (doesn't count), plate 6 is
	// deleted by the user while still a draft (counts).
	plateEvent(t, db, 5, "create", "generator", &other)
	plateEvent(t, db, 5, "confirm", "user", &other)
	plateEvent(t, db, 5, "update", "user", nil)
	plateEvent(t, db, 6, "create", "generator", &other)
	plateEvent(t, db, 6, "delete", "user", nil)

	got, err := repo.Corrections(ctx, run, untouched)
	require.NoError(t, err)
	assert.Equal(t, map[int64]airun.Correction{
		run:   {Generated: 4, Corrected: 2},
		other: {Generated: 2, Corrected: 1},
	}, got)
	require.NotNil(t, got[run].Rate())
	assert.InDelta(t, 0.5, *got[run].Rate(), 1e-9)

	only, err := repo.Corrections(ctx, other, other)
	require.NoError(t, err)
	assert.Equal(t, map[int64]airun.Correction{other: {Generated: 2, Corrected: 1}}, only)
}

func TestAIRunRepo_ListRunsFiltersAndCursor(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }
	c1 := createRun(t, repo, airun.KindChat, day(1))
	g2 := createRun(t, repo, airun.KindGenerate, day(2))
	c3 := createRun(t, repo, airun.KindChat, day(3))
	g4 := createRun(t, repo, airun.KindGenerate, day(4))

	ids := func(rows []airun.RunRow) []int64 {
		out := make([]int64, len(rows))
		for i, r := range rows {
			out[i] = r.ID
		}
		return out
	}
	all, err := repo.ListRuns(ctx, airun.RunQuery{Limit: -1})
	require.NoError(t, err)
	assert.Equal(t, []int64{g4, c3, g2, c1}, ids(all))
	assert.Equal(t, day(4), all[0].StartedAt)

	page, err := repo.ListRuns(ctx, airun.RunQuery{Limit: 2, BeforeID: c3})
	require.NoError(t, err)
	assert.Equal(t, []int64{g2, c1}, ids(page))

	chats, err := repo.ListRuns(ctx, airun.RunQuery{Kind: airun.KindChat, Limit: -1})
	require.NoError(t, err)
	assert.Equal(t, []int64{c3, c1}, ids(chats))

	from, to := day(2), day(4)
	ranged, err := repo.ListRuns(ctx, airun.RunQuery{From: &from, To: &to, Limit: -1})
	require.NoError(t, err)
	assert.Equal(t, []int64{c3, g2}, ids(ranged), "to is exclusive")
}

func TestAIRunRepo_GetRunWithStepsAndPrompt(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	id, err := repo.CreateRun(ctx, airun.RunStart{
		Kind: airun.KindGenerate, StartedAt: time.Now(), Chooser: airun.ChooserLLM, ScopeJSON: `{"kind":"day"}`,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertPrompt(ctx, "h1", "You plan meals."))
	require.NoError(t, repo.FinishRun(ctx, airun.RunEnd{
		ID: id, Status: airun.StatusOK, FinishedAt: time.Now(), Model: "claude-sonnet-5-5", PromptHash: "h1",
		Notes: "Two cook days.", ViolationsJSON: `[]`,
	}))
	big := `"` + strings.Repeat("x", 9000) + `"`
	// Inserted out of order: the detail lists them by seq.
	require.NoError(t, repo.AddStep(ctx, airun.Step{
		RunID: id, Seq: 2, Type: airun.StepToolCall, Name: "submit_plan",
		ArgsJSON: `{"a":1}`, ResultJSON: big, IsError: true, DurationMs: 3,
	}))
	require.NoError(t, repo.AddStep(ctx, airun.Step{
		RunID: id, Seq: 1, Type: airun.StepLLMCall, Name: "claude-sonnet-5-5",
		RequestJSON: `{"messages":[]}`, ResponseJSON: `{"content":[]}`, InputTokens: 10, OutputTokens: 5,
	}))

	run, err := repo.GetRun(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "You plan meals.", run.PromptText)
	assert.Equal(t, "Two cook days.", run.Notes)
	assert.Equal(t, `{"kind":"day"}`, run.ScopeJSON)
	assert.Equal(t, airun.ChooserLLM, run.Chooser)
	assert.NotNil(t, run.FinishedAt)

	steps, err := repo.ListSteps(ctx, id)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, airun.StepView{
		Seq: 1, Type: airun.StepLLMCall, Name: "claude-sonnet-5-5",
		Args: `{"messages":[]}`, Result: `{"content":[]}`, InputTokens: 10, OutputTokens: 5,
	}, steps[0])
	assert.Equal(t, 2, steps[1].Seq)
	assert.Equal(t, `{"a":1}`, steps[1].Args)
	assert.False(t, steps[1].ArgsTruncated)
	assert.True(t, steps[1].ResultTruncated)
	assert.LessOrEqual(t, len([]rune(steps[1].Result)), 8192)
	assert.True(t, steps[1].IsError)

	_, err = repo.GetRun(ctx, id+100)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// D11: agent events count only since the row's last accept/confirm, so an
// accepted chat run does not move again on a later unaccept or discard.
func TestAIRunRepo_AgentRunsSinceLastAccept(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	start := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	a := createRun(t, repo, airun.KindChat, start)
	b := createRun(t, repo, airun.KindChat, start)
	rows := plate.RowSet{PlateIDs: []int64{7}}

	plateEvent(t, db, 7, "update", "agent", &a)
	got, err := repo.AgentRuns(ctx, rows)
	require.NoError(t, err)
	assert.Equal(t, []int64{a}, got)

	plateEvent(t, db, 7, "accept", "user", nil)
	got, err = repo.AgentRuns(ctx, rows)
	require.NoError(t, err)
	assert.Equal(t, []int64{a}, got, "a live accept: Unaccept still reaches A")
	plateEvent(t, db, 7, "unlock", "user", nil)
	got, err = repo.AgentRuns(ctx, rows)
	require.NoError(t, err)
	assert.Empty(t, got, "A is accepted and no agent edited since")

	plateEvent(t, db, 7, "update", "agent", &b)
	got, err = repo.AgentRuns(ctx, rows)
	require.NoError(t, err)
	assert.Equal(t, []int64{b}, got, "only B moves on the discard")
}
