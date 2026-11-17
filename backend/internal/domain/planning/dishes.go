package planning

import (
	"maps"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// DefaultMaxDishMeals is plan.max_dish_meals_per_week's default: a dish is
// the main of at most this many lunches or dinners a week (soft rule S8).
const DefaultMaxDishMeals = 3

// MaxDishMeals is the S8 cap, the default when the setting is unset (a
// hand-built state).
func (s Settings) MaxDishMeals() int {
	if s.MaxDishMealsPerWeek <= 0 {
		return DefaultMaxDishMeals
	}
	return s.MaxDishMealsPerWeek
}

// mainOf is the food id of p's main, 0 without one: the component whose
// position is main; on a plate without stored positions (a proposal) the
// first recipe whose role counts as a lunch or dinner main.
func (s State) mainOf(p Plate) int64 {
	positioned := false
	for _, c := range p.Components {
		if c.Position == plate.PositionMain {
			return c.FoodID
		}
		positioned = positioned || c.Position != ""
	}
	if positioned {
		return 0
	}
	for _, c := range p.Components {
		if s.countsAsMain(c.FoodID) {
			return c.FoodID
		}
	}
	return 0
}

// proposedMain is the main of a proposed plate: its first recipe whose role
// counts as a lunch or dinner main.
func (s State) proposedMain(pp ProposedPlate) int64 {
	for _, c := range pp.Components {
		if s.countsAsMain(c.FoodID) {
			return c.FoodID
		}
	}
	return 0
}

func (s State) countsAsMain(id int64) bool {
	f, ok := s.Foods[id]
	return ok && f.Composed() && isMain(f.Role, mealFull)
}

// dishMeals counts, per main, the lunch and dinner plates of the window
// that have it as their main.
func (x *index) dishMeals() map[int64]int {
	out := map[int64]int{}
	for _, p := range x.s.Plates {
		if !x.s.Window.Contains(p.Date) || x.mealOf(p.SlotID) != mealFull {
			continue
		}
		if id := x.s.mainOf(p); id != 0 {
			out[id]++
		}
	}
	return out
}

// dishIssues reports S8 `dish_repeats_exceeded` for every recipe that is the
// main of more lunches or dinners in the window than the setting allows. No
// date and no options; ordered by food id.
func (x *index) dishIssues() []Issue {
	limit := x.s.Settings.MaxDishMeals()
	counts := x.dishMeals()
	var out []Issue
	for _, id := range slices.Sorted(maps.Keys(counts)) {
		if n := counts[id]; n > limit {
			out = append(out, issue("S8", CodeDishRepeats, SeveritySoft).
				with("food_id", id).with("name", x.s.Foods[id].Name).with("meals", n).with("max", limit))
		}
	}
	return out
}

// dishMeals is how many lunch or dinner plates of the window have id as
// their main so far: the fixed plates and the ones proposed until now.
func (h *planner) dishMeals(id int64) int {
	n := 0
	for _, p := range h.s.Plates {
		if h.s.Window.Contains(p.Date) && h.x.mealOf(p.SlotID) == mealFull && h.s.mainOf(p) == id {
			n++
		}
	}
	for _, pp := range h.prop.Plates {
		if h.s.Window.Contains(pp.Date) && h.x.mealOf(pp.SlotID) == mealFull && h.s.proposedMain(pp) == id {
			n++
		}
	}
	return n
}

// dishFull reports whether id is already the main of as many lunches or
// dinners as S8 allows.
func (h *planner) dishFull(id int64) bool {
	return h.dishMeals(id) >= h.s.Settings.MaxDishMeals()
}
