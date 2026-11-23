package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// V73 (c) (tune-v2 g2-replan-01): a turn regenerated Sunday, undid it and then
// had a replace refused. The reply said "Nothing changed" and still listed the
// replan's add and remove lines and the undo's reverse lines. An undo of a
// change saved in the same turn takes the turn's block back to before it.
func TestReplyTurn_UndoInTheTurnCancelsTheChange(t *testing.T) {
	t.Parallel()
	line := func(en string) fact {
		return fact{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: en, DE: en}
	}
	replan := &replyBlock{facts: []fact{line(`Sun 22 Nov dinner: removed "Shakshuka"; added "Easy chicken curry".`)}}
	undo := &replyBlock{facts: []fact{line(`Sun 22 Nov dinner: removed "Easy chicken curry"; added "Shakshuka".`)}}
	refused := &replyBlock{nothingSaved: true, facts: []fact{{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: `Left out: replacing "Minestrone".`, DE: "x"}}}

	var turn replyTurn
	turn.addWrite("regenerate", true, replan)
	turn.addWrite("undo_last_change", true, undo)
	turn.addWrite("apply_changes", false, refused)
	got := tellOf(turn.block.facts)
	assert.Equal(t, []string{refused.facts[0].EN}, got, "only the refusal: the replan and its undo changed nothing")
	assert.True(t, turn.block.nothingSaved)

	// An undo of a change from an earlier turn is told as it is.
	var later replyTurn
	later.addWrite("undo_last_change", true, undo)
	assert.Equal(t, []string{undo.facts[0].EN}, tellOf(later.block.facts))
}

// S1 (ledger V73): a week replan saved with the dish-variety repeats the plan
// already had names each in the block, always told.
func TestBlockOf_RegenerateNamesEveryKeptRepeat(t *testing.T) {
	t.Parallel()
	out := []byte(`{"status":"partial","error_key":"error.plan.hard_rule_open","notes":"Heavy days: Friday.\nDish variety: no other dish fits, so these repeat a dish eaten or cooked shortly before (the plan had as many repeats before): \"Shakshuka\" on 2026-12-06, \"Lemon salmon with rice\" on 2026-12-07.\nA hard rule is still open in the saved plan: dish variety (2026-12-06, 2026-12-07)."}`)
	b, check := blockOf(out)
	if !assert.False(t, check) || !assert.NotNil(t, b) {
		return
	}
	got := tellOf(selectBlock(b.facts))
	assert.Contains(t, got, `Still repeated soon after the last time, as before, since no other dish fits: "Shakshuka" on Sun 6 Dec, "Lemon salmon with rice" on Mon 7 Dec.`)
	assert.False(t, b.nothingSaved)
}

// Review fix: an undo that undid nothing (refused, or nothing to undo) leaves
// the turn's block, and so the change that stands, in the reply.
func TestReplyTurn_FailedUndoKeepsTheBlock(t *testing.T) {
	t.Parallel()
	line := func(en string) fact {
		return fact{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: en, DE: en}
	}
	replan := &replyBlock{facts: []fact{line(`Sun 22 Nov dinner: removed "Shakshuka"; added "Easy chicken curry".`)}}
	stop := &replyBlock{nothingSaved: true, facts: []fact{{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: "Left out: undo.", DE: "x"}}}
	var turn replyTurn
	turn.addWrite("regenerate", true, replan)
	turn.addWrite("undo_last_change", false, stop)
	got := tellOf(turn.block.facts)
	assert.Contains(t, got, replan.facts[0].EN, "the change still stands")

	// writeSaved: an undo counts when it undid something, a refused one never.
	assert.False(t, writeSaved("undo_last_change", []byte(`{"undone":0}`), nil, &replyBlock{}))
	assert.False(t, writeSaved("undo_last_change", []byte(`{"undone":1}`), nil, stop))
	assert.True(t, writeSaved("undo_last_change", []byte(`{"undone":1}`), nil, &replyBlock{}))
}

// Review fix: a later write that changes only one cooking time of a day keeps
// the earlier line of the day's other cooking time; a cook moved within the day
// leaves no stale line.
func TestMergeBlock_CookLinesAreDroppedPerCookingTime(t *testing.T) {
	t.Parallel()
	cook := func(en string) fact { return fact{Class: classMoved, Kind: kindCook, Status: statusOK, EN: en, DE: en} }
	lunch := cook(`Thu 26 Nov at lunch you cook "Soup" for Thu 26 Nov lunch.`)
	dinner := cook(`Thu 26 Nov at dinner you cook "Pasta" for Thu 26 Nov dinner.`)

	newDinner := cook(`Thu 26 Nov at dinner you cook "Curry" for Thu 26 Nov dinner.`)
	got := tellOf(mergeBlock(&replyBlock{facts: []fact{lunch, dinner}}, &replyBlock{facts: []fact{newDinner}}).facts)
	assert.Contains(t, got, lunch.EN, "the lunch cooking is still true")
	assert.NotContains(t, got, dinner.EN)
	assert.Contains(t, got, newDinner.EN)

	moved := cook(`Thu 26 Nov at dinner you cook "Soup" for Thu 26 Nov lunch, Thu 26 Nov dinner (cooked Thu 26 Nov lunch before).`)
	got = tellOf(mergeBlock(&replyBlock{facts: []fact{lunch}}, &replyBlock{facts: []fact{moved}}).facts)
	assert.NotContains(t, got, lunch.EN, "the cook moved from lunch to dinner")
	assert.Contains(t, got, moved.EN)

	none := cook("Thu 26 Nov: nothing is cooked.")
	got = tellOf(mergeBlock(&replyBlock{facts: []fact{lunch, dinner}}, &replyBlock{facts: []fact{none}}).facts)
	assert.Equal(t, []string{none.EN}, got)
}

// KR7 (tune-v2c g2-replan-01): a day replan that would leave a meal empty saves
// nothing, and the reply says so with the reason in the user's language.
func TestRegenerateRefused_EmptyDayMealIsTold(t *testing.T) {
	t.Parallel()
	b := regenerateRefused(map[string]any{"status": "error", "error_key": "error.plan.day_meal_empty", "notes": "Nothing was saved: Sunday lunch would stay empty."})
	require.True(t, b.nothingSaved)
	assert.Contains(t, b.facts[0].EN, "Sunday lunch would stay empty")
	assert.Contains(t, b.facts[0].DE, "eine Mahlzeit des Tages bliebe leer")
}
