package planning

import (
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
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

// mealOf is the kind of slot's cells, read from the slot's kind setting
// (State.SlotKind). A slot without an entry is a full meal. Names, position
// and routine cells play no part.
func (x *index) mealOf(id int64) mealKind {
	if x.s.SlotKind[id] == slot.KindLight {
		return mealLight
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
