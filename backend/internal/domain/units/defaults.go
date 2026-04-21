// Package units holds the canonical unit vocabulary for recipe ingredients
// and the universal mass/volume defaults used when no ingredient-specific
// portion is available.
package units

import "strings"

// Kind classifies a unit for UI labelling and conversion policy.
type Kind int

const (
	// KindMass units are direct mass conversions (g, kg, mg, oz).
	KindMass Kind = iota
	// KindVolume units depend on ingredient density. A universal default here
	// assumes water density (1 g/ml) and is therefore approximate.
	KindVolume
	// KindCount units have no universal gram weight (clove, piece, egg).
	// These require a per-ingredient portion record to resolve.
	KindCount
)

// Default describes a universal (ingredient-independent) unit → grams
// conversion. Volume entries are approximate (water density assumption).
type Default struct {
	Grams       float64
	Kind        Kind
	Approximate bool // true for volume entries that assume water density
}

// Defaults maps normalized unit keys to their universal gram conversion.
// Only masses and common volumes live here — count units (clove, piece,
// egg, slice) stay out because they vary per ingredient.
var Defaults = map[string]Default{
	"g":    {Grams: 1, Kind: KindMass},
	"kg":   {Grams: 1000, Kind: KindMass},
	"mg":   {Grams: 0.001, Kind: KindMass},
	"oz":   {Grams: 28.3495, Kind: KindMass},
	"lb":   {Grams: 453.592, Kind: KindMass},
	"ml":   {Grams: 1, Kind: KindVolume, Approximate: true},
	"l":    {Grams: 1000, Kind: KindVolume, Approximate: true},
	"cl":   {Grams: 10, Kind: KindVolume, Approximate: true},
	"dl":   {Grams: 100, Kind: KindVolume, Approximate: true},
	"tbsp": {Grams: 15, Kind: KindVolume, Approximate: true},
	"tsp":  {Grams: 5, Kind: KindVolume, Approximate: true},
	"cup":  {Grams: 240, Kind: KindVolume, Approximate: true},
	"floz": {Grams: 29.5735, Kind: KindVolume, Approximate: true},
	"pt":   {Grams: 473.176, Kind: KindVolume, Approximate: true},
	"qt":   {Grams: 946.353, Kind: KindVolume, Approximate: true},
	"gal":  {Grams: 3785.41, Kind: KindVolume, Approximate: true},
}

// CountUnits is a set of canonical count-unit keys. These carry no
// universal gram weight and must be resolved via a per-ingredient portion.
// Resolvers use this set to report "grams required" errors distinctly from
// "unknown unit" errors.
var CountUnits = map[string]struct{}{
	"piece":  {},
	"clove":  {},
	"slice":  {},
	"bunch":  {},
	"pinch":  {},
	"stick":  {},
	"can":    {},
	"jar":    {},
	"packet": {},
	"stalk":  {},
	"pod":    {},
	"head":   {},
	"leaf":   {},
	"leaves": {},
	"sprig":  {},
}

// aliases maps user-entered tokens (lowercased, trimmed, dot-less) to
// canonical unit keys. Keep in sync with the importer's German parser.
var aliases = map[string]string{
	// Mass
	"g":        "g",
	"gr":       "g",
	"gram":     "g",
	"grams":    "g",
	"gramm":    "g",
	"kg":       "kg",
	"kilogram": "kg",
	"mg":       "mg",
	"oz":       "oz",
	"ounce":    "oz",
	"ounces":   "oz",
	"lb":       "lb",
	"lbs":      "lb",
	"pound":    "lb",
	"pounds":   "lb",
	// Volume
	"ml":          "ml",
	"milliliter":  "ml",
	"millilitre":  "ml",
	"milliliters": "ml",
	"l":           "l",
	"liter":       "l",
	"litre":       "l",
	"liters":      "l",
	"cl":          "cl",
	"dl":          "dl",
	"tbsp":        "tbsp",
	"tb":          "tbsp",
	"tbs":         "tbsp",
	"tablespoon":  "tbsp",
	"tablespoons": "tbsp",
	"el":          "tbsp",
	"essl":        "tbsp",
	"esslöffel":   "tbsp",
	"tsp":         "tsp",
	"ts":          "tsp",
	"teaspoon":    "tsp",
	"teaspoons":   "tsp",
	"tl":          "tsp",
	"teel":        "tsp",
	"teelöffel":   "tsp",
	"cup":         "cup",
	"cups":        "cup",
	"floz":        "floz",
	"fl oz":       "floz",
	"fluidounce":  "floz",
	"pt":          "pt",
	"pint":        "pt",
	"pints":       "pt",
	"qt":          "qt",
	"quart":       "qt",
	"quarts":      "qt",
	"gal":         "gal",
	"gallon":      "gal",
	"gallons":     "gal",
	// Count — canonical keys that CountUnits tracks.
	"piece":    "piece",
	"pieces":   "piece",
	"pc":       "piece",
	"pcs":      "piece",
	"stk":      "piece",
	"stück":    "piece",
	"stueck":   "piece",
	"clove":    "clove",
	"cloves":   "clove",
	"zehe":     "clove",
	"zehen":    "clove",
	"slice":    "slice",
	"slices":   "slice",
	"scheibe":  "slice",
	"scheiben": "slice",
	"bunch":    "bunch",
	"bunches":  "bunch",
	"bund":     "bunch",
	"pinch":    "pinch",
	"pinches":  "pinch",
	"prise":    "pinch",
	"prisen":   "pinch",
	"stick":    "stick",
	"sticks":   "stick",
	"stange":   "stick",
	"stangen":  "stick",
	"can":      "can",
	"cans":     "can",
	"dose":     "can",
	"dosen":    "can",
	"jar":      "jar",
	"jars":     "jar",
	"glas":     "jar",
	"gläser":   "jar",
	"packet":   "packet",
	"packets":  "packet",
	"pck":      "packet",
	"päckchen": "packet",
	"packung":  "packet",
	"stalk":    "stalk",
	"stalks":   "stalk",
	"pod":      "pod",
	"pods":     "pod",
	"head":     "head",
	"heads":    "head",
	"leaf":     "leaf",
	"leaves":   "leaves",
	"sprig":    "sprig",
	"sprigs":   "sprig",
	// "serving" is used for OFF-imported per-serving portions; no default grams.
	"serving":  "serving",
	"servings": "serving",
	"portion":  "serving",
	"portions": "serving",
}

// Normalize maps a user-entered unit token to its canonical key.
// Returns "" when input is empty. Unknown tokens pass through unchanged
// (lowercased + trimmed) so ingredient-specific portions can still match.
func Normalize(unit string) string {
	t := strings.ToLower(strings.TrimSpace(unit))
	t = strings.TrimSuffix(t, ".")
	if t == "" {
		return ""
	}
	if canonical, ok := aliases[t]; ok {
		return canonical
	}
	return t
}

// IsCount reports whether the normalized unit is a count unit (per-ingredient
// conversion required).
func IsCount(normalized string) bool {
	_, ok := CountUnits[normalized]
	return ok
}

// LookupDefault returns the universal default for a normalized unit, or
// (Default{}, false) if none exists.
func LookupDefault(normalized string) (Default, bool) {
	d, ok := Defaults[normalized]
	return d, ok
}
