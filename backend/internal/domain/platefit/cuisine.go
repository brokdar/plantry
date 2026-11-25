// Package platefit reads a plate the way a cook would (spec §5.3, PLATE-1..3,
// 8): from the recipe attributes of its components it lists the clashes and
// the gaps over the ten PLATE-1 dimensions. The reading is computed by rules,
// without a model call, and is deterministic. An unknown attribute (empty, nil)
// is no judgment: a gap is raised only when the known values prove it.
package platefit

import "strings"

// Style is a cuisine's pairing style (PLATE-3).
type Style string

// Pairing styles.
const (
	// Bridge: a side shares the main's flavours (the cuisine's palette).
	Bridge Style = "bridge"
	// Contrast: a side supplies what the main lacks: something cool or sour.
	Contrast Style = "contrast"
)

// Cuisine is one row of the cuisine table: pairing style, palette and
// thresholds.
type Cuisine struct {
	Name  string
	Style Style // empty for the neutral fallback: no pairing judgment
	// Group is the coherence group: sides of one group mix on a plate. Empty
	// for neutral, which mixes with everything.
	Group string
	// Palette are lower-case name fragments of the sides and finishes that
	// belong to the cuisine; a side whose name or hero contains one bridges.
	Palette []string
	// Rich is the richness from which a plate needs acid (taste balance) and
	// a heavy component counts as rich (richness).
	Rich int
	// Heat is the heat from which a plate needs something cool.
	Heat int
	// SoftOK: soft plates are the norm (braises, curries, stews), so an
	// all-soft plate is only a minor gap.
	SoftOK bool
}

// Neutral is the fallback for a main of no defined cuisine.
const Neutral = "neutral"

var cuisines = map[string]Cuisine{
	"german": {
		Style: Bridge, Group: "german", Rich: 2, Heat: 2, SoftOK: true,
		Palette: []string{"gurke", "dill", "schmand", "sauerrahm", "kümmel", "kraut", "kohl", "senf", "petersilie", "essig", "kartoffel", "apfel"},
	},
	"italian": {
		Style: Bridge, Group: "west_med", Rich: 2, Heat: 2,
		Palette: []string{"rucola", "parmesan", "basilikum", "petersilie", "zitrone", "olive", "tomate", "gremolata"},
	},
	"french": {
		Style: Bridge, Group: "west_med", Rich: 2, Heat: 2, SoftOK: true,
		Palette: []string{"salade", "kopfsalat", "feldsalat", "vinaigrette", "kräuter", "cornichon", "estragon", "petersilie", "kartoffel", "bohne"},
	},
	"mediterranean": {
		Style: Bridge, Group: "west_med", Rich: 2, Heat: 2, SoftOK: true,
		Palette: []string{"zitrone", "petersilie", "basilikum", "olive", "rucola", "tomate", "gurke", "feta", "zucchini", "paprika", "aubergine"},
	},
	"indian": {
		Style: Contrast, Group: "indian", Rich: 2, Heat: 2, SoftOK: true,
		Palette: []string{"raita", "joghurt", "kachumber", "gurke", "limette", "koriander", "zwiebel", "chutney", "reis", "naan"},
	},
	"chinese": {
		Style: Contrast, Group: "chinese", Rich: 2, Heat: 2,
		Palette: []string{"gurke", "frühlingszwiebel", "sesam", "chili", "pak choi", "reis"},
	},
	"japanese": {
		Style: Contrast, Group: "japanese", Rich: 2, Heat: 2,
		Palette: []string{"gurke", "sunomono", "frühlingszwiebel", "sesam", "ingwer", "nori", "reis"},
	},
	"thai": {
		Style: Contrast, Group: "thai", Rich: 2, Heat: 2,
		Palette: []string{"limette", "minze", "basilikum", "koriander", "erdnüss", "gurke", "reis"},
	},
	"mexican": {
		Style: Contrast, Group: "mexican", Rich: 2, Heat: 2,
		Palette: []string{"limette", "rote zwiebel", "salsa", "avocado", "kohl", "koriander", "reis"},
	},
	"middle_eastern": {
		Style: Contrast, Group: "middle_eastern", Rich: 2, Heat: 2,
		Palette: []string{"joghurt", "gurke", "petersilie", "tabouleh", "granatapfel", "sumach", "minze", "hummus", "fladenbrot"},
	},
	Neutral: {Rich: 2, Heat: 2, SoftOK: true},
}

// CuisineOf returns the table row of name; anything not in the table (empty,
// neutral, another cuisine) gets the neutral fallback.
func CuisineOf(name string) Cuisine {
	c, ok := cuisines[name]
	if !ok {
		name = Neutral
		c = cuisines[Neutral]
	}
	c.Name = name
	return c
}

// inPalette reports whether one of names contains a palette fragment.
func (c Cuisine) inPalette(names ...string) bool {
	for _, n := range names {
		n = strings.ToLower(n)
		for _, p := range c.Palette {
			if strings.Contains(n, p) {
				return true
			}
		}
	}
	return false
}
