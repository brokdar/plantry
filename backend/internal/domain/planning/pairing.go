package planning

import (
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
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

// addOnFits reports whether leaf f may be added to plate p (slot is the
// plate's slot short name).
func (s State) addOnFits(p Plate, f FoodFacts, slot string) bool {
	if !s.PairAddOns {
		return true
	}
	if s.Avoid[f.ID] {
		return false
	}
	name := diet.Fold(f.Name)
	has := func(words []string) bool {
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(name, w) })
	}
	if has(cheeseWords) {
		return false
	}
	meatOrFish := false
	dairyOnPlate, saladMain := false, false
	for _, c := range p.Components {
		g, ok := s.Foods[c.FoodID]
		if !ok {
			continue
		}
		if g.Groups.MeatG > 0 || g.Groups.FishG > 0 {
			meatOrFish = true
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
	if has(nutWords) && !strings.Contains(name, "coconut") && !strings.Contains(name, "kokos") {
		return slot == "breakfast" || dairyOnPlate || saladMain
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
