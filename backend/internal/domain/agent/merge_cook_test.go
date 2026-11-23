package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// V72 T1d (tune-v2 g2-planner-07): a turn regenerated the week (Thursday ate
// Wednesday's gratin: "Thu 26 Nov: nothing is cooked.") and then swapped
// Wednesday's dinner, which moved the gratin's cooking to Thursday lunch. The
// merged block told both lines; the later write's cooking of a day replaces the
// earlier write's cooking lines of that day.
func TestMergeBlock_LaterCookingOfADayReplacesTheEarlier(t *testing.T) {
	t.Parallel()
	cook := func(en string) fact { return fact{Class: classMoved, Kind: kindCook, Status: statusOK, EN: en, DE: en} }
	first := &replyBlock{facts: []fact{
		{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Sat 21 Nov dinner: A replaced by B.", DE: "x"},
		cook(`Wed 25 Nov at dinner you cook "Kartoffelgratin" for Wed 25 Nov dinner, Thu 26 Nov lunch, Thu 26 Nov dinner.`),
		cook("Thu 26 Nov: nothing is cooked."),
		cook(`Fri 27 Nov at lunch you cook "Egg fried rice" for Fri 27 Nov lunch.`),
	}}
	second := &replyBlock{facts: []fact{
		{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Wed 25 Nov dinner: Kartoffelgratin replaced by Greek salad.", DE: "x"},
		cook(`Wed 25 Nov at dinner you cook "Greek salad" for Wed 25 Nov dinner.`),
		cook(`Thu 26 Nov at lunch you cook "Kartoffelgratin" for Thu 26 Nov lunch, Thu 26 Nov dinner.`),
	}}
	got := tellOf(mergeBlock(first, second).facts)
	assert.NotContains(t, got, "Thu 26 Nov: nothing is cooked.", "Thursday cooks after the second write")
	assert.NotContains(t, got, first.facts[1].EN, "Wednesday's cooking is the second write's")
	assert.Contains(t, got, second.facts[2].EN)
	assert.Contains(t, got, first.facts[3].EN, "a day the second write did not touch keeps its line")
	assert.Contains(t, got, first.facts[0].EN)
}
