package diet

import (
	"slices"
	"strings"
)

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
	"cannellini", "borlotti", "falafel",
})

// greenBeans are cut out of a name before the legume keywords run.
var greenBeans = []string{
	" gruene bohne", " gruenen bohne", " green bean", " string bean", " prinzessbohne",
	" buschbohne", " stangenbohne", " brechbohne",
}

// notLegumes are cut out of a name like greenBeans: herbs, seeds and
// flavourings named after beans, and pea protein, an isolate and not a legume
// meal (the English name never matched, so the German one agrees).
var notLegumes = []string{
	"bohnenkraut", "kaffeebohne", "kakaobohne", "tonkabohne", "coffee bean",
	"cocoa bean", "cacao bean", "tonka bean", "erbsenprotein", "pea protein",
}

// plantMarkers make a name none: a plant-based stand-in for meat or fish.
// Words match like keywords, phrases are looked for in the folded name.
var (
	plantWords   = foldKeywords([]string{"pflanzlich", "plantbased", "meatfree", "fleischlos", "fleischfrei"})
	plantPhrases = []string{" plant based ", " meat free "}
)

// seasonings are broths, animal fats and oils, and fish and oyster sauces:
// their words are dropped from a name before the meat and fish keywords run
// ("Hühnerbrühe", "Gänseschmalz", "Lachsöl"), so only real food counts. Grams
// of stock are mostly water and fat grams are negligible. "Gemüsebrühe mit
// Huhn" still has its Huhn.
var seasonings = foldKeywords([]string{
	"brühe", "fond", "=stock", "=broth", "bouillon", "schmalz", "fett", "öl",
	"lebertran", "fischsauce", "fischsosse", "austernsauce", "austernsosse",
})

// seasoningNouns are the English seasonings written as a separate word; the
// word before them ("beef" in "beef broth") is dropped with them.
var seasoningNouns = []string{"stock", "broth", "bouillon", "fat", "oil"}

// seasoningPhrases are the same for names of several words.
var seasoningPhrases = []string{" fish sauce ", " oyster sauce "}

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
	for _, p := range seasoningPhrases {
		rest = strings.ReplaceAll(rest, p, " ")
	}
	var ws []string
	for _, w := range strings.Fields(rest) {
		switch {
		case slices.Contains(seasoningNouns, w) && len(ws) > 0:
			ws = ws[:len(ws)-1] // "Beef broth": the word before names the animal
		case !anyMatch([]string{w}, seasonings):
			ws = append(ws, w)
		}
	}
	if anyMatch(ws, markers[0].keywords) || anyMatch(ws, markers[1].keywords) || anyMatch(ws, plantWords) {
		return GroupNone
	}
	for _, p := range plantPhrases {
		if strings.Contains(whole, p) {
			return GroupNone
		}
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
	for _, g := range notLegumes {
		whole = strings.ReplaceAll(whole, g, " ")
	}
	if anyMatch(strings.Fields(whole), legumes) {
		return GroupLegume
	}
	return GroupNone
}
