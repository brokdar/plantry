package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// A run's light request is kept with the plan: the notes name it in words,
// a later run without light (a heuristic regenerate) stands under it too.
func TestService_LightIsStoredAndKept(t *testing.T) {
	e := newSvcEnv(t)
	light := planning.LightIntent{Date: mon, Scope: "meal", Slot: "dinner"}

	req := e.req()
	req.Chooser = "heuristic"
	req.Light = []planning.LightIntent{light}
	end := e.ok(e.generate(req))
	assert.Contains(t, end.Notes, "Light: Mon dinner")

	stored, err := sqlite.NewPlanLightRepo(e.db).List(e.ctx, e.start, e.start.AddDate(0, 0, 6))
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{light}, stored)

	// A regenerate of the day does not repeat the request.
	rreq := e.req()
	rreq.Chooser = "heuristic"
	rreq.Scope = planning.Scope{Kind: planning.ScopeDay, Date: day(mon)}
	end = e.ok(func() int64 {
		id, err := e.svc.Regenerate(e.ctx, rreq)
		require.NoError(t, err)
		return id
	}())
	assert.Contains(t, end.Notes, "Light: Mon dinner")

	st, err := e.deps.Builder.Build(e.ctx, window.Window{Start: e.start, End: e.start.AddDate(0, 0, 6)}, day(svcToday), e.start, e.start.AddDate(0, 0, 6))
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{light}, st.Light)
}
