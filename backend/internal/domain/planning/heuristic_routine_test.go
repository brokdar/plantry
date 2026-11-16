package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// The routine breakfast recipe (a standalone, as the seeded overnight oats)
// is never planned into lunch or dinner, and mains are picked before other
// standalone recipes.
func TestHeuristic_RoutineRecipeIsNotALunchOrDinner(t *testing.T) {
	s := batchCooker()
	o := s.Foods[oats]
	o.Role = food.RoleStandalone
	o.LastEaten = nil
	s.Foods[oats] = o
	// A second, unrelated standalone must rank behind every main.
	wrap := recipe(301, "Wrap", food.RoleStandalone, "omnivore", 600)
	s.Foods[301] = wrap

	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	for _, ps := range p.Sessions {
		assert.NotEqual(t, oats, ps.FoodID, "the routine's recipe is no cook")
		assert.NotEqual(t, int64(301), ps.FoodID, "a main is picked while mains remain")
	}
}

// A recipe with role main stays plannable even when a routine plate eats it:
// with it as the only main, it is the one that gets cooked.
func TestHeuristic_RoutineMainStaysPlannable(t *testing.T) {
	s := batchCooker()
	o := s.Foods[oats]
	o.Role = food.RoleMain
	o.LastEaten = nil
	s.Foods[oats] = o
	for id, f := range s.Foods {
		if f.Role == food.RoleMain && id != oats {
			delete(s.Foods, id)
		}
	}

	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})

	cooked := false
	for _, ps := range p.Sessions {
		cooked = cooked || ps.FoodID == oats
	}
	assert.True(t, cooked, "a real main is plannable although a routine eats it")
}
