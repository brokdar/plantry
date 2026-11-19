package planning

import (
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Pairing rules for a no-cook add-on (a gap filler, a protein add, a side
// bread). The library has roles, shop sections, DGE food groups and names but
// no cuisine or dish-type data, so a rule is only as good as those: where a
// rule needs a dish kind the data cannot give, the conservative version holds.
//
//   - Bread, fruit and plain dairy (yogurt, skyr, quark) go with most meals:
//     no rule.
//   - Cheese is never a filler. Whether a dish takes cheese needs a dish kind
//     (pasta, gratin) that the data lacks, and a cheese on a salad that
//     already holds parmesan or on a curry is the mistake the household named.
//   - Tofu, tempeh and eggs never go on a plate that holds meat or fish (the
//     DGE groups of its foods say so): a second protein of another kind next
//     to a meat main is not a meal anyone cooks.
//   - Meat and fish leaves (smoked salmon, ham) never go on a plate that
//     already holds meat or fish.
//   - Nuts go with breakfast, with salads and with dairy; not on a curry or a
//     ragù.
//   - A food the user removed earlier in the conversation (State.Avoid) is
//     never added again.
var (
	cheeseWords = []string{"cheese", "kase", "parmesan", "mozzarella", "feta", "cheddar", "gouda", "emmental", "ricotta", "halloumi", "mascarpone", "pecorino", "gruyere"}
	nutWords    = []string{"nut", "nuss", "mandel", "almond", "peanut", "erdnuss", "cashew", "walnuss", "pistazie", "pistachio", "haselnuss"}
	dairyWords  = []string{"yogurt", "joghurt", "skyr", "quark", "milk", "milch", "kefir"}
	saladWords  = []string{"salad", "salat"}
)

// breakfastWords are the names of leaves that are breakfast food.
var breakfastWords = []string{"muesli", "musli", "oat", "hafer", "porridge", "cereal", "cornflake", "granola"}

// MealFits reports whether food f belongs at a meal of slot (the plate's slot
// short name) at all, whatever else is on the plate: a breakfast dish
// (a recipe tagged Breakfast or Brunch, a recipe that is a whole standalone
// meal planned nowhere else, a leaf named muesli, oats, porridge, cereal) is
// not food for lunch or dinner unless the plan already eats it there, and a
// sweet leaf that is not a dessert only where the plan already has one at a
// lunch or dinner. Planner and chat tools share it.
func (s State) MealFits(f FoodFacts, slot string) bool {
	if slot == "breakfast" {
		return true
	}
	if s.breakfastFood(f) && !s.eatenAtMeals(func(g FoodFacts) bool { return g.ID == f.ID }) {
		return false
	}
	if f.Sweet && f.LeafRole != food.RoleDessert && !s.eatenAtMeals(func(g FoodFacts) bool { return g.Sweet }) {
		return false
	}
	return true
}

// breakfastFood: the data says so by tag or name; a standalone recipe is a
// whole meal, never an add-on.
func (s State) breakfastFood(f FoodFacts) bool {
	if f.Composed() {
		for _, t := range f.Tags {
			if strings.EqualFold(t, "breakfast") || strings.EqualFold(t, "brunch") {
				return true
			}
		}
		return false
	}
	name := diet.Fold(f.Name)
	return slices.ContainsFunc(breakfastWords, func(w string) bool { return strings.Contains(name, w) })
}

// eatenAtMeals reports whether a lunch or dinner plate of the plan holds a
// food that matches.
func (s State) eatenAtMeals(match func(FoodFacts) bool) bool {
	for _, p := range s.Plates {
		if !slices.ContainsFunc(lightSlots, func(n string) bool { return s.slotIs(p.SlotID, n) }) {
			continue
		}
		for _, c := range p.Components {
			if g, ok := s.Foods[c.FoodID]; ok && match(g) {
				return true
			}
		}
	}
	return false
}

// addOnFits reports whether leaf f may be added to plate p (slot is the
// plate's slot short name).
func (s State) addOnFits(p Plate, f FoodFacts, slot string) bool {
	if !s.PairAddOns {
		return true
	}
	if s.Avoid[f.ID] || !s.MealFits(f, slot) || (f.Composed() && f.Role == food.RoleStandalone) {
		return false // a standalone recipe is a whole meal, never an add-on
	}
	name := diet.Fold(f.Name)
	if f.Composed() {
		// A recipe added or swapped in as a side: only the protein pairing
		// applies (cheese and nuts are leaf rules).
		return s.pairsWithPlate(p, f, name, slot)
	}
	has := func(words []string) bool {
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(name, w) })
	}
	if has(cheeseWords) {
		return false
	}
	if has(nutWords) && !strings.Contains(name, "coconut") && !strings.Contains(name, "kokos") {
		dairyOnPlate, saladMain := false, false
		for _, c := range p.Components {
			g, ok := s.Foods[c.FoodID]
			if !ok {
				continue
			}
			gn := diet.Fold(g.Name)
			for _, w := range dairyWords {
				dairyOnPlate = dairyOnPlate || strings.Contains(gn, w)
			}
			if c.FoodID == s.mainOf(p) {
				// A salad beside a ragù does not make the plate a salad: only a
				// main that is a salad takes nuts.
				for _, w := range saladWords {
					saladMain = saladMain || strings.Contains(gn, w)
				}
			}
		}
		return slot == "breakfast" || dairyOnPlate || saladMain
	}
	return s.pairsWithPlate(p, f, name, slot)
}

// pairsWithPlate is the protein pairing of a food (leaf or recipe) next to a
// plate: tofu, tempeh, egg, meat and fish never go on a plate that holds meat
// or fish.
func (s State) pairsWithPlate(p Plate, f FoodFacts, name, slot string) bool {
	meatOrFish := false
	for _, c := range p.Components {
		if g, ok := s.Foods[c.FoodID]; ok && (g.Groups.MeatG > 0 || g.Groups.FishG > 0) {
			meatOrFish = true
		}
	}
	words := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == ',' || r == '(' || r == ')' })
	tofuOrEgg := slices.ContainsFunc(words, func(w string) bool {
		return strings.HasPrefix(w, "tofu") || strings.HasPrefix(w, "tempeh") || w == "egg" || w == "eggs" || w == "ei" || w == "eier"
	})
	if meatOrFish && (tofuOrEgg || f.Groups.MeatG > 0 || f.Groups.FishG > 0) {
		return false
	}
	return true
}
