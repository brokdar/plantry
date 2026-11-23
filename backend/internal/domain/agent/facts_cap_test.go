package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// awayFacts are the facts of test1 g-away-00 (Thu 3 Dec away): the five
// lines its stored report told, and the Fri 4 Dec cooking line the cap of 5 cut
// (cooking_moved: Fri 4 Dec lunch cooks the pasta, salad, lasagne, broccoli),
// rendered as cookFacts renders it.
func awayFacts() []fact {
	return []fact{
		{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Thu 3 Dec lunch: no meal any more.", DE: "Donnerstag, 3. Dez. Mittagessen: kein Essen mehr."},
		{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Thu 3 Dec dinner: no meal any more.", DE: "Donnerstag, 3. Dez. Abendessen: kein Essen mehr."},
		{Class: classMoved, Kind: kindLeftover, Status: statusOK, EN: `Wed 2 Dec: cook 1.5 portions of the "Cheese omelette" (was 2.75).`, DE: "Mittwoch, 2. Dez.: 1,5 Portionen von „Cheese omelette“ kochen (statt 2,75)."},
		{Class: classMoved, Kind: kindLeftover, Status: statusOK, EN: `Wed 2 Dec: cook 1 portion of the "Marinated tofu" (was 2).`, DE: "Mittwoch, 2. Dez.: 1 Portion von „Marinated tofu“ kochen (statt 2)."},
		{
			Class: classMoved, Kind: kindCook, Status: statusOK,
			EN: `Fri 4 Dec at lunch you cook "Pasta with pesto and peas" for Fri 4 Dec lunch; "Green salad" for Fri 4 Dec lunch; "Spinach lasagne" for Fri 4 Dec dinner; "Steamed broccoli" for Fri 4 Dec dinner.`,
			DE: "Freitag, 4. Dez. (Mittagessen) kochst du „Pasta with pesto and peas“ für Freitag, 4. Dez. Mittagessen.",
		},
		{
			Class: classWorse, Kind: kindDay, Status: statusUnder, Key: "day:Thu 3 Dec", Must: true,
			EN: "Thu 3 Dec holds 306 kcal (was 2196), below a good amount for the day (535-654 kcal) and under the day's minimum of 446 kcal. No checked change of a planned meal brings Thu 3 Dec up to its minimum.",
			DE: "Donnerstag, 3. Dez.: 306 kcal (vorher 2196), unter dem Mindestwert von 446 kcal.",
		},
	}
}

// spec v1.5 G4 (V63 b): what moved and a day under its minimum are told
// outside the cap of 5; the stored g-away-00 reply lost the Fri cooking move.
func TestSelectBlock_WhatMovedAndTheMinimumAreNeverCut(t *testing.T) {
	t.Parallel()
	got := selectBlock(awayFacts())
	assert.Len(t, got, 6)
	assert.Contains(t, tellOf(got), awayFacts()[4].EN, "the next day's cooking move is told")
	assert.Contains(t, tellOf(got), awayFacts()[5].EN, "the day under its minimum is told")
}

// The cap of 5 applies to the rest: further changed meals, worse figures and new
// warnings; at least one changed meal stays, whatever moved.
func TestSelectBlock_TheCapHoldsForTheRest(t *testing.T) {
	t.Parallel()
	in := awayFacts()
	for _, d := range []string{"Fri 4 Dec", "Sat 5 Dec", "Sun 6 Dec"} {
		in = append(in, fact{Class: classWarn, Kind: kindWarn, Status: statusOK, EN: d + ": a new warning.", DE: d + ": eine neue Warnung."},
			fact{Class: classWorse, Kind: kindWeek, Status: statusOK, EN: d + ": the week got worse.", DE: d + ": die Woche wurde schlechter."})
	}
	got := selectBlock(in)
	var moved, capped, meals int
	for _, f := range got {
		switch {
		case f.Must || (f.Class == classMoved && movedTold[f.Kind]):
			moved++
		default:
			capped++
			if f.Class == classChanged {
				meals++
			}
		}
	}
	assert.Equal(t, 4, moved, "3 moved lines and the minimum line, all told")
	assert.Equal(t, maxBlock, capped, "the rest is capped")
	assert.GreaterOrEqual(t, meals, 1, "a changed meal keeps its place")

	// Many moved lines never push the changed meal out.
	many := []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Thu 3 Dec lunch: no meal any more.", DE: "x"}}
	for _, d := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat"} {
		many = append(many, fact{Class: classMoved, Kind: kindCook, Status: statusOK, EN: d + " at lunch you cook X.", DE: d})
	}
	assert.Len(t, selectBlock(many), 7)
}
