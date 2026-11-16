package units

import (
	"slices"
	"strings"
)

// pieceWeights holds the grams of one piece (or clove, or leaf) of an
// ingredient that recipes count rather than weigh. Order matters: the first
// entry with a matching keyword wins, so a specific name ("frühlingszwiebel",
// "süßkartoffel") precedes the generic one it contains. Weights are USDA
// SR Legacy portions where one exists (a medium onion, a large egg, a clove
// of garlic); the rest are typical edible weights. An ingredient not listed
// falls back to CountGrams.
//
// Keywords match as a substring of the lowercased name; "=word" matches a
// whole word only, for short words ("ei") that sit inside others ("eis").
//
// units says which count units the weight describes: an egg has no clove,
// and a can or a bunch of an ingredient is not a piece of it.
var pieceWeights = []struct {
	grams    float64
	units    []string
	keywords []string
}{
	{3, []string{"piece", "clove"}, []string{"knoblauch", "garlic"}}, // a counted garlic "piece" is a clove
	{15, []string{"piece"}, []string{"frühlingszwiebel", "lauchzwiebel", "spring onion", "scallion"}},
	{110, []string{"piece"}, []string{"zwiebel", "onion"}},
	{18, []string{"piece"}, []string{"eigelb", "egg yolk", "=yolk", "=yolks"}},
	{33, []string{"piece"}, []string{"eiweiß", "egg white", "=white", "=whites"}},
	{50, []string{"piece"}, []string{"=ei", "=eier", "=egg", "=eggs"}},
	{30, []string{"piece"}, []string{"ciabatta"}}, // recipes count it by the slice
	{50, []string{"piece"}, []string{"brötchen", "semmel", "=bun", "=buns", "=roll", "=rolls"}},
	{40, []string{"piece", "stalk", "stick"}, []string{"sellerie", "celery"}},
	{130, []string{"piece"}, []string{"süßkartoffel", "sweet potato"}},
	{170, []string{"piece"}, []string{"kartoffel", "potato"}},
	{61, []string{"piece"}, []string{"karotte", "möhre", "carrot"}},
	{17, []string{"piece"}, []string{"cocktailtomate", "kirschtomate", "cherry tomato"}},
	{120, []string{"piece"}, []string{"tomate", "tomato"}},
	{119, []string{"piece"}, []string{"paprika", "bell pepper"}},
	{15, []string{"piece"}, []string{"peperoni", "chili", "chilli"}},
	{196, []string{"piece"}, []string{"zucchini", "courgette"}},
	{500, []string{"piece"}, []string{"brokkoli", "broccoli"}}, // one head, like CountGrams["head"]
	{0.6, []string{"piece", "leaf"}, []string{"lorbeer", "bay leaf", "bay leaves"}},
}

// EstimateCountGrams is the grams of one count unit of the named ingredient
// when nobody has weighed it: the ingredient's own weight for a piece, clove
// or leaf it describes, else the unit's CountGrams default (0 for an unknown
// unit, and for a piece of an ingredient not listed: nobody can say what an
// unknown thing weighs, so the review asks). The
// importer's draft and Finalize both call it, so the number the review shows
// is the one that is stored.
func EstimateCountGrams(unit, name string) float64 {
	lower := strings.ToLower(name)
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return (r < 'a' || r > 'z') && !strings.ContainsRune("äöüß", r)
	})
	for _, w := range pieceWeights {
		if !slices.Contains(w.units, unit) {
			continue
		}
		for _, kw := range w.keywords {
			if whole, ok := strings.CutPrefix(kw, "="); ok {
				if slices.Contains(words, whole) {
					return w.grams
				}
			} else if strings.Contains(lower, kw) {
				return w.grams
			}
		}
	}
	if unit == "piece" {
		return 0
	}
	return CountGrams[unit]
}
