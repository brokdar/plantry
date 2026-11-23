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
// at most"): the wish reaches the heuristic as a time limit, and every weekday
// dinner's main, cooked there or eaten from a batch, takes at most 30 minutes.
// Without the wish the same library plans a slow weekday dinner, so the test
// tells the two apart.
func TestService_QuickWeekdayDinnersAreKept(t *testing.T) {
	plan := func(quick *planning.QuickLimit) (*svcEnv, []string) {
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
		var slow []string
		for _, p := range e.window() {
			if p.SlotID != e.dinner || p.Date.Weekday() == time.Saturday || p.Date.Weekday() == time.Sunday {
				continue
			}
			for _, c := range p.Components {
				var mins int
				var role string
				require.NoError(t, e.db.QueryRow(`SELECT COALESCE(prep_minutes,0)+COALESCE(cook_minutes,0), COALESCE(role,'') FROM foods WHERE id = ?`, c.FoodID).Scan(&mins, &role))
				if role == "main" && mins > 30 {
					slow = append(slow, p.DateString())
				}
			}
		}
		return e, slow
	}
	_, slow := plan(nil)
	require.NotEmpty(t, slow, "without the wish a weekday dinner is slow (the fixture discriminates)")

	_, slow = plan(&planning.QuickLimit{Minutes: 30, Slot: "dinner", Weekdays: []string{"mon", "tue", "wed", "thu", "fri"}})
	assert.Empty(t, slow, "every weekday dinner takes at most 30 minutes")
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
