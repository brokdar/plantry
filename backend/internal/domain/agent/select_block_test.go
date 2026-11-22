package agent

import (
	"fmt"
	"slices"
	"strings"
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

// SF1(c)/G4.4: the day an away left under its minimum is told whatever the
// cap of five lines: it takes one of the places and is never the line cut.
func TestSelectBlock_DayUnderItsMinimumIsNeverCut(t *testing.T) {
	t.Parallel()
	f := func(class int, kind, en string) fact {
		return fact{Class: class, Kind: kind, Status: statusOK, EN: en, DE: en}
	}
	under := fact{
		Class: classWorse, Kind: kindDay, Status: statusUnder, Must: true, Key: "day:Thu 26 Nov",
		EN: "Thu 26 Nov holds 306 kcal (was 2010), below a good amount for the day (535-654 kcal) and under the day's minimum of 446 kcal.", DE: "x",
	}
	all := []fact{
		f(classChanged, kindMeal, "Thu 26 Nov lunch: no meal any more."),
		f(classChanged, kindMeal, "Thu 26 Nov dinner: no meal any more."),
		f(classMoved, kindLeftover, `Wed 25 Nov: cook 3 portions of the "Tofu stir-fry" (was 4).`),
		f(classMoved, kindLeftover, `Mon 23 Nov: cook 2 portions of the "Curry" (was 3).`),
		f(classMoved, kindCook, "Thu 26 Nov: nothing is cooked."),
		f(classMoved, kindAway, "Thu 26 Nov: lunch and dinner out, about 1605 kcal is assumed for it."),
		under,
	}
	got := selectBlock(all)
	assert.Len(t, got, maxBlock)
	assert.Contains(t, got, under)
	assert.True(t, slices.ContainsFunc(got, func(f fact) bool { return f.Class == classChanged && f.Kind == kindMeal }), "a changed meal survives: %+v", got)
}

// G4.2 + SF1(c)/P-1: three days under their minimum each give one line (the
// no-lever sentence is merged into it), the block stays within five lines, a
// changed meal survives and every day's minimum is told.
func TestSelectBlock_ThreeDaysUnderTheirMinimumKeepAChangedMeal(t *testing.T) {
	t.Parallel()
	var all []fact
	all = append(all,
		fact{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Thu 26 Nov lunch: no meal any more.", DE: "x"},
		fact{Class: classMoved, Kind: kindCook, Status: statusOK, EN: "Thu 26 Nov: nothing is cooked.", DE: "x"})
	var extra []fact
	days := []string{"Thu 26 Nov", "Fri 27 Nov", "Sat 28 Nov"}
	for i, d := range days {
		all = append(all, fact{
			Class: classWorse, Kind: kindDay, Status: statusUnder, Must: true, Key: "day:" + d,
			EN: d + " holds 306 kcal (was 2010), under the day's minimum of " + []string{"446", "447", "448"}[i] + " kcal.", DE: "x",
		})
		extra = append(extra, fact{
			Class: classWorse, Kind: kindDay, Status: statusUnder, Must: true, Key: "day:" + d,
			EN: "No checked change of a planned meal brings " + d + " up to its minimum.", DE: "y",
		})
	}
	got := selectBlock(mergeNoLever(all, extra))
	assert.LessOrEqual(t, len(got), maxBlock)
	assert.True(t, slices.ContainsFunc(got, func(f fact) bool { return f.Class == classChanged && f.Kind == kindMeal }), "a changed meal survives: %+v", got)
	for i, d := range days {
		assert.True(t, slices.ContainsFunc(got, func(f fact) bool {
			return strings.Contains(f.EN, d+" holds") && strings.Contains(f.EN, "minimum of "+[]string{"446", "447", "448"}[i]+" kcal") && strings.Contains(f.EN, "brings "+d+" up to its minimum")
		}), "%s told with its minimum and the no-fix line: %+v", d, got)
	}
}

// More must lines than places: the block still has five lines at most and a
// changed meal stays.
func TestSelectBlock_TooManyMustLinesStayWithinTheCapAndKeepAChangedMeal(t *testing.T) {
	t.Parallel()
	all := []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Mon 23 Nov lunch: no meal any more.", DE: "x"}}
	for i := 0; i < 6; i++ {
		d := fmt.Sprintf("Day %d", i)
		all = append(all, fact{Class: classWorse, Kind: kindDay, Status: statusUnder, Must: true, Key: "day:" + d, EN: d + " is under the day's minimum of 400 kcal.", DE: "x"})
	}
	got := selectBlock(all)
	assert.LessOrEqual(t, len(got), maxBlock)
	assert.Equal(t, classChanged, got[0].Class, "the changed meal stays")
}

// SF1(c)/P-1 (PR #174 review): the no-lever line shows only when the reply has
// no offer at all; a day_fix of the result is an offer too.
func TestReplyOffer_DayFixCountsAsAnOffer(t *testing.T) {
	t.Parallel()
	after := map[string]any{"issues": map[string]any{"day_fix": []any{map[string]any{
		"date": "2026-11-26",
		"options": []any{map[string]any{
			"action": "add", "name": "Banana", "slot": "dinner", "day_kcal": 900.0,
			"op": map[string]any{"amount": 100.0},
		}},
	}}}}
	assert.NotNil(t, replyOffer(nil, after), "a day_fix is an offer: no no-lever line")
	assert.Nil(t, replyOffer(nil, map[string]any{"issues": map[string]any{}}))
}
