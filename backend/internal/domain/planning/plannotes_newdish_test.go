package planning_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// FIND-3 (CP3 dry run): when the plan leaves the week's new dish out, the
// saved note names it and says why in kitchen words; a placed one, or New
// dishes 0, adds no line.
func TestSavedPlanNotes_NewDishThatDidNotFit(t *testing.T) {
	newDish := func(s planning.State, never bool) planning.State {
		f := recipe(130, "Mild chicken curry", food.RoleMain, "omnivore", 600)
		f.ExploreTag, f.Never = true, never
		s.Foods[130] = f
		return s
	}
	line := func(s planning.State, quota int, final planning.Proposal) string {
		in := planning.ChooseInput{State: s, Target: windowTarget(s), ExploreQuota: quota}
		for _, l := range planning.SavedPlanNotes(in, final, final) {
			if strings.HasPrefix(l, "The new dish") {
				return l
			}
		}
		return ""
	}

	s := newDish(batchCooker(), false)
	assert.Equal(t, "The new dish Mild chicken curry did not fit this week: no meal this week had room for it.", line(s, 1, planning.Proposal{}))
	assert.Empty(t, line(s, 0, planning.Proposal{}), "New dishes 0")

	never := newDish(batchCooker(), true)
	assert.Equal(t, "The new dish Mild chicken curry did not fit this week: it holds something you never eat.", line(never, 1, planning.Proposal{}))

	placed := planning.Proposal{
		Sessions: []planning.ProposedSession{session("a", 130, sat, dinner, 1)},
		Plates: []planning.ProposedPlate{{
			Date: day(sat), SlotID: dinner, Eaters: 1,
			Components: []planning.ProposedComponent{{FoodID: 130, Portions: 1, SessionKey: "a"}},
		}},
	}
	assert.Empty(t, line(s, 1, placed), "placed")
}
