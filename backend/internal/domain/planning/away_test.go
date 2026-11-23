package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// A day away (no slot planned: every cell No plate or a routine without a
// plate) and a routine-only day (every planned meal a routine, V63 c) are not
// judged by H9, S2 or the day S3 and leave the weekly averages. A day with any
// non-routine meal left is judged on its planned part (SB3, SF4).

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

func findDay(ev planning.Evaluation, date string) (planning.DayEval, bool) {
	for _, d := range ev.Days {
		if d.Date == date {
			return d, true
		}
	}
	return planning.DayEval{}, false
}

func TestEvaluate_DayAwayIsNotJudged(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000)
	s.Profile.ProteinPct = f64(30)
	setMode(&s, cell(mon, breakfast), availability.ModeRoutine) // a routine without a plate
	setMode(&s, cell(mon, lunch), availability.ModeNone)
	setMode(&s, cell(mon, dinner), availability.ModeNone)

	ev, issues := weekOf(s)

	_, ok := findDay(ev, mon)
	assert.False(t, ok, "a day with no slot planned is not accounted at all (SB3)")
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
	require.NotNil(t, d.TargetKcal)
	assert.InDelta(t, 2000, *d.TargetKcal, 0.01, "scaled to the planned meals")
	assert.Contains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
	assert.Contains(t, codeDates(ev.Issues, "S2", planning.CodeKcalBelowBand), mon)
	assert.Equal(t, 7, ev.Week.Days)
}

// spec v1.5 SB3 (V63 c, replacing #172's "a routine breakfast left is judged
// on its part"): a day whose only planned meals are routine (lunch and dinner
// away) is judged like a fully away day: no floor, no H9, not in the week
// (tune2/tune3 g-away-08: routine breakfast left, lunch and dinner away).
func TestEvaluate_RoutineOnlyDayIsNotJudged(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000)
	s.SlotKind = map[int64]slot.Kind{breakfast: slot.KindLight}
	setMode(&s, cell(mon, breakfast), availability.ModeRoutine)
	setMode(&s, cell(mon, lunch), availability.ModeNone)
	setMode(&s, cell(mon, dinner), availability.ModeNone)
	s.Sessions = append(s.Sessions, persistedSession(900, curry, mon, breakfast, 1, true))
	s.Plates = append(s.Plates, persisted(900, mon, breakfast, true, curry, 900))

	ev, issues := weekOf(s)

	_, ok := findDay(ev, mon)
	assert.False(t, ok, "a routine-only day is not accounted (SB3, V63 c)")
	assert.NotContains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
	assert.Equal(t, 6, ev.Week.Days, "the routine-only day is not in the week")
}

// The same day with a normal planned breakfast (not a routine) is judged on
// its planned part as before (#172).
func TestEvaluate_PlannedBreakfastLeftIsStillJudged(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000)
	s.SlotKind = map[int64]slot.Kind{breakfast: slot.KindLight}
	setMode(&s, cell(mon, lunch), availability.ModeNone)
	setMode(&s, cell(mon, dinner), availability.ModeNone) // weekOf plans a 500 kcal curry breakfast

	ev, issues := weekOf(s)

	d := dayEval(t, ev, mon)
	assert.Positive(t, d.FloorKcal)
	assert.Less(t, d.FloorKcal, 1500.0, "the minimum is prorated to the planned part")
	assert.Contains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
	assert.Equal(t, 7, ev.Week.Days)
}

// A light breakfast left beside lunch No plate and a routine dinner without a
// plate is a planned meal too: the day is judged (SB3).
func TestEvaluate_NoneLunchAndUnplatedRoutineDinnerIsJudged(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000)
	s.SlotKind = map[int64]slot.Kind{breakfast: slot.KindLight}
	setMode(&s, cell(mon, lunch), availability.ModeNone)
	setMode(&s, cell(mon, dinner), availability.ModeRoutine)

	ev, _ := weekOf(s)

	d := dayEval(t, ev, mon)
	assert.Positive(t, d.Kcal, "the breakfast counts on the day")
	assert.Equal(t, 7, ev.Week.Days)
}

// d3: a day away is never called light, whatever mark is stored for its date.
func TestEvaluate_AwayDayCarriesNoLightMark(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(3000)
	setMode(&s, cell(mon, breakfast), availability.ModeRoutine) // a routine without a plate
	setMode(&s, cell(mon, lunch), availability.ModeNone)
	setMode(&s, cell(mon, dinner), availability.ModeNone)
	s.Light = []planning.LightIntent{{Date: mon, Scope: planning.LightScopeMeal, Slot: "lunch"}, {Date: mon, Scope: planning.LightScopeDay}}

	ev, _ := weekOf(s)

	_, ok := findDay(ev, mon)
	assert.False(t, ok, "a day away is not listed, so nothing calls it light")
	for _, is := range ev.Issues {
		if is.Date == mon {
			assert.NotContains(t, []string{"H9", "S2", "S3"}, is.Rule, "no %s on a day away", is.Code)
		}
	}
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
