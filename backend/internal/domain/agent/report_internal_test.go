package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// enLines and deLines are the facts of a sink in one language.
func enLines(s *factSink) []string {
	var out []string
	for _, f := range s.items {
		out = append(out, f.EN)
	}
	return out
}

func deLines(s *factSink) []string {
	var out []string
	for _, f := range s.items {
		out = append(out, f.DE)
	}
	return out
}

// c: no fact line is empty or said twice.
func TestBuildFacts_NoEmptyAndNoDuplicateItems(t *testing.T) {
	t.Parallel()
	one, two := 500, 500
	r := changeReport{Changed: []reportMeal{
		{Day: "Thu 26 Nov", Meal: "breakfast", Before: []string{"Oats 1 portion"}, After: []string{"Oats 1 portion"}}, // nothing to say: was "Thu 26 Nov breakfast: ."
		{Day: "Thu 26 Nov", Meal: "dinner", Before: []string{"Chili 1 portion"}, After: []string{"Rice 1 portion"}, KcalFrom: &one, KcalTo: &two},
		{Day: "Thu 26 Nov", Meal: "dinner", Before: []string{"Chili 1 portion"}, After: []string{"Rice 1 portion"}, KcalFrom: &one, KcalTo: &two},
	}}
	in := reportInput{facts: &factSink{}}
	buildFacts(in, r)
	in.facts.add(classMoved, kindLeftover, statusOK, "Same.", "Gleich.")
	in.facts.add(classMoved, kindLeftover, statusOK, "same.", "gleich.")
	tell := tellOf(selectBlock(in.facts.items))
	seen := map[string]bool{}
	for _, s := range tell {
		assert.NotRegexp(t, emptyFact, s)
		assert.False(t, seen[s], "twice: %s", s)
		seen[s] = true
	}
	assert.Len(t, tell, 2, "%v", tell)
}

// The block shows at most five lines of classes 2 to 5 in class order; what got
// worse and new warnings win a place over what moved; the refused part is outside
// the cap.
func TestSelectBlock_CapAndOrder(t *testing.T) {
	t.Parallel()
	mk := func(class int, s string) fact {
		return fact{Class: class, Kind: kindMeal, Status: statusOK, EN: s, DE: s}
	}
	all := []fact{
		mk(classMoved, "m1"), mk(classMoved, "m2"), mk(classMoved, "m3"), mk(classMoved, "m4"),
		mk(classChanged, "c1"), mk(classChanged, "c2"),
		mk(classWorse, "w1"), mk(classWarn, "n1"),
		mk(classRefused, "r1"), mk(classRefused, "r2"),
	}
	got := tellOf(selectBlock(all))
	assert.Equal(t, []string{"r1", "r2", "c1", "c2", "m1", "w1", "n1"}, got, "refused first, then classes 2 to 5 in order, five of them")
	assert.Len(t, selectBlock(all[:8]), 5)
}

// A dish that moved and now has two new cookings: the report tells the same
// facts in the same order on every run, whatever the map order, and each cooking
// time names where the dish was cooked before. (The old "no longer cooked" line is
// not told for a dish cooked elsewhere now.)
func TestCookingFacts_MovedDishIsDeterministic(t *testing.T) {
	t.Parallel()
	foods := map[int64]planning.FoodFacts{1: {Name: "Lachs", Minutes: 40}}
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	one := 1.0
	mk := func(sessions []planning.Session, plates ...planning.Plate) planning.State {
		return planning.State{
			Foods: foods, SlotNames: map[int64]string{3: "slot.dinner"}, SlotOrder: plate.SlotOrder{3: 1},
			Window: window.Window{Start: day(21), End: day(27)}, Sessions: sessions, Plates: plates,
		}
	}
	eats := func(id int64, d int, session int64) planning.Plate {
		return planning.Plate{ID: id, Date: day(d), SlotID: 3, Eaters: 1, Components: []planning.Component{{FoodID: 1, SessionID: session, Portions: &one}}}
	}
	cook := func(id int64, d int) planning.Session {
		return planning.Session{CookSession: plate.CookSession{ID: id, FoodID: 1, CookDate: day(d), CookSlotID: 3, Portions: 1}}
	}
	bs := mk([]planning.Session{cook(1, 23)}, eats(1, 23, 1))
	as := mk([]planning.Session{cook(2, 27), cook(3, 24)}, eats(2, 27, 2), eats(3, 24, 3))

	var first []string
	for i := 0; i < 30; i++ {
		in := reportInput{beforeSt: bs, afterSt: as, facts: &factSink{}}
		cookFacts(in, changeReport{})
		leftovers(bs, as, in.facts, nil)
		got := enLines(in.facts)
		if i == 0 {
			first = got
			continue
		}
		assert.Equal(t, first, got, "run %d", i)
	}
	assert.Equal(t, []string{
		`Tue 24 Nov at dinner you cook "Lachs" for Tue 24 Nov dinner (cooked Mon 23 Nov dinner before); the longest recipe takes about 40 min.`,
		`Fri 27 Nov at dinner you cook "Lachs" for Fri 27 Nov dinner (cooked Mon 23 Nov dinner before); the longest recipe takes about 40 min.`,
	}, first, "the earliest new cooking comes first, and the dish is not told as no longer cooked")
}

func ptrF(v float64) *float64 { return &v }
