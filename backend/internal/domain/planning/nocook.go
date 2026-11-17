package planning

import (
	"regexp"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

// TypicalLeafGrams is the portion a no-cook leaf is offered in: the library
// stores no typical portion for a leaf.
const TypicalLeafGrams = 100

// NoCook reports whether f needs no cooking. The library has no flag for
// it, so a leaf counts when its store section (the user's override or the
// classified name) is produce, dairy or bakery and its name is not a
// food that is cooked first (potato, egg, pulses); a recipe counts when its
// cook minutes are set to 0 and it is not a main.
func NoCook(f *food.Food) bool {
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
	return !cookedFirst.MatchString(f.Name)
}

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
