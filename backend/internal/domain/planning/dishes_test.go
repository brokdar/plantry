package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

func dishIssues(ev planning.Evaluation) []planning.Issue {
	var out []planning.Issue
	for _, is := range ev.Issues {
		if is.Code == planning.CodeDishRepeats {
			out = append(out, is)
		}
	}
	return out
}

// S8: a dish that is the main of more lunches and dinners than the setting
// allows is a soft issue without date or options; breakfasts, sides and
// plates outside the window do not count.
func TestDishRepeats(t *testing.T) {
	dinners := []string{sat, sun, mon, tue}
	build := func(limit int, n int, extra ...planning.Plate) planning.Evaluation {
		s := baseState()
		s.SlotKind = map[int64]slot.Kind{breakfast: slot.KindLight}
		s.Settings.MaxDishMealsPerWeek = limit
		tgt := windowTarget(s)
		var prop planning.Proposal
		for _, d := range dinners[:n] {
			prop.Plates = append(prop.Plates, pplate(d, dinner, fresh(curry)))
		}
		prop.Plates = append(prop.Plates, pplate(wed, breakfast, fresh(curry)))
		m := planning.Merge(s, tgt, prop)
		m.Plates = append(m.Plates, extra...)
		return planning.Evaluate(m, tgt)
	}

	assert.Empty(t, dishIssues(build(0, 3)), "3 meals are the default cap; the breakfast does not count")
	got := dishIssues(build(0, 4))
	require.Len(t, got, 1)
	assert.Equal(t, "S8", got[0].Rule)
	assert.Empty(t, got[0].Date)
	assert.Empty(t, got[0].Options)
	assert.EqualValues(t, curry, got[0].Params["food_id"])
	assert.EqualValues(t, 4, got[0].Params["meals"])
	assert.EqualValues(t, 3, got[0].Params["max"])
	assert.Len(t, dishIssues(build(2, 3)), 1, "the setting lowers the cap")

	side := planning.Plate{ID: 9, Date: day(wed), SlotID: lunch, Eaters: 1, Components: []planning.Component{
		{FoodID: stew, Portions: f64(1), Position: plate.PositionMain},
		{FoodID: curry, Portions: f64(1), Position: plate.PositionSide},
	}}
	assert.Empty(t, dishIssues(build(0, 3, side)), "a stored side is not the main")

	ev := build(0, 4)
	board := planning.BoardSoftIssues(planning.State{}, ev.Issues)
	assert.NotEmpty(t, dishIssues(planning.Evaluation{Issues: board}), "the board shows S8")
}

// The heuristic keeps each dish to the S8 cap when the plan allows it (with
// one cooking occasion a day a lunch nothing else can feed may still take a
// capped batch: S8 is soft).
func TestHeuristic_DishCap(t *testing.T) {
	s := batchCooker()
	s.Settings.MaxDishMealsPerWeek = 3
	s.SlotKind = map[int64]slot.Kind{breakfast: slot.KindLight}
	s.Settings.MaxCookOccasionsPerDay = 2
	tgt := windowTarget(s)
	p := choose(t, planning.ChooseInput{State: s, Target: tgt})
	ev := planning.Evaluate(planning.Merge(s, tgt, p), tgt)
	assert.Empty(t, dishIssues(ev))
}

// A hidden substitution variant counts as its recipe for S8: two dinners of
// the variant and two of the recipe are four meals of one dish.
func TestDishRepeats_HiddenVariantCountsAsItsRecipe(t *testing.T) {
	const hidden int64 = 702
	s := baseState()
	s.SlotKind = map[int64]slot.Kind{breakfast: slot.KindLight}
	h := s.Foods[curry]
	h.ID, h.Archived, h.BaseFood = hidden, true, curry
	s.Foods[hidden] = h
	tgt := windowTarget(s)
	var prop planning.Proposal
	for _, d := range []string{sat, sun} {
		prop.Plates = append(prop.Plates, pplate(d, dinner, fresh(curry)))
	}
	m := planning.Merge(s, tgt, prop)
	for i, d := range []string{mon, tue} {
		m.Plates = append(m.Plates, planning.Plate{ID: int64(20 + i), Date: day(d), SlotID: dinner, Eaters: 1, Components: []planning.Component{
			{FoodID: hidden, Portions: f64(1), Position: plate.PositionMain},
		}})
	}
	got := dishIssues(planning.Evaluate(m, tgt))
	require.Len(t, got, 1)
	assert.EqualValues(t, curry, got[0].Params["food_id"])
	assert.EqualValues(t, 4, got[0].Params["meals"])
}
