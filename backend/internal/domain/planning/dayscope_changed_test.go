package planning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// A plate of the day confirmed between the scope's resolve and the run's
// start ends the run as a changed plan, not a generic error.
func TestService_DayScopePlateConfirmedBeforeRunStart(t *testing.T) {
	e := newDayEnv(t)
	e.ok(e.generate(e.req()))
	sh := findDayShape(t, e, e.window())
	var run func()
	e.deps.Go = func(f func()) { run = f }
	e.svc = planning.NewService(e.deps)

	id := e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: day(sh.date)})
	_, err := e.plates.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{sh.left.ID}})
	require.NoError(t, err)
	run()

	end := e.runs.end(id)
	assert.Equal(t, airun.StatusError, end.Status)
	assert.Equal(t, "error.plan.changed_during_run", end.ErrorKey)
}

// dayMeddler deletes a later plate of the batch the day cooks while the
// chooser runs: the save's removal then differs from the dry run's.
type dayMeddler struct {
	e  *svcEnv
	id int64
}

func (m dayMeddler) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	_, err := m.e.plates.Delete(m.e.ctx, m.id)
	require.NoError(m.e.t, err)
	return planning.Heuristic{}.Choose(ctx, in)
}

func TestService_DayScopeBatchChangedBetweenDryRunAndSave(t *testing.T) {
	e := newDayEnv(t)
	e.ok(e.generate(e.req()))
	before := e.window()
	sh := findDayShape(t, e, before)
	d := day(sh.date)
	var other int64
	for _, l := range sh.cookBatch.Links {
		if l.Cell.Date.After(d) {
			other = l.PlateID
			break
		}
	}
	require.NotZero(t, other)
	beforeSessions, _, err := e.deps.Sessions.ListInRange(e.ctx, e.start, e.start.AddDate(0, 0, 6))
	require.NoError(t, err)
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: dayMeddler{e, other}}
	e.svc = planning.NewService(e.deps)

	id := e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: d})

	end := e.runs.end(id)
	assert.Equal(t, airun.StatusError, end.Status)
	assert.Equal(t, "error.plan.changed_during_run", end.ErrorKey)
	after := e.window()
	assert.Len(t, after, len(before)-1, "only the user's delete changed the plan")
	for k, p := range before {
		if p.ID != other {
			assert.Equal(t, p.ID, after[k].ID, k)
		}
	}
	afterSessions, _, err := e.deps.Sessions.ListInRange(e.ctx, e.start, e.start.AddDate(0, 0, 6))
	require.NoError(t, err)
	assert.Len(t, afterSessions, len(beforeSessions), "the save wrote nothing")
}
