package planview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// A routine is no cooking occasion (spec §3.2, §6.4 H2): the oats of a
// routine breakfast never give "Quick cook: Omelette" and never count as a
// cook day on the board.
func TestRoutineIsNotCooking(t *testing.T) {
	setup := func() (*fixture, int64) {
		f := newFixture()
		f.overrides[cellID(date("2026-11-03"), breakfast)] = availability.ModeRoutine
		sid := f.session(omeletID, "2026-11-03", breakfast, 2)
		f.composed("2026-11-03", breakfast, omeletID, sid)
		return f, sid
	}

	t.Run("Today lists no routine cook and says nothing to cook", func(t *testing.T) {
		f, _ := setup()
		td, err := f.service().Today(ctx, date("2026-11-03"))
		require.NoError(t, err)
		assert.Equal(t, VerdictNothingToCook, td.Verdict.Kind)
		assert.Empty(t, td.Cook)
	})

	t.Run("the board flags the session and skips it in cook days", func(t *testing.T) {
		f, sid := setup()
		start := date("2026-10-31") // the shopping day is Saturday
		p, err := f.service().Plan(ctx, &start, date("2026-11-03"))
		require.NoError(t, err)
		found := false
		for _, v := range p.Sessions {
			if v.ID == sid {
				found = true
				assert.True(t, v.Routine)
			}
		}
		require.True(t, found)
		assert.Equal(t, 0, p.Stats.CookDays)
	})
}

// A light slot is no lunch or dinner: the same recipe on its plates must not
// raise S8, while the same repeat in a full-meal slot still does.
func TestPlan_DishRepeatsCountOnlyFullMealSlots(t *testing.T) {
	s8 := func(p *Plan) bool {
		for _, is := range p.Soft {
			if is.Code == planning.CodeDishRepeats {
				return true
			}
		}
		return false
	}
	run := func(slotID int64, breakfastActive bool) *Plan {
		f := newFixture()
		for i := range f.slots {
			f.slots[i].Kind = slot.KindMeal
			if f.slots[i].ID == breakfast {
				f.slots[i].Kind = slot.KindLight
				f.slots[i].Active = breakfastActive
			}
		}
		sid := f.session(chiliID, "2026-11-01", slotID, 8)
		for _, d := range []string{"2026-11-01", "2026-11-02", "2026-11-03", "2026-11-04"} {
			f.composed(d, slotID, chiliID, sid)
		}
		start := date("2026-10-31")
		p, err := f.service().Plan(ctx, &start, date("2026-11-04"))
		require.NoError(t, err)
		return p
	}
	assert.False(t, s8(run(breakfast, true)), "light breakfast repeats are routine")
	assert.False(t, s8(run(breakfast, false)), "inactive light slot repeats are routine too")
	assert.True(t, s8(run(lunch, true)), "lunch repeats still raise S8")
}
