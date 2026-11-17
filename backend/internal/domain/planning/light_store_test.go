package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func TestMergeLight(t *testing.T) {
	stored := []planning.LightIntent{{Date: mon, Scope: "day"}, {Date: tue, Scope: "meal", Slot: "dinner"}}
	got := planning.MergeLight(stored, []planning.LightIntent{
		{Date: mon, Scope: "meal", Slot: "lunch"}, {Date: tue, Scope: "none"},
	})
	assert.Equal(t, []planning.LightIntent{{Date: mon, Scope: "day"}, {Date: mon, Scope: "meal", Slot: "lunch"}}, got,
		"a meal intent leaves the day intent, none without a slot clears its date")
	assert.Equal(t, stored, planning.MergeLight(stored, nil))
	assert.Equal(t, []string{mon, tue}, planning.LightDates([]planning.LightIntent{{Date: mon, Scope: "day"}, {Date: tue, Scope: "none"}, {Date: mon, Scope: "meal", Slot: "lunch"}}))
}

func TestResolveLight(t *testing.T) {
	s := lightState()
	s.Light = []planning.LightIntent{{Date: mon, Scope: "day"}, {Date: tue, Scope: "day"}}
	days := func(dates ...string) (out []planning.LightIntent) {
		for _, d := range dates {
			out = append(out, planning.LightIntent{Date: d, Scope: "day"})
		}
		return out
	}

	assert.NoError(t, planning.ResolveLight(s, days(wed)), "three light days are allowed")
	err := planning.ResolveLight(s, days(wed, "2026-11-12"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lose")
	assert.NoError(t, planning.ResolveLight(s, days(mon, tue)), "a date replaces its own intent: still two days")
	assert.NoError(t, planning.ResolveLight(s, []planning.LightIntent{{Date: wed, Scope: "meal", Slot: "dinner"}, {Date: "2026-11-12", Scope: "meal", Slot: "dinner"}}), "meal intents are not capped")

	assert.Error(t, planning.ResolveLight(s, days("2026-12-24")), "outside the window")
	assert.Error(t, planning.ResolveLight(s, days("2026-11-06")), "before the window")

	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.snack_time", dinner: "slot.dinner"}
	err = planning.ResolveLight(s, []planning.LightIntent{{Date: wed, Scope: "meal", Slot: "lunch"}})
	require.Error(t, err, "a custom slot matches nothing, so it is refused")
	assert.Contains(t, err.Error(), "lunch")
}

// S6 asks a run to make the meal lighter; a confirmed plate or a routine
// cell cannot be changed by any run, so it never raises it.
func TestLightIssues_SkipUnfixablePlates(t *testing.T) {
	light := planning.LightIntent{Date: mon, Scope: "meal", Slot: "dinner"}
	build := func(confirmed, routine bool) planning.Evaluation {
		s := lightState()
		s.Profile.KcalTarget = f64(1500)
		s.Light = []planning.LightIntent{light}
		for i := range s.Cells {
			if s.Cells[i].Date.Format("2006-01-02") == mon && s.Cells[i].SlotID == dinner && routine {
				s.Cells[i].Mode = availability.ModeRoutine
			}
		}
		s.Foods[curry] = planning.FoodFacts{ID: curry, Name: "Curry", Kind: "composed", Role: "main", DietClass: "vegan", NutritionComplete: true, Macros: s.Foods[curry].Macros, ReferencePortions: 1}
		s.Plates = []planning.Plate{persisted(1, mon, dinner, confirmed, curry, 0)}
		return planning.Evaluate(s, windowTarget(s))
	}
	assert.Equal(t, []string{mon}, codeDates(build(false, false).Issues, "S6", planning.CodeMealNotLight), "a draft plate is flagged")
	assert.Empty(t, codeDates(build(true, false).Issues, "S6", planning.CodeMealNotLight), "confirmed")
	assert.Empty(t, codeDates(build(false, true).Issues, "S6", planning.CodeMealNotLight), "routine")
}

// In lose mode the floor of a small target (1500 for a man) lies above the
// light day's 85 %: the day is not asked for less than the floor, so the
// week target is not one the floor forbids.
func TestDayKcals_LightDayHeldAtTheFloor(t *testing.T) {
	s := lightState()
	m := profile.SexMale
	s.Profile.KcalTarget, s.Profile.Goal, s.Profile.Sex = f64(1600), profile.GoalLose, &m
	s.Light = []planning.LightIntent{{Date: mon, Scope: "day"}}
	for _, d := range planning.DayKcals(s, windowTarget(s)) {
		want := 1600.0
		if d.Date.Format("2006-01-02") == mon {
			want = 1500 // 85 % would be 1360, the floor is 1500
		}
		assert.InDelta(t, want, d.Adjusted, 0.01, d.Date)
	}

	s.Profile.KcalTarget = f64(2000) // 85 % is 1700, above the floor
	for _, d := range planning.DayKcals(s, windowTarget(s)) {
		if d.Date.Format("2006-01-02") == mon {
			assert.InDelta(t, 1700, d.Adjusted, 0.01)
		}
	}
}
