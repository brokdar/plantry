package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func lightState() planning.State {
	s := baseState()
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	return s
}

// weekWith fills every cell with a Curry plate (500 kcal, 1500 a day) like
// weekOf, under the light requests of the proposal.
func weekWith(s planning.State, light ...planning.LightIntent) planning.Evaluation {
	tgt := windowTarget(s)
	var prop planning.Proposal
	for _, c := range s.Cells {
		prop.Plates = append(prop.Plates, pplate(c.Date.Format("2006-01-02"), c.SlotID, fresh(curry)))
	}
	prop.Light = light
	return planning.Evaluate(planning.Merge(s, tgt, prop), tgt)
}

func TestValidateLight(t *testing.T) {
	got, err := planning.ValidateLight([]planning.LightIntent{
		{Date: mon, Scope: "day", Slot: "lunch"},
		{Date: mon, Scope: "day"},
		{Date: tue, Scope: "meal", Slot: "dinner"},
	})
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{{Date: mon, Scope: "day"}, {Date: tue, Scope: "meal", Slot: "dinner"}}, got)

	for name, bad := range map[string]planning.LightIntent{
		"meal without slot": {Date: mon, Scope: "meal"},
		"breakfast":         {Date: mon, Scope: "meal", Slot: "breakfast"},
		"unknown scope":     {Date: mon, Scope: "week"},
		"bad date":          {Date: "monday", Scope: "day"},
	} {
		_, err := planning.ValidateLight([]planning.LightIntent{bad})
		assert.Error(t, err, name)
	}
}

func TestEvaluate_LightMeal(t *testing.T) {
	s := lightState()
	s.Profile.KcalTarget = f64(1500) // a meal's share 500, the light cap 375
	light := planning.LightIntent{Date: mon, Scope: "meal", Slot: "dinner"}

	ev := weekWith(s, light)
	got := codeDates(ev.Issues, "S6", planning.CodeMealNotLight)
	assert.Equal(t, []string{mon}, got, "a 500 kcal dinner is not 75 % of its share")
	for _, is := range ev.Issues {
		if is.Rule == "S6" {
			assert.InDelta(t, 375, is.Params["max_kcal"], 0.01)
			assert.InDelta(t, 500, is.Params["normal_share_kcal"], 0.01)
			assert.Equal(t, planning.SeveritySoft, is.Severity)
		}
	}
	assert.Equal(t, []planning.LightIntent{light}, ev.Light)
	for _, d := range ev.Days {
		if d.Date == mon {
			assert.Equal(t, "meal", d.Light)
			assert.InDelta(t, 1375, *d.TargetKcal, 0.01, "a light meal moves the day target by 25 % of its share")
			assert.InDelta(t, 1500, d.NormalTargetKcal, 0.01)
		}
	}

	t.Run("a lighter meal clears the issue, the day target follows", func(t *testing.T) {
		// Dinner of 375 kcal exactly: soup at 0.75 portions of 500 is too
		// fiddly, so use a 300 kcal recipe and 600 kcal at lunch.
		s := lightState()
		s.Profile.KcalTarget = f64(1500)
		tgt := windowTarget(s)
		var prop planning.Proposal
		for _, c := range s.Cells {
			food := curry
			if c.Date.Format("2006-01-02") == mon && c.SlotID == dinner {
				food = rice // 200 kcal
			}
			if c.Date.Format("2006-01-02") == mon && c.SlotID == lunch {
				food = chili // 600 kcal
			}
			prop.Plates = append(prop.Plates, pplate(c.Date.Format("2006-01-02"), c.SlotID, fresh(food)))
		}
		prop.Light = []planning.LightIntent{light}
		ev := planning.Evaluate(planning.Merge(s, tgt, prop), tgt)
		assert.Empty(t, codeDates(ev.Issues, "S6", planning.CodeMealNotLight))
		// 1300 of the shifted 1375 is 95 %: inside the band centred there.
		assert.Empty(t, codeDates(ev.Issues, "S2", planning.CodeKcalBelowBand))
	})

	t.Run("without intents nothing changes", func(t *testing.T) {
		ev := weekWith(s)
		assert.Empty(t, codeDates(ev.Issues, "S6", planning.CodeMealNotLight))
		assert.Nil(t, ev.Light)
	})
}

func TestEvaluate_LightDay(t *testing.T) {
	light := planning.LightIntent{Date: mon, Scope: "day"}

	t.Run("target 85 percent, band centred there", func(t *testing.T) {
		s := lightState()
		s.Profile.KcalTarget = f64(1800) // monday: 1530, band 1377-1683; normal band 1620-1980
		ev := weekWith(s, light)
		below := codeDates(ev.Issues, "S2", planning.CodeKcalBelowBand)
		assert.NotContains(t, below, mon, "1500 is inside the light day's band")
		assert.Len(t, below, 6, "the normal days (1500 of 1800) are below their band")
		for _, d := range ev.Days {
			if d.Date == mon {
				assert.InDelta(t, 1530, *d.TargetKcal, 0.01)
				assert.InDelta(t, 1800, d.NormalTargetKcal, 0.01)
				assert.Equal(t, "day", d.Light)
			}
		}
	})

	t.Run("the floors are not lowered", func(t *testing.T) {
		s := lightState()
		s.Profile.KcalTarget = f64(2100) // floor 1575, adjusted target 1785
		ev := weekWith(s, light)
		assert.Contains(t, codeDates(ev.Issues, "H9", planning.CodeKcalBelowFloor), mon)
		s.Profile.KcalTarget = f64(1900) // floor 1425
		ev = weekWith(s, light)
		assert.NotContains(t, codeDates(ev.Issues, "H9", planning.CodeKcalBelowFloor), mon)
	})
}

func TestEvaluate_LightWeekLedger(t *testing.T) {
	s := lightState()
	s.Profile.KcalTarget = f64(1600) // 1500 a day: 93.75 % of normal, below the 95 % week band
	plain := weekWith(s)
	assert.Contains(t, codeDates(plain.Issues, "S4", planning.CodeWeekKcalBelowBand), "")
	require.NotNil(t, plain.Week)
	assert.Zero(t, plain.Week.NormalTargetKcal, "no light request: one target")

	var light []planning.LightIntent
	for _, d := range []string{mon, tue, wed, "2026-11-12"} {
		light = append(light, planning.LightIntent{Date: d, Scope: "day"})
	}
	ev := weekWith(s, light...)
	require.NotNil(t, ev.Week)
	assert.InDelta(t, (3*1600+4*1360)/7.0, ev.Week.TargetKcal, 0.01)
	assert.InDelta(t, 1600, ev.Week.NormalTargetKcal, 0.01)
	assert.InDelta(t, 1500/ev.Week.TargetKcal*100, ev.Week.Pct, 0.01)
	assert.Empty(t, codeDates(ev.Issues, "S4", planning.CodeWeekKcalBelowBand), "honouring the wish does not raise S4")
}
