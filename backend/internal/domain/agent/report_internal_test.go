package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
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

// The dish that moved names its earliest new cooking, whatever the map order.
func TestLeftovers_MovedDishIsDeterministic(t *testing.T) {
	t.Parallel()
	foods := map[int64]planning.FoodFacts{1: {Name: "Lachs"}}
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	mkState := func(sessions ...planning.Session) planning.State {
		return planning.State{
			Foods: foods, SlotNames: map[int64]string{3: "slot.dinner"}, Sessions: sessions,
			Window: planning.State{}.Window,
		}
	}
	bs := mkState(planning.Session{CookSession: plate.CookSession{ID: 1, FoodID: 1, CookDate: day(23), CookSlotID: 3, Portions: 1}})
	as := mkState(
		planning.Session{CookSession: plate.CookSession{ID: 2, FoodID: 1, CookDate: day(27), CookSlotID: 3, Portions: 1}},
		planning.Session{CookSession: plate.CookSession{ID: 3, FoodID: 1, CookDate: day(24), CookSlotID: 3, Portions: 1}},
	)
	bs.Plates = []planning.Plate{{ID: 1, Date: day(23), SlotID: 3, Eaters: 1, Components: []planning.Component{{SessionID: 1, Portions: ptrF(1)}}}}
	// the dish is cooked elsewhere now: the cooking-time fact says where, not "no longer cooked"
	sink := &factSink{}
	leftovers(bs, as, sink, nil)
	assert.Empty(t, sink.items)
}

func ptrF(v float64) *float64 { return &v }
