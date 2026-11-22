package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// B-3/KR1 and the universal "no new hard violation": replanning Wed 9 Dec of
// the stored tune2 g-replan-12 plan removed the plates that cook "Linsen-
// Bolognese" and "Marinated tofu" for Thursday lunch; R1 moved both cooks to
// Thursday lunch, which already cooks at dinner, and the run saved a day with
// two cooking times (H2). The batches now stay cooked on Wednesday dinner, for
// the Thursday lunch that eats them, and Thursday keeps its one cooking time.
func TestRegenerateDay_NeverSavesAnH2WhereAMovedCookLands(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)
	e := replayEnvAt(t, m3DevStartDB, start)
	today := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	cookTimes := func() int {
		st, err := testsvc.PlanState(e.db)(context.Background(), window.Window{Start: start, End: start.AddDate(0, 0, 6)}, today)
		require.NoError(t, err)
		for _, d := range planning.Evaluate(st, planning.WindowTarget(st)).Days {
			if d.Date == "2026-12-10" {
				return d.CookOccasions
			}
		}
		t.Fatal("no Thursday in the evaluation")
		return 0
	}
	require.Equal(t, 1, cookTimes(), "the stored Thursday cooks once")

	ctx := context.Background()
	day := time.Date(2026, 12, 9, 0, 0, 0, 0, time.UTC)
	id, err := e.planner.Regenerate(ctx, planning.Request{
		Start: start, Today: today,
		Scope: planning.Scope{Kind: planning.ScopeDay, Date: day}, Chooser: airun.ChooserHeuristic,
	})
	require.NoError(t, err)
	v, err := e.planner.Run(ctx, id)
	require.NoError(t, err)

	assert.Equal(t, 1, cookTimes(), "Thursday still cooks once (run %s %s: %s)", v.Status, v.ErrorKey, v.Notes)
	assert.Contains(t, []airun.Status{airun.StatusOK, airun.StatusPartial}, v.Status, v.ErrorKey)
	assert.NotContains(t, v.Notes, "now cooked Thursday")
	for _, sid := range []int64{120, 121} {
		var date string
		var slot int64
		require.NoError(t, e.db.QueryRow(`SELECT cook_date, cook_slot_id FROM cook_sessions WHERE id = ?`, sid).Scan(&date, &slot))
		assert.Equal(t, "2026-12-09", date[:10], "session %d stays cooked on Wednesday", sid)
		assert.Equal(t, int64(3), slot)
	}
}
