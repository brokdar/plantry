package planning

import (
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// mealKind is what a cell asks of its plate: a full meal needs a main, a
// light meal (breakfast, snack) takes anything.
type mealKind uint8

const (
	mealFull mealKind = iota
	mealLight
)

// mainRoles says which recipe roles count as the main of a plate, per meal
// kind. A standalone recipe (oats, yoghurt with berries) is a whole
// breakfast but no lunch or dinner; a role-less recipe is trusted as a main.
// A light meal needs no main: every role is fine there.
var mainRoles = map[mealKind][]food.Role{
	mealFull: {food.RoleMain, ""},
}

// isMain reports whether a recipe of role r is the main of a plate of kind k.
func isMain(r food.Role, k mealKind) bool {
	return k == mealLight || slices.Contains(mainRoles[k], r)
}

// suits is the candidate table's suitability column: where a recipe of role
// r can be the plate's main ("lunch_dinner" for mains, "breakfast" for
// standalone recipes) or what it accompanies.
func suits(r food.Role) string {
	switch {
	case isMain(r, mealFull):
		return "lunch_dinner"
	case r == food.RoleStandalone:
		return "breakfast"
	case sideRole(r):
		return "with_main"
	}
	return "extra"
}

// mealOf is the kind of slot's cells. The day's first slot is a light meal
// when the day has at least three slots, and so is the last one with at
// least four; a slot with a routine cell in the window is a habit meal
// (breakfast oats) too. Names play no part: a renamed breakfast stays light.
func (x *index) mealOf(slot int64) mealKind {
	order, ok := x.s.SlotOrder[slot]
	if ok && len(x.s.SlotOrder) >= 3 {
		rank := 0
		for id, o := range x.s.SlotOrder {
			if id != slot && o < order {
				rank++
			}
		}
		if rank == 0 || (len(x.s.SlotOrder) >= 4 && rank == len(x.s.SlotOrder)-1) {
			return mealLight
		}
	}
	for _, c := range x.s.Cells {
		if c.SlotID == slot && x.routine(plate.CellKey{Date: c.Date, SlotID: slot}) {
			return mealLight
		}
	}
	return mealFull
}

// hasFullMains reports whether the library holds a recipe that can be a
// lunch or dinner main; without one nothing can be repaired.
func hasFullMains(s State) bool {
	for _, f := range s.Foods {
		if f.Composed() && isMain(f.Role, mealFull) {
			return true
		}
	}
	return false
}
