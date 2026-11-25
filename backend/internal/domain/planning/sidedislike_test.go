package planning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// The builder derives the "rather not" sides from plate feedback: a side on
// two plates rated disliked or not_eaten in the last 8 weeks is one; a
// loved plate in that span cancels it; older feedback is ignored.
func TestBuilder_RatherNotSides(t *testing.T) {
	e := newSvcEnv(t)
	one := 1.0
	refused, cancelled, old := e.sides[0], e.sides[1], e.sides[2]
	rated := func(date string, side int64, status string) {
		p, err := e.plates.Create(e.ctx, plate.PlateInput{Date: day(date), SlotID: e.lunch, Components: []plate.PlateComponent{
			{FoodID: e.mains[5], Portions: &one, Position: plate.PositionMain},
			{FoodID: side, Portions: &one, Position: plate.PositionSide},
		}})
		require.NoError(t, err)
		e.exec(`INSERT INTO plate_feedback (plate_id, status) VALUES (?, ?)`, p.ID, status)
	}
	rated("2026-10-20", refused, "disliked")
	rated("2026-11-02", refused, "not_eaten")
	rated("2026-10-21", cancelled, "disliked")
	rated("2026-10-22", cancelled, "disliked")
	rated("2026-10-30", cancelled, "loved")
	rated("2026-09-01", old, "disliked") // more than 8 weeks before today (2026-11-06)
	rated("2026-10-23", old, "disliked")

	var st planning.State
	require.NoError(t, e.deps.Tx.InTx(e.ctx, func(ctx context.Context) error {
		var err error
		w := day(sat)
		st, err = e.deps.Builder.Build(ctx, window.Window{Start: w, End: w.AddDate(0, 0, 6)}, day(svcToday), w, w.AddDate(0, 0, 6))
		return err
	}))
	assert.Equal(t, map[int64]bool{refused: true}, st.RatherNot)
}

// A side the household would rather not get ranks after every acceptable
// side, with plate fit on (before the fresh side that settles the gap) and
// off (before the favourite).
func TestHeuristic_RatherNotSideRanksLast(t *testing.T) {
	on := plateFitCooker(true)
	on.RatherNot = map[int64]bool{sideSalad: true}
	assert.Equal(t, sideRoast, firstSide(t, on))

	off := plateFitCooker(false)
	off.RatherNot = map[int64]bool{sideRice: true}
	assert.Equal(t, sideRoast, firstSide(t, off), "the next favourite")

	only := plateFitCooker(true)
	only.RatherNot = map[int64]bool{sideSalad: true, sideRoast: true}
	assert.Contains(t, []int64{sideSalad, sideRoast}, firstSide(t, only), "with no other side a rather-not side still serves")
}

// Batch variation never varies a leftover plate onto a rather-not side:
// keeping the batch's side is the better alternative.
func TestHeuristic_BatchVariationSkipsRatherNot(t *testing.T) {
	s := varyCooker(true)
	s.RatherNot = map[int64]bool{300: true}
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	for key, sides := range batchSides(s, p) {
		assert.NotContains(t, sides, int64(300), "batch %s", key)
	}
}
