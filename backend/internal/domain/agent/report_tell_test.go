package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"

	"github.com/stretchr/testify/assert"
)

func TestMealChange_SaysWhatHappenedInPlainWords(t *testing.T) {
	t.Parallel()
	from, to := 1583, 1212
	for _, c := range []struct {
		name string
		m    reportMeal
		want string
	}{
		{
			"replaced",
			reportMeal{Day: "Tue 24 Nov", Meal: "dinner", Before: []string{"Salmon 1 portion", "Potatoes 1 portion"}, After: []string{"Salmon 1 portion", "Broccoli 1 portion"}, KcalFrom: &from, KcalTo: &to},
			`Tue 24 Nov dinner: "Potatoes" replaced by "Broccoli" (1583 → 1212 kcal).`,
		},
		{
			"resized and added",
			reportMeal{Day: "Tue 24 Nov", Meal: "lunch", Before: []string{"Curry 1 portion"}, After: []string{"Curry 1.5 portions", "Bread 80 g"}},
			`Tue 24 Nov lunch: added "Bread"; "Curry" from 1 portion to 1.5 portions (half a portion more).`,
		},
		{
			"moved",
			reportMeal{Day: "Mon 23 Nov", Meal: "dinner", MovedFrom: "Fri 27 Nov dinner", Before: []string{"Chili 1 portion"}, After: []string{"Chili 1 portion"}},
			`Mon 23 Nov dinner: "Chili" moved here from Fri 27 Nov dinner.`,
		},
		{"away", reportMeal{Day: "Sun 22 Nov", Meal: "lunch", Before: []string{"Pasta 1 portion"}}, "Sun 22 Nov lunch: no meal any more."},
	} {
		assert.Equal(t, c.want, mealChange(c.m), c.name)
	}
}

// The structured report keeps changed, days and cooking; the facts block and its
// English tell are what the reply says.
func TestChangeReport_JSONHoldsFactsAndTell(t *testing.T) {
	t.Parallel()
	r := changeReport{Facts: []fact{{Class: classMoved, Kind: kindCook, Status: statusOK, EN: "x", DE: "y"}}, Tell: []string{"x"}, Offer: &offerLine{EN: "o", DE: "p"}}
	raw, _ := json.Marshal(r)
	assert.JSONEq(t, `{"facts":[{"class":3,"kind":"cook","status":"ok","en":"x","de":"y"}],"tell":["x"],"offer":{"en":"o","de":"p"},"earlier_warnings_unaffected":0}`, string(raw))
}

// E036: a day whose protein moved by 5 g or more says it in grams.
func TestProteinFacts_BeforeAndAfterInGrams(t *testing.T) {
	t.Parallel()
	in := reportInput{
		before: planning.Evaluation{Days: []planning.DayEval{{Date: "2026-11-22", ProteinG: 91}}},
		after:  planning.Evaluation{Days: []planning.DayEval{{Date: "2026-11-22", ProteinG: 104, ProteinTargetG: 75}}},
		facts:  &factSink{},
	}
	r := changeReport{Days: []reportDay{{Day: "Sun 22 Nov", KcalBefore: 2000, KcalAfter: 2100}}}
	proteinFacts(in, r)
	assert.Equal(t, []string{"Sun 22 Nov already held 91 g of protein, above the 75 g it needs; now 104 g."}, enLines(in.facts))
	assert.Equal(t, []string{"Sonntag, 22. Nov. hatte schon 91 g Eiweiß, über den nötigen 75 g; jetzt 104 g."}, deLines(in.facts))
	in.after.Days[0].ProteinG = 93
	in.facts = &factSink{}
	proteinFacts(in, r)
	assert.Empty(t, in.facts.items)
}

