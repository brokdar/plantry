package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func meal(slot string, hearty bool) planning.LightIntent {
	l := planning.LightIntent{Date: mon, Scope: "meal", Slot: slot}
	if hearty {
		l.Level = planning.LevelHearty
	}
	return l
}

// A meal intent moves the day's target by that meal's change, capped at the
// policy's day band (maintain 90-110 %). Three meals of 500 kcal.
func TestMealIntents_ShiftTheDayTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		intent []planning.LightIntent
		want   float64
	}{
		{"one light meal: -25 % of 500", []planning.LightIntent{meal("dinner", false)}, 1375},
		{"two light meals are capped at the band's low end", []planning.LightIntent{meal("lunch", false), meal("dinner", false)}, 1350},
		{"one hearty meal: +25 % of 500", []planning.LightIntent{meal("dinner", true)}, 1625},
		{"two hearty meals are capped at the band's high end", []planning.LightIntent{meal("lunch", true), meal("dinner", true)}, 1650},
		{"light and hearty cancel", []planning.LightIntent{meal("lunch", false), meal("dinner", true)}, 1500},
		{"a day intent wins over a meal intent", []planning.LightIntent{{Date: mon, Scope: "day"}, meal("dinner", false)}, 1275},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := lightState()
			s.Profile.KcalTarget = f64(1500)
			d := dayEval(t, weekWith(s, tc.intent...), mon)
			require.NotNil(t, d.TargetKcal)
			assert.InDelta(t, tc.want, *d.TargetKcal, 0.01)
			if tc.want != 1500 {
				assert.InDelta(t, 1500, d.NormalTargetKcal, 0.01)
			}
		})
	}

	t.Run("the cap follows the goal's band", func(t *testing.T) {
		s := lightState()
		s.Profile.KcalTarget = f64(1500)
		s.Profile.Goal = profile.GoalLose // 90-105 %
		d := dayEval(t, weekWith(s, meal("lunch", true), meal("dinner", true)), mon)
		assert.InDelta(t, 1575, *d.TargetKcal, 0.01)
	})
}

// H9 follows the shifted target; the absolute floor never moves.
func TestMealIntents_FloorFollowsShift(t *testing.T) {
	floorOf := func(t *testing.T, s planning.State, in planning.LightIntent) (float64, bool) {
		t.Helper()
		// Rice in every cell: 600 kcal a day, far under any floor.
		tgt := windowTarget(s)
		var prop planning.Proposal
		for _, c := range s.Cells {
			prop.Plates = append(prop.Plates, pplate(c.Date.Format("2006-01-02"), c.SlotID, fresh(rice)))
		}
		prop.Light = []planning.LightIntent{in}
		ev := planning.Evaluate(planning.Merge(s, tgt, prop), tgt)
		for _, is := range ev.Issues {
			if is.Code == planning.CodeKcalBelowFloor && is.Date == mon {
				return is.Params["floor_kcal"].(float64), true
			}
		}
		return 0, false
	}

	s := lightState()
	s.Profile.KcalTarget = f64(2000) // meal 667; normal floor 1500
	got, ok := floorOf(t, s, meal("dinner", false))
	require.True(t, ok)
	assert.InDelta(t, 0.75*(2000-0.25*2000.0/3), got, 1, "75 % of the shifted target 1833")
	got, ok = floorOf(t, s, meal("dinner", true))
	require.True(t, ok)
	assert.InDelta(t, 0.75*(2000+0.25*2000.0/3), got, 1, "75 % of the raised target 2167")

	male := profile.SexMale
	s.Profile.Sex = &male // absolute floor 1500
	s.Profile.KcalTarget = f64(1800)
	got, ok = floorOf(t, s, meal("dinner", false))
	require.True(t, ok)
	assert.InDelta(t, 1500, got, 0.01, "the absolute floor does not move")
}

func TestMealIntents_ProteinTargetUnchanged(t *testing.T) {
	s := lightState()
	s.Profile.KcalTarget = f64(1500)
	w := 70.0
	s.Profile.WeightKg = &w
	normal := dayEval(t, weekWith(s), mon).ProteinTargetG
	require.Positive(t, normal)
	for _, in := range []planning.LightIntent{meal("dinner", false), meal("dinner", true)} {
		assert.InDelta(t, normal, dayEval(t, weekWith(s, in), mon).ProteinTargetG, 0.01)
	}
}

func TestMealIntents_WeekTargetIsTheAverageOfTheShiftedDays(t *testing.T) {
	s := lightState()
	s.Profile.KcalTarget = f64(1500)
	ev := weekWith(s, meal("dinner", false))
	require.NotNil(t, ev.Week)
	assert.InDelta(t, (1375+6*1500)/7.0, ev.Week.TargetKcal, 0.01)
	assert.InDelta(t, 1500, ev.Week.NormalTargetKcal, 0.01)

	ev = weekWith(s)
	assert.Zero(t, ev.Week.NormalTargetKcal)
}
