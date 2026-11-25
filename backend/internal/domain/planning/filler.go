package planning

import (
	"maps"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Filler fit (CP1): a no-cook filler (addNoCook's kcal and protein adds, a
// band move of one; plate fit on) or a leftover plate's varied side (batch
// variation) joins a plate only when a cook would serve it there. On top of the add-on pairing rules (addOnFits, which the
// planner's own top-ups otherwise skip), beside a savoury lunch or dinner
// main:
//
//  1. Breakfast and sweet foods stay at breakfast: no muesli, oats, jam or
//     honey, no fresh fruit or fruit salad, no milk or kefir. Cultured dairy
//     (yogurt, skyr, quark; one per plate) only as a dollop or cold sauce
//     where the dish takes one: a curry, stew, chili, soup, wrap, bowl, salad,
//     potato or fish dish, or an Indian, Middle Eastern, Mexican or
//     Mediterranean main; never a pasta, bake, stir-fry, East Asian main or
//     a main that carries cheese (a Greek salad: cheeseDishWords).
//  2. A protein filler (meat, fish, tofu, tempeh, egg, a legume) never joins
//     a plate that already carries protein (its main or another component).
//  3. On a plate without one, it must fit the dish: tofu and tempeh with a
//     curry, stir-fry or bowl (or an Asian main); egg with a salad, bowl,
//     curry, stir-fry or potato dish; cold fish and meat with a salad, bowl,
//     wrap, bread or potato dish (never a hot soup or pasta); a legume (hummus,
//     chickpeas) with a salad, bowl, wrap, curry, stew or soup (or a Middle
//     Eastern, Indian or Mexican main).
//
// A cheese or nut leaf is a topping where the dish takes one (toppingFits:
// parmesan on a pasta, feta on a salad, cashews on a curry). A plate holds its
// main and at most maxPlateSides sides and add-ons.
//
// Vegetables, bread (not a second starch) and the rest join as before. When
// no filler fits, the plate keeps its gap; the safety floor lift skips all of
// this (any verified food).

// fillerClass is what a filler leaf is to a plate.
type fillerClass int

const (
	fillOther fillerClass = iota
	fillBreakfast
	fillFruit
	fillDrink
	fillDairy
	fillProtein
)

var (
	jamWords       = []string{"marmalade", "marmela", "konfituere", "honig", "honey", "nutella"} // and the word jam
	drinkWords     = []string{"milk", "milch", "kefir", "buttermilch", "buttermilk"}
	fruitLeafWords = []string{"apfel", "apple", "banan", "orange", "mandarin", "clementin", "beere", "berry", "berries", "birne", "pear", "kiwi", "mango", "traube", "grape", "pfirsich", "peach", "nektarin", "melon", "ananas", "pineapple", "pflaume", "plum", "kirsch", "cherr", "aprikose", "apricot", "obst", "fruit", "frucht"}
	tofuWords      = []string{"tofu", "tempeh", "seitan"}
	eggWords       = []string{"egg", "eggs", "ei", "eier", "omelette", "omelet", "frittata", "ruehrei"}
	proteinWords   = []string{"tofu", "tempeh", "seitan", "halloumi", "paneer", "omelette", "omelet", "frittata", "ruehrei", "schnitzel", "falafel"}
)

// Dish kinds of a main, from its kind attribute or else its name.
var dishKindWords = map[string][]string{
	"curry":   {"curry", "dal", "dhal", "masala", "korma", "tikka"},
	"stew":    {"stew", "eintopf", "gulasch", "goulash", "chili", "ragout", "tagine", "casserole"},
	"soup":    {"soup", "suppe", "broth", "bruehe", "minestrone"},
	"salad":   {"salad", "salat"},
	"wrap":    {"wrap", "burrito", "taco", "tortilla", "quesadilla", "fajita", "kebab", "doener", "pita", "sandwich", "burger"},
	"stirfry": {"stir-fry", "stir fry", "stirfry", "wok", "fried rice", "noodle", "teriyaki", "pad thai", "ramen"},
	"bowl":    {"bowl"},
	"potato":  {"kartoffel", "potato", "roesti"},
	"bread":   {"brot", "bread", "toast", "bruschetta"},
	"bake":    {"gratin", "auflauf", "ueberbacken", "lasagn"},
	"egg":     {"shakshuka", "omelett", "frittata", "ruehrei"},
}

// cheeseDishWords name mains that already carry cheese without naming it: a
// gratin or lasagne is baked under cheese, a Greek salad or caprese holds
// feta or mozzarella. Such a main takes no cheese topping and no dairy.
var cheeseDishWords = []string{"gratin", "ueberbacken", "lasagn", "greek salad", "griechischer salat", "caprese", "pizza"}

// asianCuisines are the cuisines whose mains take tofu.
var asianCuisines = []string{"chinese", "japanese", "thai", "indian"}

// nameWords splits a folded name into words.
func nameWords(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == ',' || r == '(' || r == ')' || r == '-' || r == '/' })
}

