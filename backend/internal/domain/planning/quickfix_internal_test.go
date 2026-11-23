package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// V73: the quick wish is judged by the cooking time at the meal. A slow dish
// cooked on Sunday and eaten on Monday's limited dinner as a leftover costs
// nothing there: only a batch cooked in the limited cell counts as slow.
func TestSlowBatch_OnlyTheCookCellCounts(t *testing.T) {
	t.Parallel()
	sun := time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC)
	mon := sun.AddDate(0, 0, 1)
	const dinner, stew int64 = 3, 7
	s := State{
		Foods:      map[int64]FoodFacts{stew: {ID: stew, Kind: food.KindComposed, Role: food.RoleMain, Minutes: 90}},
		MaxMinutes: map[cellID]int{idOf(plate.CellKey{Date: mon, SlotID: dinner}): 30},
	}
	prop := Proposal{
		Sessions: []ProposedSession{{Key: "s1", FoodID: stew, CookDate: sun, CookSlotID: dinner, Portions: 2}},
		Plates: []ProposedPlate{
			{Date: sun, SlotID: dinner, Components: []ProposedComponent{{FoodID: stew, Portions: 1, SessionKey: "s1"}}},
			{Date: mon, SlotID: dinner, Components: []ProposedComponent{{FoodID: stew, Portions: 1, SessionKey: "s1"}}},
		},
	}
	key, _ := slowBatch(s, prop, map[string]bool{})
	assert.Empty(t, key, "a leftover at the limited dinner is no slow batch")

	prop.Sessions[0].CookDate = mon // the stew now cooks at the limited dinner
	key, limit := slowBatch(s, prop, map[string]bool{})
	assert.Equal(t, "s1", key)
	assert.Equal(t, 30, limit)
}

// Review fix: a quick swap never brings back a food the user removed
// (ChooseInput.Avoid / AvoidBatches, State.Avoid) or the run excludes.
func TestFixQuick_NeverSwapsInAnAvoidedFood(t *testing.T) {
	t.Parallel()
	mon := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	const dinner, stew, fast int64 = 3, 7, 8
	mains := func() map[int64]FoodFacts {
		return map[int64]FoodFacts{
			stew: {ID: stew, Name: "Stew", Kind: food.KindComposed, Role: food.RoleMain, Minutes: 90, NutritionComplete: true, Macros: nutrition.Macros{Kcal: 600}},
			fast: {ID: fast, Name: "Fast", Kind: food.KindComposed, Role: food.RoleMain, Minutes: 15, NutritionComplete: true, Macros: nutrition.Macros{Kcal: 600}},
		}
	}
	run := func(mod func(*ChooseInput)) int64 {
		in := ChooseInput{
			State:  State{Foods: mains(), MaxMinutes: map[cellID]int{idOf(plate.CellKey{Date: mon, SlotID: dinner}): 30}},
			Target: Target{Cells: []plate.CellKey{{Date: mon, SlotID: dinner}}},
		}
		mod(&in)
		prop := Proposal{
			Sessions: []ProposedSession{{Key: "s1", FoodID: stew, CookDate: mon, CookSlotID: dinner, Portions: 1}},
			Plates:   []ProposedPlate{{Date: mon, SlotID: dinner, Components: []ProposedComponent{{FoodID: stew, Portions: 1, SessionKey: "s1"}}}},
		}
		got, _ := fixQuick(in, prop)
		return got.Sessions[0].FoodID
	}
	require.Equal(t, fast, run(func(*ChooseInput) {}), "the fixture swaps in the quick main when nothing is avoided")
	assert.Equal(t, stew, run(func(in *ChooseInput) { in.Avoid = []int64{fast} }))
	assert.Equal(t, stew, run(func(in *ChooseInput) { in.AvoidBatches = []int64{fast} }))
	assert.Equal(t, stew, run(func(in *ChooseInput) { in.State.Avoid = map[int64]bool{fast: true} }))
}
