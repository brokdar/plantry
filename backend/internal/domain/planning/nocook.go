package planning

import (
	"regexp"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

// TypicalLeafGrams is the portion a no-cook leaf is offered in: the library
// stores no typical portion for a leaf.
const TypicalLeafGrams = 100

// NoCook reports whether f needs no cooking: the user's override
// (food.Food.NoCookOverride) first, else a derivation. A leaf counts when its store section (the user's override or the
// classified name) is produce, dairy or bakery and its name is not a
// food that is cooked first (potato, egg, pulses); a recipe counts when its
// cook minutes are set to 0 and it is not a main.
func NoCook(f *food.Food) bool {
	if f.NoCookOverride != nil {
		return *f.NoCookOverride
	}
	return derivedNoCook(f)
}

// NoCookSource reports where NoCook's answer for f comes from: "user" or
// "derived".
func NoCookSource(f *food.Food) string {
	if f.NoCookOverride != nil {
		return "user"
	}
	return "derived"
}

func derivedNoCook(f *food.Food) bool {
	if f.Kind == food.KindComposed {
		return f.CookMinutes != nil && *f.CookMinutes == 0 && (f.Role == nil || *f.Role != food.RoleMain)
	}
	section := shopping.Classify(f.Name)
	if f.ShopSection != nil {
		section = *f.ShopSection
	}
	switch section {
	case food.SectionProduce, food.SectionDairyEggs, food.SectionBakery:
	default:
		return false
	}
	name := eatenAsIsFat.ReplaceAllString(f.Name, " ")
	return !cookedFirst.MatchString(name) && !notEatenAsIs.MatchString(name)
}

// ServingGrams is the grams the planner offers f in: the stored serving
// size, else TypicalLeafGrams.
func ServingGrams(f FoodFacts) float64 {
	if f.ServingG > 0 {
		return f.ServingG
	}
	return TypicalLeafGrams
}

// notEatenAsIs names the produce, dairy and bakery foods that go into a dish
// and are never a plate add-on: herbs, spices, aromatics, fats and oils,
// cream, broths, pastes, vinegar, flour, starch, pastry and dough. A match
// only changes the derived default; the user's override wins.
//
// English words match as whole words with an optional plural s (salted
// almonds, pasteurized, buttermilk and creamy do not match). German words
// match as the end of a compound on purpose (Rapsöl, Tomatenmark, Olivenöl),
// with an optional inflection ending.
var notEatenAsIs = regexp.MustCompile(`(?i)(^|[^\p{L}])(` +
	`(herb|basil|parsley|thyme|chive|coriander|cilantro|mint|dill|rosemary|oregano|sage|bay leaf|spice|seasoning|salt|peppercorn|black pepper|cayenne|cumin|turmeric|cinnamon|` +
	`garlic|onion|shallot|leek|ginger|chil+i|chile|scallion|` +
	`oil|butter|ghee|lard|margarine|tallow|cooking cream|crème|creme|` +
	`broth|stock|bouillon|paste|puree|purée|vinegar|juice|sauce|starch|pastry|dough|batter)(s)?($|[^\p{L}])` +
	`|cream(s)?($|[^\p{L} ]| [^c]))|` +
	`(kräuter|kraeuter|petersilie|thymian|schnittlauch|koriander|minze|dill|rosmarin|salbei|lorbeer|gewürz|salz|pfeffer|kümmel|kurkuma|zimt|` +
	`knoblauch|zwiebel|schalotte|lauch|ingwer|` +
	`öl|oel|butter|schmalz|margarine|sahne|kochcreme|` +
	`brühe|bruehe|fond|bouillon|tomatenmark|mark|essig|saft|soße|sosse|stärke|staerke|teig|mehl)(n|en|e|s)?($|[^\p{L}])`)

// eatenAsIsFat names the fat-named foods that are eaten as they are: nut
// butters and a food packed in oil, stripped from the name before
// notEatenAsIs applies.
var eatenAsIsFat = regexp.MustCompile(`(?i)((peanut|nut|almond|cashew|hazelnut|cocoa|cacao|shea) butter|(erdnuss|mandel|cashew|haselnuss|nuss|kakao)butter| in (öl|oel|oil))`)

var cookedFirst = regexp.MustCompile(`(?i)\b(potato|kartoffel|egg|ei|eier|bean|bohne|lentil|linse|pea|erbse|rice|reis|pasta|nudel|flour|mehl|dough|teig|yeast|hefe|corn ?starch|pudding)`)

// Sweet thresholds (per 100 g): the library has no food category, so a sweet
// is told by its sugar content. Fruit (about 10-15 g sugar, plenty of fibre)
// and bread (under 5 g) stay fillers; candy, chocolate, jam, honey and cake
// (20 g and more sugar, little fibre) are free-sugar foods (WHO: under 10 %
// of energy) and never fill a kcal gap. A leaf without sugar data is no sweet.
const (
	sweetSugarG = 20
	sweetFiberG = 5
)

// IsSweet reports whether f is a leaf that is mostly sugar (see sweetSugarG).
func IsSweet(f *food.Food) bool {
	if f.Kind != food.KindLeaf || f.Sugar100g == nil || *f.Sugar100g < sweetSugarG {
		return false
	}
	return f.Fiber100g == nil || *f.Fiber100g < sweetFiberG
}