// hasWholeWord reports whether one of words is a word of name (short words
// such as "ei" or "jam" match only whole).
func hasWholeWord(name string, words []string) bool {
	return slices.ContainsFunc(nameWords(name), func(w string) bool { return slices.Contains(words, w) })
}

func containsAny(name string, words []string) bool {
	return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(name, w) })
}

// classifyFiller is the class of filler leaf f (name folded).
func classifyFiller(f FoodFacts, name string) fillerClass {
	switch {
	case containsAny(name, breakfastWords) || containsAny(name, jamWords) || hasWholeWord(name, []string{"jam"}):
		return fillBreakfast
	case containsAny(name, drinkWords) && !containsAny(name, []string{"kokos", "coconut"}):
		return fillDrink
	case containsAny(name, dairyWords):
		return fillDairy
	case containsAny(name, tofuWords) || hasWholeWord(name, eggWords) || f.Groups.MeatG > 0 || f.Groups.FishG > 0 || f.Groups.LegumeG > 0:
		return fillProtein
	case containsAny(name, fruitLeafWords) && !strings.Contains(name, "chutney"):
		return fillFruit
	}
	return fillOther
}

// dishKinds are the kinds of main m (its kind attribute, else its name).
func dishKinds(m FoodFacts) map[string]bool {
	out := map[string]bool{}
	if m.Attrs.Kind != "" {
		out[m.Attrs.Kind] = true
	}
	name := diet.Fold(m.Name)
	for k, words := range dishKindWords {
		if containsAny(name, words) {
			out[k] = true
		}
	}
	if containsAny(name, pastaWords) {
		out["pasta"] = true
	}
	return out
}

// carriesProtein reports whether food g is a protein source on a plate: meat,
// fish or legumes, a protein side, tofu, egg or cheese mains by name, or a
// quarter or more of its energy from protein.
func carriesProtein(g FoodFacts) bool {
	name := diet.Fold(g.Name)
	if g.Groups.MeatG+g.Groups.FishG+g.Groups.LegumeG >= 30 || g.Role == food.RoleSideProtein ||
		containsAny(name, proteinWords) || hasWholeWord(name, eggWords) {
		return true
	}
	return g.Macros.Kcal > 0 && g.Macros.Protein*4 >= g.Macros.Kcal*0.25
}

// fillerFits reports whether leaf or no-cook recipe f may fill plate p (slot
// is the plate's slot short name). Plate fit off: always.
func (s State) fillerFits(p Plate, f FoodFacts, slot string) bool {
	return !s.Settings.PlateFit || s.addFits(p, f, slot)
}

// addFits is the filler rule itself (see the top of this file), for any
// no-cook food joining a plate: a filler, or a batch variation side.
func (s State) addFits(p Plate, f FoodFacts, slot string) bool {
	pair := s
	pair.PairAddOns = true
	m, savoury := s.savouryMain(p, slot)
	if savoury && s.sidesBeside(p, f) >= maxPlateSides {
		return false // no clutter: a main and at most two sides or add-ons
	}
	name := diet.Fold(f.Name)
	pair.Settings.PlateFit = true // a cheese or nut by toppingFits
	if !pair.addOnFits(p, f, slot) {
		return false
	}
	if !savoury || f.Composed() {
		return true // no savoury main to judge by; a no-cook recipe add is judged by addOnFits
	}
	kinds := dishKinds(m)
	in := func(ks ...string) bool { return slices.ContainsFunc(ks, func(k string) bool { return kinds[k] }) }
	cuisine := m.Attrs.Cuisine
	switch classifyFiller(f, name) {
	case fillBreakfast, fillFruit, fillDrink:
		return false
	case fillDairy:
		// A dollop or a cold sauce (raita, tzatziki, dill yogurt with fish,
		// herb quark with potatoes); not on a pasta, bake or stir-fry.
		if in("pasta", "bake", "stirfry") || slices.Contains(asianCuisines[:3], cuisine) || containsAny(diet.Fold(m.Name), cheeseDishWords) {
			return false
		}
		fish := m.Groups.FishG > 0 || in("fish")
		return fish || in("curry", "stew", "soup", "wrap", "potato", "bowl", "salad") ||
			slices.Contains([]string{"indian", "middle_eastern", "mexican", "mediterranean"}, cuisine)
	case fillProtein:
		for _, c := range p.Components {
			if g, ok := s.Foods[c.FoodID]; ok && c.FoodID != f.ID && carriesProtein(g) {
				return false
			}
		}
		switch {
		case containsAny(name, tofuWords):
			return in("curry", "stirfry", "bowl") || slices.Contains(asianCuisines, cuisine)
		case hasWholeWord(name, eggWords):
			return in("salad", "bowl", "curry", "stirfry", "potato")
		case f.Groups.MeatG > 0 || f.Groups.FishG > 0:
			return in("salad", "bowl", "wrap", "bread", "potato") && !in("soup", "pasta", "stew", "curry")
		default: // a legume
			return in("salad", "bowl", "wrap", "curry", "stew", "soup") || slices.Contains([]string{"middle_eastern", "indian", "mexican"}, cuisine)
		}
	}
	return true
}

