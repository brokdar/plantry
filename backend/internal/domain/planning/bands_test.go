package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// withBands gives a state the slot bands of the default breakfast, lunch and
// dinner: expected shares 27.0 / 37.8 / 35.1 %.
func withBands(s planning.State) planning.State {
	bands := []slot.Band{{SlotID: breakfast, MinPct: 20, MaxPct: 30}, {SlotID: lunch, MinPct: 30, MaxPct: 40}, {SlotID: dinner, MinPct: 25, MaxPct: 40}}
	s.SlotBand = map[int64]slot.Band{}
	for _, b := range bands {
		s.SlotBand[b.SlotID] = b
	}
	s.SlotShare = slot.Shares(bands)
	return s
}

// SB3/SF4: a slot not planned is assumed to hit its expected share; the day's
// planned target and floor prorate by it, and the day shows the numbers.
func TestBands_AwayProratesFloorAndTargetByTheSlotShare(t *testing.T) {
	t.Parallel()
	s := withBands(baseState())
	s.Profile.KcalTarget = f64(2000)
	setMode(&s, cell(mon, dinner), availability.ModeNone)

	ev, _ := weekOf(s)

	d := dayEval(t, ev, mon)
	assert.False(t, d.Away)
	assert.InDelta(t, 2000*0.351, d.AwayKcal, 1, "the dinner out is assumed at 35.1 % of 2000")
	assert.InDelta(t, 2000-2000*0.351, d.PlannedTargetKcal, 1)
	require.NotNil(t, d.TargetKcal)
	assert.InDelta(t, 2000*(1-0.351), *d.TargetKcal, 1)
	assert.InDelta(t, max(0.75*2000, 1200)*(1-0.351), d.FloorKcal, 1, "the floor prorates by the same share, the 75 % term and the absolute term alike")
	full := dayEval(t, ev, tue)
	assert.Zero(t, full.AwayKcal, "a day with every slot planned shows none")
}

// RL4/SB2: a light meal's cap and a filling meal's minimum are 0.75 and 1.25 of
// the slot's expected share of the day's full target.
func TestBands_LightCapAndFillingMinimumFollowTheSlotShare(t *testing.T) {
	t.Parallel()
	s := withBands(baseState())
	s.Profile.KcalTarget = f64(2000)
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	s.Light = []planning.LightIntent{
		{Date: mon, Scope: planning.LightScopeMeal, Slot: "dinner"},
		{Date: tue, Scope: planning.LightScopeMeal, Slot: "lunch", Level: planning.LevelHearty},
	}

	ev, _ := weekOf(s)

	var dinnerCap, lunchMin float64
	for _, p := range ev.Plates {
		if p.Date == mon && p.SlotID == dinner {
			dinnerCap = p.MaxKcal
		}
		if p.Date == tue && p.SlotID == lunch {
			lunchMin = p.MinKcal
		}
	}
	assert.InDelta(t, 0.75*2000*0.351, dinnerCap, 1)
	assert.InDelta(t, 1.25*2000*0.378, lunchMin, 1)
}

// NU11 (S10): a lunch or dinner outside its band, as percent of the day's full
// target, is a soft issue without options; a marked meal is exempt.
func TestBands_MealOutsideItsBandIsASoftIssue(t *testing.T) {
	t.Parallel()
	s := withBands(softState(2200)) // chili plates: breakfast, lunch, dinner of Saturday
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	ev := planning.Evaluate(s, windowTarget(s))
	for _, is := range softOf(ev, "S10") {
		assert.Equal(t, planning.CodeMealOutsideSlotBand, is.Code)
		assert.Equal(t, planning.SeveritySoft, is.Severity)
		assert.Empty(t, is.Options)
	}
	// Narrow the dinner band so its plate must miss it, and mark it: exempt.
	s.SlotBand[dinner] = slot.Band{SlotID: dinner, MinPct: 1, MaxPct: 2}
	ev = planning.Evaluate(s, windowTarget(s))
	require.NotEmpty(t, softOf(ev, "S10"), "a dinner far above a 1-2 % band")
	onDinner := false
	for _, is := range softOf(ev, "S10") {
		onDinner = onDinner || *is.SlotID == int64(dinner)
	}
	assert.True(t, onDinner)
	s.Light = []planning.LightIntent{{Date: sat, Scope: planning.LightScopeMeal, Slot: "dinner"}}
	ev = planning.Evaluate(s, windowTarget(s))
	for _, is := range softOf(ev, "S10") {
		assert.NotEqual(t, int64(dinner), *is.SlotID, "a marked meal is exempt")
	}
}
