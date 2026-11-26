package platefit

import (
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Dimension is one of the ten PLATE-1 dimensions.
type Dimension string

// Dimensions, in reading order.
const (
	TasteBalance Dimension = "taste_balance"
	Fresh        Dimension = "fresh"
	Texture      Dimension = "texture"
	Colour       Dimension = "colour"
	Temperature  Dimension = "temperature"
	CuisineFit   Dimension = "cuisine"
	Pairing      Dimension = "pairing"
	SauceStarch  Dimension = "sauce_starch"
	Proportions  Dimension = "proportions"
	Richness     Dimension = "richness"
)

// Dimensions are the ten PLATE-1 dimensions in reading order.
var Dimensions = []Dimension{
	TasteBalance, Fresh, Texture, Colour, Temperature,
	CuisineFit, Pairing, SauceStarch, Proportions, Richness,
}

// Component is one food on a plate with what the reading needs of it.
type Component struct {
	ID    int64
	Name  string
	Role  food.Role
	Leaf  bool // a garnish leaf: counts for taste, texture, colour, fresh and palette only
	Attrs flavor.Attributes
	// Fresh: a raw, green or citrus element. Nil is unknown.
	Fresh *bool
	// Starch: the component carries a starch. Nil is unknown, except that a
	// side_starch is a starch.
	Starch *bool
}

// Plate is a main and its sides and finishes.
type Plate struct {
	Main  Component
	Sides []Component
}

func (p Plate) all() []Component { return append([]Component{p.Main}, p.Sides...) }

// Gap is what a plate lacks. A minor gap is below the cuisine's threshold: a
// ranking signal, never a tip.
type Gap struct {
	Dimension Dimension
	Minor     bool
}

// Reading is a plate's plate fit: clashes and gaps, each in dimension order.
type Reading struct {
	Cuisine string // the main's cuisine after the neutral fallback
	Clashes []Dimension
	Gaps    []Gap
}

// Raised returns the clashes and the gaps that are not minor (Gap{d} is the
// major gap of d), in dimension order: what a plate shows; a good plate
// raises nothing.
func (r Reading) Raised() []Dimension {
	var out []Dimension
	for _, d := range Dimensions {
		if slices.Contains(r.Clashes, d) || slices.Contains(r.Gaps, Gap{Dimension: d}) {
			out = append(out, d)
		}
	}
	return out
}

// tri is a three-valued answer: unknown when the known values cannot decide.
type tri int

const (
	no tri = iota
	yes
	unknown
)

// anyOf answers whether some component matches: yes when one with a known
// value matches, no when every value is known and none matches.
func anyOf(cs []Component, known, match func(Component) bool) tri {
	out := no
	for _, c := range cs {
		switch {
		case !known(c):
			out = unknown
		case match(c):
			return yes
		}
	}
	return out
}

func dishes(cs []Component) []Component {
	return slices.DeleteFunc(slices.Clone(cs), func(c Component) bool { return c.Leaf })
}

func hasTaste(c Component) bool { return c.Attrs.Taste != nil }

func acid(n int) func(Component) bool {
	return func(c Component) bool { return c.Attrs.Taste.Acid >= n }
}

func isCold(c Component) bool { return c.Attrs.Temperature == "cold" }

func hasTemperature(c Component) bool { return c.Attrs.Temperature != "" }

func isStarch(c Component) bool {
	if c.Starch != nil {
		return *c.Starch
	}
	return c.Role == food.RoleSideStarch
}

func knowsStarch(c Component) bool { return c.Starch != nil || c.Role == food.RoleSideStarch }

func heavy(c Component) bool { return c.Role == food.RoleMain || c.Role == food.RoleSideStarch }

func (c Component) richness() int {
	if c.Attrs.Richness == nil {
		return 0
	}
	return *c.Attrs.Richness
}

func (c Component) heat() int {
	if c.Attrs.Taste == nil {
		return 0
	}
	return c.Attrs.Taste.Heat
}

// Proven returns c with every unknown value that could close a gap set to
// one that closes none (not fresh, no starch unless its role is one, soft,
// beige, hot, a flat taste, neutral cuisine). A candidate read as Proven
// settles only the gaps its known values settle: a blank side never wins by
// leaving a gap unknown.
func Proven(c Component) Component {
	no := false
	if c.Fresh == nil {
		c.Fresh = &no
	}
	if c.Starch == nil && c.Role != food.RoleSideStarch {
		c.Starch = &no
	}
	if c.Attrs.Texture == "" {
		c.Attrs.Texture = "soft"
	}
	if c.Attrs.Colour == "" {
		c.Attrs.Colour = "beige"
	}
	if c.Attrs.Temperature == "" && !c.Leaf {
		c.Attrs.Temperature = "hot"
	}
	if c.Attrs.Taste == nil {
		c.Attrs.Taste = &flavor.Taste{}
	}
	if c.Attrs.Cuisine == "" {
		c.Attrs.Cuisine = Neutral
	}
	return c
}

// Read returns the plate fit of p. It is pure: the same plate gives the same
// reading.
func Read(p Plate) Reading {
	cu := CuisineOf(p.Main.Attrs.Cuisine)
	all := p.all()
	ds := dishes(all)
	r := Reading{Cuisine: cu.Name}
	clash := func(d Dimension, ok bool) {
		if ok {
			r.Clashes = append(r.Clashes, d)
		}
	}
	gap := func(d Dimension, ok, minor bool) {
		if ok {
			r.Gaps = append(r.Gaps, Gap{Dimension: d, Minor: minor || slices.Contains(cu.Relaxed, d)})
		}
	}
	sharp := anyOf(all, hasTaste, acid(2)) == yes // an acid element that brightens

	maxRich, maxHeat := 0, 0
	for _, c := range all {
		maxRich = max(maxRich, c.richness())
		maxHeat = max(maxHeat, c.heat())
	}
	gap(TasteBalance, maxRich >= cu.Rich && anyOf(all, hasTaste, acid(1)) == no, false)
	gap(Fresh, anyOf(all, func(c Component) bool { return c.Fresh != nil }, func(c Component) bool { return *c.Fresh }) == no, sharp)
	gap(Texture, anyOf(all, func(c Component) bool { return c.Attrs.Texture != "" }, func(c Component) bool {
		return c.Attrs.Texture == "crisp" || c.Attrs.Texture == "crunchy"
	}) == no, false)
	gap(Colour, anyOf(all, func(c Component) bool { return c.Attrs.Colour != "" }, func(c Component) bool {
		return !slices.Contains([]string{"beige", "brown", "white"}, c.Attrs.Colour)
	}) == no, false)
	cold := anyOf(ds, hasTemperature, isCold)
	gap(Temperature, maxHeat >= cu.Heat && cold == no, sharp)
	clash(CuisineFit, !groupsMix(ds))
	gap(Pairing, pairingGap(cu, p, cold), false)
	saucy := p.Main.Attrs.Saucy != nil && *p.Main.Attrs.Saucy
	gap(SauceStarch, saucy && anyOf(ds, knowsStarch, isStarch) == no, false)

	starches, mains, richHeavy, richSum := 0, 0, 0, 0
	for _, c := range ds {
		if isStarch(c) {
			starches++
		}
		if c.Role == food.RoleMain {
			mains++
		}
		if heavy(c) && c.richness() >= cu.Rich {
			richHeavy++
			richSum += c.richness()
		}
	}
	clash(Proportions, starches >= 2 || mains >= 2)
	clash(Richness, richHeavy >= 2 && richSum >= richSumMax)
	return r
}

// richSumMax: two rich heavy components whose richness adds up to this are
// one rich element too many.
const richSumMax = 5

// mixingGroups are the pairs of coherence groups whose sides share a plate
// the way their home cooking does: German food takes the West-Mediterranean
// salads and vegetables (green salad, carrot or tomato salad), the Eastern
// and Western Mediterranean meet, and a cool yogurt-cucumber side belongs to
// both the Middle-Eastern and the Indian table.
var mixingGroups = map[[2]string]bool{
	{"german", "west_med"}:         true,
	{"middle_eastern", "west_med"}: true,
	{"indian", "middle_eastern"}:   true,
}

// groupsMix reports whether the known, non-neutral cuisines of cs belong to
// one coherence group or to groups that mix (mixingGroups).
func groupsMix(cs []Component) bool {
	var seen []string
	for _, c := range cs {
		if g := CuisineOf(c.Attrs.Cuisine).Group; g != "" && !slices.Contains(seen, g) {
			seen = append(seen, g)
		}
	}
	for i, a := range seen {
		for _, b := range seen[i+1:] {
			if !mixingGroups[[2]string{min(a, b), max(a, b)}] {
				return false
			}
		}
	}
	return true
}

// pairingGap applies the main cuisine's pairing style. Contrast: nothing cool
// and nothing sharp, sides or not. Bridge: there are dish sides (a garnish is
// not a side) and no side or garnish shares the main's food world (its
// coherence group or palette).
func pairingGap(cu Cuisine, p Plate, cold tri) bool {
	switch cu.Style {
	case Contrast:
		return cold == no && anyOf(p.all(), hasTaste, acid(2)) == no
	case Bridge:
		if len(dishes(p.Sides)) == 0 {
			return false // a main alone, or with garnish only, has nothing to bridge with
		}
		belongs := func(c Component) bool { return bridges(cu, c) }
		known := func(c Component) bool { return c.Leaf || c.Attrs.Cuisine != "" || belongs(c) }
		return anyOf(p.Sides, known, belongs) == no
	}
	return false
}

func bridges(cu Cuisine, side Component) bool {
	if !side.Leaf && cu.Group != "" && CuisineOf(side.Attrs.Cuisine).Group == cu.Group {
		return true
	}
	return cu.inPalette(append([]string{side.Name}, side.Attrs.Heroes...)...)
}
