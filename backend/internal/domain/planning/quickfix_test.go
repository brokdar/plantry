package planning_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// V72 T1a (tune-v2 g2-planner-07, "quick dinners Monday to Friday, 30 minutes
// at most"): the wish reaches the heuristic as a time limit, and every main
// cooked at a weekday dinner takes at most 30 minutes (a leftover eaten there
// costs no time, V73).
// Without the wish the same library plans a slow weekday dinner, so the test
// tells the two apart.
func TestService_QuickWeekdayDinnersAreKept(t *testing.T) {
	plan := func(quick *planning.QuickLimit) (int, []string) {
		e := newSvcEnv(t)
		for i, id := range e.mains {
			mins := 60
			if i >= 8 {
				mins = 20 // Main 09 and 10: quick, the last by id, not eaten recently
			}
			e.exec(`UPDATE foods SET prep_minutes = ? WHERE id = ?`, mins, id)
		}
		e.deps.Extractor = &fakeExtractor{c: planning.Constraints{Quick: quick}}
		e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: planning.Heuristic{}}
		e.svc = planning.NewService(e.deps)
		req := e.req()
		req.Chooser, req.Instruction = airun.ChooserLLM, "quick dinners Monday to Friday, 30 minutes at most"
		end := e.runs.end(e.generate(req))
		require.NotEqual(t, airun.StatusError, end.Status, end.ErrorKey)
		// A leftover costs no time at the dinner that eats it: only a dinner
		// that cooks a slow main breaks the wish.
		var slow []string
		rows, err := e.db.Query(`SELECT cs.cook_date FROM cook_sessions cs JOIN foods f ON f.id = cs.food_id
			WHERE cs.cook_slot_id = ? AND f.role = 'main' AND COALESCE(f.prep_minutes,0)+COALESCE(f.cook_minutes,0) > 30
			AND cs.cook_date BETWEEN ? AND ? AND strftime('%w', cs.cook_date) BETWEEN '1' AND '5'`,
			e.dinner, e.start.Format(time.DateOnly), e.start.AddDate(0, 0, 6).Format(time.DateOnly))
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var d string
			require.NoError(t, rows.Scan(&d))
			slow = append(slow, d)
		}
		require.NoError(t, rows.Err())
		var leftovers int
		require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM plate_components pc JOIN plates p ON p.id = pc.plate_id
			JOIN cook_sessions cs ON cs.id = pc.cook_session_id JOIN foods f ON f.id = pc.food_id
			WHERE p.slot_id = ? AND f.role = 'main' AND strftime('%w', p.date) BETWEEN '1' AND '5'
			AND (cs.cook_date <> p.date OR cs.cook_slot_id <> p.slot_id)`, e.dinner).Scan(&leftovers))
		return leftovers, slow
	}
	leftoversBefore, slow := plan(nil)
	require.NotEmpty(t, slow, "without the wish a weekday dinner is slow (the fixture discriminates)")
	require.Positive(t, leftoversBefore, "the fixture eats leftovers at weekday dinners")

	leftovers, slow := plan(&planning.QuickLimit{Minutes: 30, Slot: "dinner", Weekdays: []string{"mon", "tue", "wed", "thu", "fri"}})
	assert.Empty(t, slow, "every weekday dinner takes at most 30 minutes")
	assert.Positive(t, leftovers, "a leftover costs no time at the dinner: the wish keeps leftover dinners (V73)")
}

// The extraction schema carries the limit and ParseConstraints keeps it when it
// names planned cells.
func TestParseConstraints_QuickLimit(t *testing.T) {
	e := newSvcEnv(t)
	st, err := e.deps.Builder.Build(e.ctx, window.Window{Start: e.start, End: e.start.AddDate(0, 0, 6)}, day(svcToday), e.start, e.start.AddDate(0, 0, 6))
	require.NoError(t, err)
	c := planning.ParseConstraints(st, []byte(`{"quick":{"minutes":30,"slot":"dinner","weekdays":["mon","tue","wed","thu","fri"]}}`))
	require.NotNil(t, c.Quick)
	assert.Equal(t, 30, c.Quick.Minutes)
	assert.Empty(t, c.Ignored)
	none := planning.ParseConstraints(st, []byte(`{"quick":{"minutes":30,"slot":"supper"}}`))
	assert.Nil(t, none.Quick)
	assert.NotEmpty(t, none.Ignored, "a limit that names no planned cell is said, not dropped silently")
}
