package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// candidateSplit names, for every composed recipe the chooser is not offered,
// the reason, from the same filter eligible() applies.
func TestCandidateSplit(t *testing.T) {
	mk := func(id int64, role food.Role, class diet.Class) FoodFacts {
		return FoodFacts{ID: id, Name: "F", Kind: food.KindComposed, Role: role, DietClass: class}
	}
	in := ChooseInput{
		State: State{
			Foods: map[int64]FoodFacts{
				1: mk(1, food.RoleMain, "vegan"),
				2: mk(2, food.RoleMain, "omnivore"),    // diet_incompatible
				3: mk(3, food.RoleSideVeg, "vegan"),    // replaced
				4: mk(4, food.RoleSideVeg, "vegan"),    // kept: the swapped side's role
				5: mk(5, food.RoleSideStarch, "vegan"), // side_swap_other_role
				6: {ID: 6, Name: "Leaf", Kind: food.KindLeaf},
			},
			Profile: Profile{DietType: profile.DietVegan},
		},
		Avoid: []int64{3},
		Side:  &SideSwap{FoodID: 3, Role: food.RoleSideVeg},
	}
	included, excluded := candidateSplit(in)
	var inc []int64
	for _, f := range included {
		inc = append(inc, f.ID)
	}
	assert.Equal(t, []int64{4}, inc)
	got := map[int64]string{}
	for _, e := range excluded {
		got[e.ID] = e.Reason
	}
	assert.Equal(t, map[int64]string{
		1: excludeSideRole, 2: excludeDiet, 3: excludeReplaced, 5: excludeSideRole,
	}, got)
	kept, _ := eligibleSplit(in)
	assert.Equal(t, len(eligible(in)), len(kept), "one filter")
}

// GATE-2: with the Chef on, a recipe over the allowance in every target cell
// is not offered to the chooser.
func TestCandidateSplit_ChefSlotTime(t *testing.T) {
	slow := FoodFacts{ID: 1, Name: "Slow", Kind: food.KindComposed, Role: food.RoleMain, Minutes: 45}
	fast := FoodFacts{ID: 2, Name: "Fast", Kind: food.KindComposed, Role: food.RoleMain, Minutes: 20}
	tue := plate.CellKey{Date: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), SlotID: 1}
	in := ChooseInput{
		State: State{
			Foods:    map[int64]FoodFacts{1: slow, 2: fast},
			Profile:  Profile{MinutesWeekday: 30, MinutesWeekend: 60},
			Settings: Settings{ChefOn: true},
		},
		Target: Target{Cells: []plate.CellKey{tue}},
	}
	kept, dropped := eligibleSplit(in)
	if assert.Len(t, kept, 1) && assert.Len(t, dropped, 1) {
		assert.Equal(t, int64(2), kept[0].ID)
		assert.Equal(t, excludeSlotTime, dropped[0].Reason)
	}
	in.State.Settings.ChefOn = false
	kept, _ = eligibleSplit(in)
	assert.Len(t, kept, 2)
}