// savouryMain is plate p's main when it is a savoury lunch or dinner main
// (not sweet, not a dessert, not a breakfast dish; a shakshuka tagged brunch
// is one).
func (s State) savouryMain(p Plate, slot string) (FoodFacts, bool) {
	m, ok := s.Foods[s.mainOf(p)]
	return m, (slot == "lunch" || slot == "dinner") && ok && !m.Sweet && m.Role != food.RoleDessert &&
		!containsAny(diet.Fold(m.Name), breakfastWords)
}

// maxPlateSides caps a lunch or dinner plate's sides and add-ons beside its
// main (CP1: "a dinner of six items"); a tip's garnish is not one.
const maxPlateSides = 2

// sidesBeside counts plate p's components other than its main and f.
func (s State) sidesBeside(p Plate, f FoodFacts) int {
	mainID, n := s.mainOf(p), 0
	for _, c := range p.Components {
		if c.FoodID != mainID && c.FoodID != f.ID {
			n++
		}
	}
	return n
}

// toppingFits reports whether a cheese or nut leaf (name folded) tops plate p
// with savoury main m. Cheese: on a pasta, risotto, bake, salad, bowl,
// potato or egg dish, wrap, bread or chili, or an Italian, French, Mediterranean or
// Mexican main; never on a curry, stir-fry, soup, fish or Asian main, beside fish, or on
// a plate that already has a cheese (in its main's name too: no feta on a
// feta quiche, no cheese on a gratin or Greek salad: cheeseDishWords). Nuts: on a salad, bowl, curry or stir-fry, never a pasta.
func (s State) toppingFits(p Plate, m FoodFacts, name string) bool {
	kinds := dishKinds(m)
	in := func(ks ...string) bool { return slices.ContainsFunc(ks, func(k string) bool { return kinds[k] }) }
	cuisine := m.Attrs.Cuisine
	if !containsAny(name, cheeseWords) && !strings.Contains(name, "kaese") {
		return in("salad", "bowl", "curry", "stirfry") && !in("pasta")
	}
	if in("curry", "stirfry", "soup", "fish") || slices.Contains(asianCuisines, cuisine) || containsAny(diet.Fold(m.Name), cheeseDishWords) {
		return false
	}
	for _, c := range p.Components {
		g, ok := s.Foods[c.FoodID]
		gn := diet.Fold(g.Name)
		if ok && (g.Groups.FishG > 0 || containsAny(gn, cheeseWords) || strings.Contains(gn, "kaese")) {
			return false
		}
	}
	return in("pasta", "bake", "salad", "bowl", "potato", "wrap", "bread", "egg") || strings.Contains(diet.Fold(m.Name), "chili") ||
		slices.Contains([]string{"italian", "french", "mediterranean", "mexican"}, cuisine)
}

// dropMisfitAdds takes off the proposal's new plates the no-cook adds (a
// component with no cook session) that no longer fit their plate because a
// later pass changed its main (guardDayMains, guardWeek, fixQuick,
// fixVariety): a yogurt chosen for a curry does not stay on the minestrone
// that replaced it. An add stays when taking it off would raise a hard issue
// (the safety floor first); otherwise the plate keeps its gap. Plate fit off:
// the proposal as it is.
func dropMisfitAdds(base State, t Target, in Proposal) Proposal {
	if !base.Settings.PlateFit {
		return in
	}
	out := in
	var hard map[string]bool
	for i := range out.Plates {
		for ci := len(out.Plates[i].Components) - 1; ci >= 0; ci-- {
			pc := out.Plates[i].Components[ci]
			if out.Plates[i].PlateID != 0 || pc.SessionKey != "" || pc.SessionID != 0 {
				continue
			}
			merged := Merge(base, t, out)
			mi := slices.IndexFunc(merged.Plates, func(p Plate) bool { return p.ID == -int64(i+1) })
			if mi < 0 || merged.mainOf(merged.Plates[mi]) == pc.FoodID {
				continue
			}
			p := merged.Plates[mi]
			if merged.fillerFits(withoutComp(p, ci), merged.Foods[pc.FoodID], slotName(merged, p.SlotID)) {
				continue
			}
			if hard == nil {
				hard = hardKeys(evaluateLean(merged, t).Issues)
			}
			next := cloneProposal(out)
			next.Plates[i].Components = slices.Delete(next.Plates[i].Components, ci, ci+1)
			after := hardKeys(evaluateLean(Merge(base, t, next), t).Issues)
			if slices.ContainsFunc(slices.Collect(maps.Keys(after)), func(k string) bool { return !hard[k] }) {
				continue
			}
			out, hard = next, after
		}
	}
	return out
}
