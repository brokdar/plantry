package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func evalDay(t *testing.T, ev planning.Evaluation, date string) planning.DayEval {
	t.Helper()
	for _, d := range ev.Days {
		if d.Date == date {
			return d
		}
	}
	require.Failf(t, "day missing", "no evaluation for %s", date)
	return planning.DayEval{}
}

func TestEvaluate(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(2000)
	s.Profile.MeatDaysPerWeekMax = intp(1)
	setMode(&s, cell(mon, dinner), availability.ModeNone) // a short day
	tgt := windowTarget(s)

	prop := planning.Proposal{
		Sessions: []planning.ProposedSession{
			session("s1", chili, sat, dinner, 6), // cooked Sat for 3 eaters, eaten Sat and Sun
			session("s2", chili, tue, dinner, 1), // cooked Tue, eaten Wed: Tue is a cook day, not a meat day
			session("s3", stew, mon, lunch, 1),   // unknown diet class: a warning
		},
		Plates: []planning.ProposedPlate{
			{Date: day(sat), SlotID: dinner, Eaters: 3, Components: []planning.ProposedComponent{onKey(chili, "s1")}},
			{Date: day(sun), SlotID: lunch, Eaters: 3, Components: []planning.ProposedComponent{onKey(chili, "s1")}},
			{Date: day(mon), SlotID: lunch, Eaters: 1, Components: []planning.ProposedComponent{onKey(stew, "s3")}},
			{Date: day(wed), SlotID: lunch, Eaters: 1, Components: []planning.ProposedComponent{onKey(chili, "s2")}},
		},
	}
	ev := planning.Evaluate(planning.Merge(s, tgt, prop), tgt)

	t.Run("meat eating day versus cook day", func(t *testing.T) {
		assert.True(t, evalDay(t, ev, sat).Meat)
		assert.Equal(t, []string{"Chili"}, evalDay(t, ev, sat).MeatFoods)
		tueEval := evalDay(t, ev, tue)
		assert.False(t, tueEval.Meat, "chili is only cooked on Tuesday")
		assert.Empty(t, tueEval.MeatFoods)
		assert.Equal(t, 1, tueEval.CookOccasions)
		assert.True(t, evalDay(t, ev, wed).Meat)
		assert.Equal(t, 3, ev.MeatDays, "sat, sun and wed eat chili")
		require.NotNil(t, ev.MeatDaysMax)
		assert.Equal(t, 1, *ev.MeatDaysMax)
	})

	t.Run("short day scales the target", func(t *testing.T) {
		d := evalDay(t, ev, mon)
		require.NotNil(t, d.TargetKcal)
		assert.InDelta(t, 2000*2.0/3.0, *d.TargetKcal, 0.01)
		assert.InDelta(t, 500, d.Kcal, 0.01) // the stew
		assert.False(t, d.Complete, "breakfast is empty")
		full := evalDay(t, ev, sat)
		require.NotNil(t, full.TargetKcal)
		assert.InDelta(t, 2000, *full.TargetKcal, 0.01)
	})

	t.Run("macros are per eater", func(t *testing.T) {
		d := evalDay(t, ev, sat)
		assert.InDelta(t, 600, d.Kcal, 0.01, "one portion per eater, not times 3 eaters")
		assert.InDelta(t, 30, d.ProteinG, 0.01)
		assert.InDelta(t, 30*4/600.0*100, d.ProteinPct, 0.01)
	})

	t.Run("plates carry eaters and components", func(t *testing.T) {
		var sunPlate *planning.PlateEval
		for i := range ev.Plates {
			if ev.Plates[i].Date == sun && ev.Plates[i].SlotID == lunch {
				sunPlate = &ev.Plates[i]
			}
		}
		require.NotNil(t, sunPlate)
		assert.Equal(t, 3.0, sunPlate.Eaters)
		assert.InDelta(t, 600, sunPlate.Kcal, 0.01)
		require.Len(t, sunPlate.Components, 1)
		c := sunPlate.Components[0]
		assert.Equal(t, chili, c.FoodID)
		assert.Equal(t, "Chili", c.Name)
		assert.Equal(t, "main", c.Role)
		assert.Equal(t, "s1", c.SessionKey)
		require.NotNil(t, c.Portions)
		assert.Equal(t, 1.0, *c.Portions)
	})

	t.Run("issues include warnings and violations", func(t *testing.T) {
		var warn, viol int
		for _, is := range ev.Issues {
			if is.Severity == planning.SeverityWarning {
				warn++
			} else {
				viol++
			}
		}
		assert.Positive(t, warn, "diet_unknown of the stew is a warning")
		assert.Positive(t, viol, "meat days 3 > 1 and empty cells")
		assert.Equal(t, warn, ev.Totals.Warnings)
		assert.Equal(t, viol, ev.Totals.Violations)
		assert.Contains(t, issueCodes(ev.Issues), planning.CodeDietUnknown)
		assert.Contains(t, issueCodes(ev.Issues), planning.CodeMeatDaysExceeded)
	})
}

func issueCodes(issues []planning.Issue) []string {
	out := make([]string, len(issues))
	for i, is := range issues {
		out[i] = is.Code
	}
	return out
}

// A day of No-plate meals has nothing to evaluate.
func TestEvaluate_SkipsNoneDays(t *testing.T) {
	s := baseState()
	for _, slot := range []int64{breakfast, lunch, dinner} {
		setMode(&s, cell(sun, slot), availability.ModeNone)
	}
	ev := planning.Evaluate(s, windowTarget(s))
	for _, d := range ev.Days {
		assert.NotEqual(t, sun, d.Date)
	}
	assert.NotNil(t, ev.Issues)
	assert.NotNil(t, ev.Plates)
}
