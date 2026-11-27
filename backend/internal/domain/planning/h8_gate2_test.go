package planning_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// CP2 regression (cuisine-explorer, variety_weeks 4, Chef on): GATE-2 blocks
// every recipe over the weekday time allowance on weekdays, so the weekdays
// can only take the quick mains, all eaten recently. H8 counted the slow
// recipes as choices for those weekdays, so the repeats stayed violations,
// and the variety repair swapped slow mains into weekdays past GATE-2 (or,
// where it found none, nothing was saved). A slow recipe is a choice only
// for the window's weekend cooks: here the pool is too small, H8 relaxes and
// no weekday cooks a slow main.
func TestService_GateTwoSlowRecipesDoNotBlockVarietyRelaxation(t *testing.T) {
	e := newSvcEnv(t)
	set := testsvc.Settings(e.db)
	require.NoError(t, set.Set(e.ctx, "chef.enabled", "true"))
	require.NoError(t, set.Set(e.ctx, "plan.batch_cooking", "false"))
	e.exec(`UPDATE user_profile SET minutes_weekday = 30, minutes_weekend = 60 WHERE id = 1`)
	for i, id := range e.mains {
		mins := 60
		if i < 3 {
			mins = 20 // Main 01-03: quick, and eaten in the last two weeks
		}
		e.exec(`UPDATE foods SET prep_minutes = ? WHERE id = ?`, mins, id)
	}

	end := e.runs.end(e.generate(e.req()))

	require.NotEqual(t, airun.StatusError, end.Status, "notes %q key %q violations %s", end.Notes, end.ErrorKey, end.ViolationsJSON)
	assert.NotContains(t, end.ViolationsJSON, `"variety_repeat"`)
	var slowWeekday []string
	rows, err := e.db.Query(`SELECT cs.cook_date FROM cook_sessions cs JOIN foods f ON f.id = cs.food_id
		WHERE f.role = 'main' AND f.prep_minutes > 30 AND cs.cook_date BETWEEN ? AND ? AND strftime('%w', cs.cook_date) BETWEEN '1' AND '5'`,
		e.start.Format(time.DateOnly), e.start.AddDate(0, 0, 6).Format(time.DateOnly))
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var d string
		require.NoError(t, rows.Scan(&d))
		slowWeekday = append(slowWeekday, d)
	}
	require.NoError(t, rows.Err())
	assert.Empty(t, slowWeekday, "GATE-2: no weekday cooks a main over the weekday allowance")
}
