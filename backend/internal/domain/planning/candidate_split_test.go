package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
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
