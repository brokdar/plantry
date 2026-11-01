package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// The DoD walkthrough's S1 and S2: a re-plan that removes an uncooked
// batch's cook plate moves the cook only to a free day inside the range,
// with no freezer plates; otherwise it drops the batch with every plate
// eating it, and the preview lists them all.

// boardIssues returns the codes of the window's board issues.
func boardIssues(t *testing.T, e *replanEnv) []string {
	t.Helper()
	start := day(sat)
	p, err := testsvc.PlanView(e.db).Plan(e.ctx, &start, day(wed))
	require.NoError(t, err)
	var codes []string
	for _, is := range p.Issues {
		codes = append(codes, is.Code)
	}
	return codes
}

// S1: the batch's first remaining plate is on a day that already cooks, so
// moving it there would be a hidden H2. The batch goes instead.
func TestReplan_OrphanOnABusyDayIsDropped(t *testing.T) {
	e, cook, q := orphanEnv(t)
	sid := *cook.Components[0].CookSessionID
	friDinner := e.userPlate(fri, e.dinner, e.mains[6]) // Friday already cooks
	friLunch := e.window()[key(fri, e.lunch)]

	pv, err := e.svc.PreviewReplan(e.ctx, q)

	require.NoError(t, err)
	assert.Empty(t, pv.Moved)
	assert.Equal(t, []planning.DroppedPlate{{
		SessionID: sid, FoodName: "Main 05", PlateID: friLunch.ID, Date: day(fri), SlotID: e.lunch, Confirmed: true,
	}}, pv.Dropped)
	ids := []int64{}
	for _, a := range pv.ConfirmedAffected {
		ids = append(ids, a.PlateID)
	}
	assert.ElementsMatch(t, []int64{cook.ID, friLunch.ID}, ids, "the dropped plate is confirmed, so it needs unlock")
	assert.Empty(t, pv.Spare, "a dropped batch has no spare")

	var ce *planning.ConfirmedAffectedError
	_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: q, Spare: planning.SpareKeep})
	require.ErrorAs(t, err, &ce)
	assert.Len(t, ce.Plates, 2)

	_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: q, UnlockConfirmed: true, Spare: planning.SpareKeep})

	require.NoError(t, err)
	assert.True(t, e.plateGone(cook.ID))
	assert.True(t, e.plateGone(friLunch.ID), "left without components")
	_, err = e.plates.Session(e.ctx, sid)
	assert.ErrorIs(t, err, domain.ErrNotFound, "the batch is gone")
	assert.False(t, e.plateGone(friDinner.ID))
	assert.NotContains(t, boardIssues(t, e), planning.CodeTooManyCookOccasions)
}

// With room for a second occasion that day, the batch still moves.
func TestReplan_OrphanMovesWhenTheDayHasRoom(t *testing.T) {
	e, cook, q := orphanEnv(t)
	require.NoError(t, testsvc.Settings(e.db).Set(e.ctx, "plan.max_cook_occasions_per_day", "2"))
	sid := *cook.Components[0].CookSessionID
	e.userPlate(fri, e.dinner, e.mains[6])

	pv, err := e.svc.PreviewReplan(e.ctx, q)

	require.NoError(t, err)
	assert.Equal(t, []planning.MovedBatch{{SessionID: sid, FoodName: "Main 05", Cook: ck(fri, e.lunch)}}, pv.Moved)
	assert.Empty(t, pv.Dropped)
	_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: q, UnlockConfirmed: true, Spare: planning.SpareKeep})
	require.NoError(t, err)
	assert.True(t, ck(fri, e.lunch).Equal(e.session(sid).CookCell()))
	assert.NotContains(t, boardIssues(t, e), planning.CodeTooManyCookOccasions)
}

