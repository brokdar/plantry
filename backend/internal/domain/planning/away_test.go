package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// A day away (every lunch and dinner No plate) is not judged by H9, S2 or
// the day S3 and leaves the weekly averages; light and routine cells do not
// count.

func dayEval(t *testing.T, ev planning.Evaluation, date string) planning.DayEval {
	t.Helper()
	for _, d := range ev.Days {
		if d.Date == date {
			return d
		}
	}
	require.Failf(t, "no day", "%s not evaluated", date)
	return planning.DayEval{}
}

func TestEvaluate_DayAwayIsNotJudged(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000) // one breakfast of 500 kcal is under any floor
	s.Profile.ProteinPct = f64(30)
	setMode(&s, cell(mon, breakfast), availability.ModeRoutine)
	setMode(&s, cell(mon, lunch), availability.ModeNone)
	setMode(&s, cell(mon, dinner), availability.ModeNone)
	s.Sessions = append(s.Sessions, persistedSession(900, curry, mon, breakfast, 1, true))
	s.Plates = append(s.Plates, persisted(900, mon, breakfast, true, curry, 900))

	ev, issues := weekOf(s)

	d := dayEval(t, ev, mon)
	assert.True(t, d.Away)
	assert.InDelta(t, 500, d.Kcal, 0.01, "the routine breakfast still counts on the day")
	assert.NotContains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
	for _, is := range ev.Issues {
		if is.Date == mon {
			assert.NotContains(t, []string{"H9", "S2", "S3"}, is.Rule, "no %s on a day away", is.Code)
		}
	}
	require.NotNil(t, ev.Week)
	assert.Equal(t, 6, ev.Week.Days, "the day away is not in the week")
	assert.Contains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), tue, "other days are still judged")
}

func TestEvaluate_HalfAwayDayIsStillJudged(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000) // 2 of 3 meals: target 2000, floor 1500, the day holds 1000
	setMode(&s, cell(mon, lunch), availability.ModeNone)

	ev, issues := weekOf(s)

	d := dayEval(t, ev, mon)
	assert.False(t, d.Away)
	require.NotNil(t, d.TargetKcal)
	assert.InDelta(t, 2000, *d.TargetKcal, 0.01, "scaled to the planned meals")
	assert.Contains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
	assert.Contains(t, codeDates(ev.Issues, "S2", planning.CodeKcalBelowBand), mon)
	assert.Equal(t, 7, ev.Week.Days)
}

// Lunch No plate and a routine dinner that holds a plate is a day away too
// (breakfast is a light slot): routine cells do not count, so every judged meal is away. Decision: kept
// (the routine is the user's own, not the planner's to judge).
func TestEvaluate_NoneLunchAndRoutineDinnerIsAway(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000)
	s.SlotKind = map[int64]slot.Kind{breakfast: slot.KindLight}
	setMode(&s, cell(mon, lunch), availability.ModeNone)
	setMode(&s, cell(mon, dinner), availability.ModeRoutine)

	ev, issues := weekOf(s)

	d := dayEval(t, ev, mon)
	assert.True(t, d.Away)
	assert.Positive(t, d.Kcal, "the routine dinner's plate still counts on the day")
	assert.NotContains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
	assert.NotContains(t, codeDates(ev.Issues, "S2", planning.CodeKcalBelowBand), mon)
	assert.Equal(t, 6, ev.Week.Days)
}

// d3: a day away is never called light, whatever mark is stored for its date.
func TestEvaluate_AwayDayCarriesNoLightMark(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000)
	s.SlotKind = map[int64]slot.Kind{breakfast: slot.KindLight}
	setMode(&s, cell(mon, lunch), availability.ModeNone)
	setMode(&s, cell(mon, dinner), availability.ModeNone)
	s.Light = []planning.LightIntent{{Date: mon, Scope: planning.LightScopeMeal, Slot: "lunch"}, {Date: mon, Scope: planning.LightScopeDay}}

	ev, _ := weekOf(s)

	d := dayEval(t, ev, mon)
	require.True(t, d.Away)
	assert.Empty(t, d.Light)
	assert.Empty(t, d.LightLevel)
	assert.Empty(t, d.LightMeals)
}

// SF4: a day with a meal away is judged on the floor prorated to the planned
// meals: max(75 % of the scaled target, the absolute floor x share).
func TestEvaluate_FloorFollowsThePlannedMeals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		none  []int64
		share float64
	}{{"all meals planned", nil, 1}, {"lunch away", []int64{lunch}, 2.0 / 3}, {"lunch and dinner away", []int64{lunch, dinner}, 1.0 / 3}} {
		t.Run(tc.name, func(t *testing.T) {
			s := baseState()
			s.Profile.KcalTarget = f64(3000)
			for _, sl := range tc.none {
				setMode(&s, cell(mon, sl), availability.ModeNone)
			}
			ev, _ := weekOf(s)
			d := dayEval(t, ev, mon)
			want := 0.75 * 3000 * tc.share
			assert.InDelta(t, want, d.FloorKcal, 1, "floor = 75 %% of the scaled target (the absolute floor is below it here)")
		})
	}
}
