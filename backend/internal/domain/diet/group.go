package diet

import "strings"

// Group is the food group a leaf counts toward in the DGE weekly food rules.
// There is no food category in the library, so it is derived from the name
// like Classify: the same folded en/de keyword matching (see classify.go).
//
// Source: DGE "Gut essen und trinken" 2024,
// https://www.dge.de/gesunde-ernaehrung/gut-essen-und-trinken/dge-empfehlungen/
// (meat and sausage at most 300 g per week, fish 1-2 times per week,
// legumes at least once per week).
type Group string

// The groups. Fruit and vegetables are not told here: the planner uses the
// produce shop section for them.
const (
	GroupNone   Group = ""
	GroupMeat   Group = "meat"
	GroupFish   Group = "fish"
	GroupLegume Group = "legume"
)

// legumes are the keywords of pulses and their soy products. Decisions: peas
// (Erbsen) and beans count, as the DGE counts them among the Hülsenfrüchte,
// but green beans are a vegetable (greenBeans are cut out first). Tofu,
// tempeh, edamame and hummus count as legume products; soy sauce and soy
// drinks do not match. Peanuts (Erdnüsse) are botanically legumes but eaten
// as nuts, so they do not count.
var legumes = foldKeywords([]string{
	"bohne", "beans", "linse", "lentil", "kichererbse", "chickpea", "erbse",
	"=peas", "tofu", "tempeh", "edamame", "hummus", "lupine", "lupin-",
})

// greenBeans are cut out of a name before the legume keywords run.
var greenBeans = []string{
	" gruene bohne", " gruenen bohne", " green bean", " string bean", " prinzessbohne",
	" buschbohne", " stangenbohne", " brechbohne",
}

// Not meat: fats and binders whose grams are negligible.
var notMeat = map[string]bool{"gelatin": true, "schmalz": true, "=lard": true}

// Not fish: Worcestershire sauce is a seasoning.
var notFish = map[string]bool{"worcester": true}

var meatKeywords, fishKeywords []string

func init() {
	for _, kw := range animal[0].keywords { // Omnivore
		if !notMeat[kw] {
			meatKeywords = append(meatKeywords, kw)
		}
	}
	for _, kw := range animal[1].keywords { // Pescetarian
		if !notFish[kw] {
			fishKeywords = append(fishKeywords, kw)
		}
	}
}

// GroupOf derives the DGE food group of a leaf name: meat (including sausage
// and ham), fish (including seafood), legume, or none. Phrases cut out the
// non-animal look-alikes first (Hühnerei, Butternut), and a vegan/vegetarian
// marker ("Vegane Wurst") makes the food none; meat wins over fish over
// legume when a name holds several.
func GroupOf(name string) Group {
	whole := " " + strings.Join(words(name), " ") + " "
	rest := whole
	fish := false
	for _, p := range phrases {
		if strings.Contains(rest, p.phrase) {
			fish = fish || p.class == Pescetarian
			rest = strings.ReplaceAll(rest, p.phrase, " ")
		}
	}
	ws := strings.Fields(rest)
	if anyMatch(ws, markers[0].keywords) || anyMatch(ws, markers[1].keywords) {
		return GroupNone
	}
	switch {
	case anyMatch(ws, meatKeywords):
		return GroupMeat
	case fish || anyMatch(ws, fishKeywords):
		return GroupFish
	}
	for _, g := range greenBeans {
		whole = strings.ReplaceAll(whole, g, " ")
	}
	if anyMatch(strings.Fields(whole), legumes) {
		return GroupLegume
	}
	return GroupNone
}