// S2: the batch's frozen portions feed the next window. Moving its cook
// there would cook next week what was bought this week, so it goes, and the
// preview names the plate outside the range.
func TestReplan_OrphanFeedingTheNextWindowIsDropped(t *testing.T) {
	e, cook, q := orphanEnv(t)
	require.NoError(t, testsvc.Settings(e.db).Set(e.ctx, "plan.freezing_enabled", "true"))
	sid := *cook.Components[0].CookSessionID
	_, err := e.plates.AddLeftovers(e.ctx, sid, []plate.CellKey{ck("2026-11-17", e.lunch)})
	require.NoError(t, err)
	v := e.session(sid)
	require.Equal(t, plate.KindFrozen, v.Links[len(v.Links)-1].Kind)
	next := v.Links[len(v.Links)-1].PlateID
	friLunch := e.window()[key(fri, e.lunch)]

	pv, err := e.svc.PreviewReplan(e.ctx, q)

	require.NoError(t, err)
	assert.Empty(t, pv.Moved)
	dropped := []int64{}
	for _, d := range pv.Dropped {
		assert.Equal(t, sid, d.SessionID)
		dropped = append(dropped, d.PlateID)
	}
	assert.ElementsMatch(t, []int64{friLunch.ID, next}, dropped)
	affected := []int64{}
	for _, a := range pv.ConfirmedAffected {
		affected = append(affected, a.PlateID)
	}
	assert.ElementsMatch(t, []int64{cook.ID, friLunch.ID, next}, affected)

	_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: q, UnlockConfirmed: true, Spare: planning.SpareFreeze})

	require.NoError(t, err)
	for _, id := range []int64{cook.ID, friLunch.ID, next} {
		assert.True(t, e.plateGone(id), "plate %d", id)
	}
	_, err = e.plates.Session(e.ctx, sid)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// The preview and the re-plan agree: the re-plan removes the preview's
// No-plate plates, removes the dropped batches' components, moves the moved
// batches, and touches no other plate.
func TestReplan_PreviewMatchesTheReplan(t *testing.T) {
	e, cook, q := orphanEnv(t)
	// Main 05 (Thu lunch → Fri lunch) and Main 08 (Wed lunch → Thu dinner)
	// both lose their cook plate. Thursday has no other cook, so Main 08
	// moves there; Friday cooks Main 09 already, so Main 05 goes.
	busy := e.userPlate(fri, e.dinner, e.mains[8])
	moving := e.userPlate(wed, e.lunch, e.mains[7])
	_, err := e.plates.AddLeftovers(e.ctx, *moving.Components[0].CookSessionID, []plate.CellKey{ck(thu, e.dinner)})
	require.NoError(t, err)
	q.NoneCells = append(q.NoneCells, ck(wed, e.lunch))
	before := map[int64]plate.Plate{}
	all, err := e.plates.Range(e.ctx, day(sat), day("2026-11-20"))
	require.NoError(t, err)
	for _, p := range all {
		before[p.ID] = p
	}

	pv, err := e.svc.PreviewReplan(e.ctx, q)
	require.NoError(t, err)
	_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: q, UnlockConfirmed: true, Spare: planning.SpareKeep})
	require.NoError(t, err)

	gone := map[int64]bool{}
	for _, c := range pv.NoneCells {
		for _, p := range before {
			if p.Cell().Equal(c) {
				gone[p.ID] = true
			}
		}
	}
	losing := map[int64]bool{}
	for _, d := range pv.Dropped {
		losing[d.PlateID] = true
	}
	after, err := e.plates.Range(e.ctx, day(sat), day("2026-11-20"))
	require.NoError(t, err)
	still := map[int64]plate.Plate{}
	for _, p := range after {
		still[p.ID] = p
	}
	for id, p := range before {
		now, ok := still[id]
		switch {
		case gone[id]:
			assert.False(t, ok, "No-plate plate %d goes", id)
		case losing[id]:
			assert.True(t, !ok || len(now.Components) < len(p.Components), "plate %d loses a component", id)
		default:
			require.True(t, ok, "plate %d stays", id)
			assert.Len(t, now.Components, len(p.Components), "plate %d keeps its food", id)
		}
	}
	require.Len(t, pv.Moved, 1)
	require.Len(t, pv.Dropped, 1)
	for _, m := range pv.Moved {
		assert.True(t, m.Cook.Equal(e.session(m.SessionID).CookCell()), "batch %d cooks where the preview said", m.SessionID)
	}
	assert.False(t, e.plateGone(busy.ID))
	assert.True(t, e.plateGone(cook.ID))
	assert.NotContains(t, boardIssues(t, e), planning.CodeTooManyCookOccasions)
}