// E036c: with a day away, a week figure that only moved because the week is
// counted over fewer days is no fact, unless the meat is over its limit; then it
// is one line.
func TestWarnings_AwayDoesNotCallUnchangedFoodWorse(t *testing.T) {
	t.Parallel()
	meat := func(g, max float64, days int) planning.Issue {
		return planning.Issue{Rule: "S7", Code: planning.CodeWeekMeatAboveDGE, Severity: planning.SeveritySoft, Params: map[string]any{"meat_g": g, "max_g": max, "days": float64(days)}}
	}
	in := reportInput{
		before: planning.Evaluation{Issues: []planning.Issue{meat(321, 300, 7)}},
		after:  planning.Evaluation{Issues: []planning.Issue{meat(321, 257, 6)}, Week: &planning.WeekEval{Days: 6}},
		facts:  &factSink{},
	}
	warnings(in, planning.State{}, true, nil)
	if assert.Len(t, in.facts.items, 1, "the remaining days eat what they ate") {
		assert.Equal(t, "With a day away the week is counted over 6 days; the meat limit for those days is 257 g and the plan holds 321 g, as before; that is 64 g over.", in.facts.items[0].EN)
		assert.Equal(t, classWorse, in.facts.items[0].Class)
		assert.Equal(t, statusOver, in.facts.items[0].Status)
	}
	in.facts = &factSink{}
	warnings(in, planning.State{}, false, nil)
	if assert.Len(t, in.facts.items, 1, "without an away it is told") {
		assert.Contains(t, in.facts.items[0].EN, "Worse:")
		assert.Contains(t, in.facts.items[0].DE, "Schlimmer:")
	}
}

// E041: a week figure is told only when this change made it worse or took it
// across a line.
func TestWeekFacts_OnlyWhatGotWorse(t *testing.T) {
	t.Parallel()
	in := reportInput{
		before: planning.Evaluation{Week: &planning.WeekEval{Days: 7, MeatG: 321, Pct: 100, AvgProteinG: 93, TargetProteinG: 71, LegumeMeals: 9}},
		after:  planning.Evaluation{Week: &planning.WeekEval{Days: 7, MeatG: 325, Pct: 101, AvgProteinG: 92, TargetProteinG: 71, LegumeMeals: 8}},
		facts:  &factSink{},
	}
	weekFacts(in, planning.State{}, planning.State{})
	assert.Empty(t, in.facts.items, "a little is no news")
	in.after.Week.LegumeMeals = 0
	weekFacts(in, planning.State{}, planning.State{})
	assert.Equal(t, []string{"For the week: beans or lentils are on the plan not at all (was 9 times)."}, enLines(in.facts))
	assert.Equal(t, []string{"Für die Woche: Bohnen oder Linsen stehen gar nicht auf dem Plan (vorher 9 Mal)."}, deLines(in.facts))
	assert.Equal(t, statusUnder, in.facts.items[0].Status)
}

// A cooking time that changed is one fact: its dishes, the day and meal, the
// meals they feed, where a dish was cooked before and the longest recipe's total
// minutes; a guest at a small meal is told with the portions cooked for them.
func TestBuildFacts_CookingTimeAndGuest(t *testing.T) {
	t.Parallel()
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
			SlotOrder: map[int64]int{2: 1, 3: 2},
			Window:    window.Window{Start: d(21), End: d(27)},
		}
	}
	in := reportInput{
		beforeSt: st(27, 1), afterSt: st(26, 2),
		after: planning.Evaluation{
			Plates: []planning.PlateEval{{PlateID: 8, Kcal: 331}},
			Days:   []planning.DayEval{{Date: "2026-11-23", LightMeals: []string{"light lunch"}}},
		},
		facts: &factSink{},
	}
	buildFacts(in, changeReport{})
	en := enLines(in.facts)
	assert.Contains(t, en, `Thu 26 Nov at dinner you cook "Chili con carne" for Fri 27 Nov dinner (cooked Fri 27 Nov dinner before); the longest recipe takes about 1 h 20 min.`)
	assert.Contains(t, en, `Mon 23 Nov lunch: now for 2 people, 2 portions of "Hasselback potatoes"; it is the small lunch (331 kcal each).`)
	de := deLines(in.facts)
	assert.Contains(t, de, `Donnerstag, 26. Nov. (Abendessen) kochst du „Chili con carne“ für Freitag, 27. Nov. Abendessen (vorher Freitag, 27. Nov. Abendessen gekocht); das längste Rezept braucht etwa 1 Std. 20 Min.`)
	in.after.Days[0].LightMeals = nil
	in.facts = &factSink{}
	buildFacts(in, changeReport{})
	assert.NotContains(t, strings.Join(enLines(in.facts), "\n"), "small")
}
