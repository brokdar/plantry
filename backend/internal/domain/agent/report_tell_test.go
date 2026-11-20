package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"

	"github.com/stretchr/testify/assert"
)

func TestMealChange_SaysWhatHappenedInPlainWords(t *testing.T) {
	from, to := 1583, 1212
	for _, c := range []struct {
		name string
		m    reportMeal
		want string
	}{
		{
			"replaced",
			reportMeal{Day: "Tue 24 Nov", Meal: "dinner", Before: []string{"Salmon 1 portion", "Potatoes 1 portion"}, After: []string{"Salmon 1 portion", "Broccoli 1 portion"}, KcalFrom: &from, KcalTo: &to},
			"Tue 24 Nov dinner: Potatoes replaced by Broccoli (1583 → 1212 kcal).",
		},
		{
			"resized and added",
			reportMeal{Day: "Tue 24 Nov", Meal: "lunch", Before: []string{"Curry 1 portion"}, After: []string{"Curry 1.5 portions", "Bread 80 g"}},
			"Tue 24 Nov lunch: added Bread; Curry from 1 portion to 1.5 portions (half a portion more).",
		},
		{
			"moved",
			reportMeal{Day: "Mon 23 Nov", Meal: "dinner", MovedFrom: "Fri 27 Nov dinner", Before: []string{"Chili 1 portion"}, After: []string{"Chili 1 portion"}},
			"Mon 23 Nov dinner: Chili moved here from Fri 27 Nov dinner.",
		},
		{"away", reportMeal{Day: "Sun 22 Nov", Meal: "lunch", Before: []string{"Pasta 1 portion"}}, "Sun 22 Nov lunch: no meal any more."},
	} {
		assert.Equal(t, c.want, mealChange(c.m), c.name)
	}
}

// The structured report keeps changed, days and cooking; the sentences are in
// tell only.
func TestChangeReport_JSONHoldsTellNotTheSentenceLists(t *testing.T) {
	r := changeReport{Leftovers: []string{"a"}, Week: []string{"b"}, NewWarnings: []string{"c"}, Worse: []string{"d"}, AlsoChanged: []string{"e"}, Tell: []string{"x"}}
	raw, _ := json.Marshal(r)
	assert.JSONEq(t, `{"tell":["x"],"earlier_warnings_unaffected":0}`, string(raw))
}

// E036: a day whose protein moved by 5 g or more says it in grams.
func TestBuildTell_ProteinBeforeAndAfterInGrams(t *testing.T) {
	in := reportInput{
		before: planning.Evaluation{Days: []planning.DayEval{{Date: "2026-11-22", ProteinG: 91}}},
		after:  planning.Evaluation{Days: []planning.DayEval{{Date: "2026-11-22", ProteinG: 104, ProteinTargetG: 75}}},
	}
	r := changeReport{Days: []reportDay{{Day: "Sun 22 Nov", KcalBefore: 2000, KcalAfter: 2100}}}
	assert.Contains(t, buildTell(in, r), "Sun 22 Nov already held 91 g of protein, above the 75 g it needs; now 104 g.")
	in.after.Days[0].ProteinG = 93
	assert.NotContains(t, strings.Join(buildTell(in, r), "\n"), "protein")
}

// E036c: with a day away, a week figure that only moved because the week is
// counted over fewer days is one neutral line, never "worse".
func TestWarnings_AwayDoesNotCallUnchangedFoodWorse(t *testing.T) {
	meat := func(g, max float64, days int) planning.Issue {
		return planning.Issue{Rule: "S7", Code: planning.CodeWeekMeatAboveDGE, Severity: planning.SeveritySoft, Params: map[string]any{"meat_g": g, "max_g": max, "days": float64(days)}}
	}
	in := reportInput{
		before: planning.Evaluation{Issues: []planning.Issue{meat(321, 300, 7)}},
		after:  planning.Evaluation{Issues: []planning.Issue{meat(321, 257, 6)}, Week: &planning.WeekEval{Days: 6}},
	}
	out, worse, _, neutral := warnings(in, planning.State{}, true)
	assert.Empty(t, out)
	assert.Empty(t, worse, "the remaining days eat what they ate")
	assert.Equal(t, "With a day away the week is counted over 6 days; the meat limit for those days is 257 g and the plan holds 321 g, as before; that is 64 g over.", neutral)
	_, worse, _, _ = warnings(in, planning.State{}, false)
	assert.Len(t, worse, 1, "without an away it is told")
}

