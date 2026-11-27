package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// The chooser is not offered an archived recipe, and H8's eligible count
// does not count it.
func TestArchivedIsNoCandidate(t *testing.T) {
	mk := func(id int64, archived bool) FoodFacts {
		return FoodFacts{ID: id, Name: "F", Kind: food.KindComposed, Role: food.RoleMain, Archived: archived}
	}
	in := ChooseInput{State: State{Foods: map[int64]FoodFacts{1: mk(1, false), 2: mk(2, true)}}}
	kept, _ := eligibleSplit(in)
	assert.Equal(t, []int64{1}, []int64{kept[0].ID})
	assert.Len(t, kept, 1)

	x := &index{s: in.State}
	x.s.Cells = nil
	assert.Equal(t, 1, x.eligibleRecipes(4))
}
