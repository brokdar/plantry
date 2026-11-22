package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// G4.2/G4.3 and the "told" rule: when the block is cut to its five lines, the
// changed meals and the cooking that moved stay; the side figures of a meal go
// first. tune2 g-away-03 lost "Wed 2 Dec: cook 4 portions ..." for the
// protein and moved-target lines, and g-replan-11 lost both regenerated meals
// for week figures and warnings (the stored facts below, plus the cut lines).
func TestSelectBlock_KeepsChangedMealsAndCookingWhenCut(t *testing.T) {
	t.Parallel()
	f := func(class int, kind, status, en string) fact {
		return fact{Class: class, Kind: kind, Status: status, EN: en, DE: en}
	}
	away := []fact{
		f(classChanged, kindMeal, statusOK, "Thu 3 Dec dinner: no meal any more."),
		f(classChanged, kindProtein, statusOK, "Thu 3 Dec: protein 162 g → 78 g (it needs 61 g)."),
		f(classChanged, kindDay, statusOK, "Thu 3 Dec: the amount that suits the day is now 1878 kcal (was 2600)."),
		f(classMoved, kindLeftover, statusOK, `Wed 2 Dec: cook 4 portions of the "Roasted vegetables" (was 6).`),
		f(classWorse, kindWeek, statusUnder, "For the week: average energy is 94% of the target (was 97%)."),
		f(classWorse, kindDay, statusUnder, "Thu 3 Dec holds 1216 kcal (was 2440), under the day's minimum of 1408 kcal."),
	}
	got := selectBlock(away)
	assert.Len(t, got, maxBlock)
	assert.Contains(t, got, away[0])
	assert.Contains(t, got, away[3], "the cooking day is told")
	assert.Contains(t, got, away[5], "the day under its minimum is told")

	replan := []fact{
		f(classChanged, kindMeal, statusOK, "Mon 30 Nov lunch: Pork schnitzel replaced by Erdnusseintopf."),
		f(classChanged, kindMeal, statusOK, "Mon 30 Nov dinner: Pork schnitzel replaced by Turkey chili."),
		f(classMoved, kindCook, statusOK, "Tue 1 Dec at lunch you cook Boiled potatoes for Tue 1 Dec lunch."),
		f(classWorse, kindWeek, statusUnder, "For the week: fish is on the plan not at all (was once)."),
		f(classWorse, kindWarn, statusUnknown, "Worse: Weekly carbs: 37%."),
		f(classWorse, kindWarn, statusUnknown, "Worse: Weekly fat: 46%."),
		f(classWarn, kindWarn, statusOver, "New warning: Tue 1 Dec has 2 separate cooking times; the limit is 1."),
		f(classWarn, kindWarn, statusUnknown, "New warning: Wed 2 Dec dinner: cooked more than once this week."),
	}
	got = selectBlock(replan)
	assert.Len(t, got, maxBlock)
	for _, i := range []int{0, 1, 2} {
		assert.Contains(t, got, replan[i])
	}
	// Told in class order.
	for i := 1; i < len(got); i++ {
		assert.LessOrEqual(t, got[i-1].Class, got[i].Class)
	}
}