// E041: week figures are told only past a real step or across a line; the
// whole list is one "for the week" item.
func TestWeekFacts_OnlyWhatMovedPastAStep(t *testing.T) {
	in := reportInput{
		before: planning.Evaluation{Week: &planning.WeekEval{Days: 7, MeatG: 321, Pct: 100, AvgProteinG: 93, TargetProteinG: 71, LegumeMeals: 9}},
		after:  planning.Evaluation{Week: &planning.WeekEval{Days: 7, MeatG: 325, Pct: 101, AvgProteinG: 92, TargetProteinG: 71, LegumeMeals: 8}},
	}
	assert.Empty(t, weekFacts(in, planning.State{}, planning.State{}), "a little is no news")
	in.after.Week.LegumeMeals = 0
	got := weekFacts(in, planning.State{}, planning.State{})
	assert.Equal(t, []string{"beans or lentils are on the plan not at all (was 9 times)"}, got)
}

// E051: the batch resizes of a compound change reach the tell, and a cooking
// that shrinks on another day and a guest at a small meal are told.
func TestBuildTell_E051(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 11, day, 0, 0, 0, 0, time.UTC) }
	f1, f2, f3 := float64(1), float64(1), float64(1)
	st := func(chiliCook int, eaters float64) planning.State {
		return planning.State{
			SlotNames: map[int64]string{2: "slot.lunch", 3: "slot.dinner"},
			Foods:     map[int64]planning.FoodFacts{1: {Name: "Chili con carne", Minutes: 80}, 2: {Name: "Hasselback potatoes", Minutes: 75}},
			Sessions: []planning.Session{
				{CookSession: plate.CookSession{ID: 1, FoodID: 1, CookDate: d(chiliCook), CookSlotID: 3}},
				{CookSession: plate.CookSession{ID: 2, FoodID: 2, CookDate: d(27), CookSlotID: 3}},
			},
			Plates: []planning.Plate{
				{ID: 7, Date: d(27), SlotID: 3, Eaters: 1, Components: []planning.Component{{FoodID: 1, Portions: &f1, SessionID: 1}, {FoodID: 2, Portions: &f2, SessionID: 2}}},
				{ID: 8, Date: d(23), SlotID: 2, Eaters: eaters, Components: []planning.Component{{FoodID: 2, Portions: &f3, SessionID: 2}}},
			},
		}
	}
	in := reportInput{
		beforeSt: st(27, 1), afterSt: st(26, 2),
		after: planning.Evaluation{
			Plates: []planning.PlateEval{{PlateID: 8, Kcal: 331}},
			Days:   []planning.DayEval{{Date: "2026-11-23", LightMeals: []string{"light lunch"}}},
		},
	}
	got := buildTell(in, changeReport{Leftovers: []string{`Sun 22 Nov: cook 3.5 portions of the "Curry" (was 3).`}})
	assert.Contains(t, got, `Fri 27 Nov at dinner you now cook only "Hasselback potatoes": about 1 h 15 min in all (was 2 h 35 min); the "Chili con carne" is now eaten as leftovers.`)
	assert.Contains(t, got, "Mon 23 Nov lunch: now for 2 people; it is the small lunch (331 kcal each).")
	assert.Contains(t, got, `Sun 22 Nov: cook 3.5 portions of the "Curry" (was 3).`)
	for _, line := range got {
		if strings.Contains(line, "only") || strings.Contains(line, "small") || strings.Contains(line, "cook 3.5") {
			assert.NotEmpty(t, mustItems(line), line)
		}
	}
	in.after.Days[0].LightMeals = nil
	assert.NotContains(t, strings.Join(buildTell(in, changeReport{}), "\n"), "small")
}
