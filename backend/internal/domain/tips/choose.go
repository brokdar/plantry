// Package tips computes the Chef's plate tips (spec §5.4, TIP-1..4, 6, 7): one
// sentence and an Apply that holds a checked change for one plate. A tip is
// computed by rules, without a model call: plate fit (platefit) finds what a
// plate raises, and a change is offered only when the plate read again with
// it no longer raises that dimension and raises nothing new.
package tips

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// Kind is a tip's kind (TIP-2). Technique advice is never a tip.
type Kind string

// Kinds, in the order a tie between two changes is broken.
const (
	KindAcid       Kind = "finishing_acid"
	KindHerbs      Kind = "fresh_herbs"
	KindCrunch     Kind = "crunch"
	KindPickle     Kind = "quick_pickle"
	KindCool       Kind = "cool_element"
	KindSauce      Kind = "sauce"
	KindSideSwap   Kind = "side_swap"
	KindCuisineFix Kind = "cuisine_fix"
)

var kindOrder = []Kind{KindAcid, KindHerbs, KindCrunch, KindPickle, KindCool, KindSauce, KindSideSwap, KindCuisineFix}

// Settles lists the dimensions each kind may settle, from culinary
// principle: a finishing acid balances taste and gives a hot or contrast
// plate its sharp note (plate fit reads a sharp element as cutting heat); herbs bring freshness and colour; crunch brings texture; a
// cool element cools heat and gives the contrast; a quick pickle is sharp,
// fresh and crisp at once; a sauce carries a starch or lightens a rich
// plate; a contrast side swap fixes pairing, texture, colour and the plate's
// build (sauce without a starch, two starches, two rich heavy parts); a
// cuisine fix settles a cuisine clash only. A tip of a kind is offered only
// for a dimension of its list.
var Settles = map[Kind][]platefit.Dimension{
	KindAcid:       {platefit.TasteBalance, platefit.Temperature, platefit.Pairing},
	KindHerbs:      {platefit.Fresh, platefit.Colour},
	KindCrunch:     {platefit.Texture},
	KindPickle:     {platefit.TasteBalance, platefit.Temperature, platefit.Pairing, platefit.Fresh, platefit.Texture},
	KindCool:       {platefit.Temperature, platefit.Pairing},
	KindSauce:      {platefit.SauceStarch, platefit.Richness},
	KindSideSwap:   {platefit.Pairing, platefit.Texture, platefit.Colour, platefit.SauceStarch, platefit.Proportions, platefit.Richness},
	KindCuisineFix: {platefit.CuisineFit},
}

// Change is a candidate tip for one plate: add the garnish Add, or replace
// the side at Swap (an index into Plate.Sides) with With.
type Change struct {
	Kind Kind
	// Dimension is the plate's top raised dimension, which the change
	// settles; Settles lists every raised dimension it settles, in dimension
	// order.
	Dimension platefit.Dimension
	Settles   []platefit.Dimension
	Add       *platefit.Component
	Swap      int
	With      platefit.Component
}

// Food is the food the change brings onto the plate.
func (c Change) Food() platefit.Component {
	if c.Add != nil {
		return *c.Add
	}
	return c.With
}

// sideRoles are the roles a swapped-in side can have.
var sideRoles = []food.Role{food.RoleSideStarch, food.RoleSideVeg, food.RoleSideProtein, food.RoleSauce}

// priority orders the dimensions a tip settles first: a clash (something
// wrong on the plate) before a gap (something missing); among gaps the
// plate's build and taste come before its texture, temperature and contrast,
// and freshness and colour last. Heat is settled before texture: a cool or
// sharp element is what a hot plate misses most.
var priority = []platefit.Dimension{
	platefit.CuisineFit, platefit.Proportions, platefit.Richness,
	platefit.SauceStarch, platefit.TasteBalance, platefit.Temperature,
	platefit.Pairing, platefit.Texture, platefit.Fresh, platefit.Colour,
}

// Top returns the raised dimension of r a tip must settle: the first in
// priority order.
func Top(r platefit.Reading) (platefit.Dimension, bool) {
	raised := r.Raised()
	for _, d := range priority {
		if slices.Contains(raised, d) {
			return d, true
		}
	}
	return "", false
}

// Candidates returns the changes that settle p's top raised dimension (Top)
// with a kind whose list holds it (Settles) and raise nothing p does not,
// best first: most dimensions settled, then a food of the main cuisine's
// palette (coriander on a curry, not basil), then a garnish before a swap, then
// kind order, then name. garnishes are leaves a plate can gain with no
// cooking; sides are recipes that can replace one of p's dish sides. The
// plate is read again with the candidate as it is and as Proven: the top
// dimension must be settled by the candidate's known values, and neither
// reading may raise anything new (a second starch or a cuisine clash
// included). A candidate that would hide a gap behind an unknown attribute
// is never offered (known).
func Candidates(p platefit.Plate, garnishes, sides []platefit.Component) []Change {
	before := platefit.Read(p)
	top, ok := Top(before)
	if !ok {
		return nil
	}
	raised := before.Raised()
	cu := platefit.CuisineOf(p.Main.Attrs.Cuisine)
	var out []Change
	try := func(next platefit.Plate, at int, kinds []Kind, ch Change) {
		ki := slices.IndexFunc(kinds, func(k Kind) bool { return slices.Contains(Settles[k], top) })
		if ki < 0 {
			return
		}
		proven := platefit.Plate{Main: next.Main, Sides: slices.Clone(next.Sides)}
		proven.Sides[at] = platefit.Proven(proven.Sides[at])
		after := platefit.Read(proven).Raised()
		if slices.Contains(after, top) {
			return
		}
		for _, d := range slices.Concat(after, platefit.Read(next).Raised()) {
			if !slices.Contains(raised, d) {
				return
			}
		}
		for _, d := range raised {
			if !slices.Contains(after, d) {
				ch.Settles = append(ch.Settles, d)
			}
		}
		ch.Kind, ch.Dimension = kinds[ki], top
		out = append(out, ch)
	}
	for _, g := range garnishes {
		k := garnishKind(g)
		if !g.Leaf || k == "" || onPlate(p, g.ID) || !known(p, -1, g) {
			continue
		}
		next := platefit.Plate{Main: p.Main, Sides: append(slices.Clone(p.Sides), g)}
		try(next, len(p.Sides), []Kind{k}, Change{Add: &g})
	}
	for i, old := range p.Sides {
		if old.Leaf {
			continue
		}
		for _, s := range sides {
			if s.Leaf || !slices.Contains(sideRoles, s.Role) || onPlate(p, s.ID) || !known(p, i, s) || !fits(p, i, s, slices.Contains(raised, platefit.SauceStarch)) {
				continue
			}
			next := platefit.Plate{Main: p.Main, Sides: slices.Clone(p.Sides)}
			next.Sides[i] = s
			try(next, i, swapKinds(s), Change{Swap: i, With: s})
		}
	}
	slices.SortStableFunc(out, func(a, b Change) int {
		if c := cmp.Compare(len(b.Settles), len(a.Settles)); c != 0 {
			return c
		}
		if pa, pb := inPalette(cu, a.Food()), inPalette(cu, b.Food()); pa != pb {
			if pa {
				return -1
			}
			return 1
		}
		if (a.Add == nil) != (b.Add == nil) {
			if a.Add != nil {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(slices.Index(kindOrder, a.Kind), slices.Index(kindOrder, b.Kind)); c != 0 {
			return c
		}
		return cmp.Compare(a.Food().Name, b.Food().Name)
	})
	return out
}

// fits holds the culinary checks plate fit does not read for a side s
// replacing side i: a starch never joins a dish whose starch is unknown (it
// could be a second starch), and the starch that carries a sauced main is a
// plain one, not itself saucy or rich (a gratin brings its own cream).
func fits(p platefit.Plate, i int, s platefit.Component, sauced bool) bool {
	if starch(s) {
		for j, o := range append([]platefit.Component{p.Main}, p.Sides...) {
			if j != i+1 && !o.Leaf && o.Starch == nil && o.Role != food.RoleSideStarch {
				return false
			}
		}
	}
	if sauced && starch(s) {
		saucy := s.Attrs.Saucy != nil && *s.Attrs.Saucy
		rich := s.Attrs.Richness != nil && *s.Attrs.Richness > 2
		return !saucy && !rich
	}
	return true
}

// inPalette reports whether c's name or a hero holds a fragment of cu's
// palette.
func inPalette(cu platefit.Cuisine, c platefit.Component) bool {
	for _, n := range append([]string{c.Name}, c.Attrs.Heroes...) {
		n = strings.ToLower(n)
		if slices.ContainsFunc(cu.Palette, func(f string) bool { return strings.Contains(n, f) }) {
			return true
		}
	}
	return false
}

func starch(c platefit.Component) bool {
	if c.Starch != nil {
		return *c.Starch
	}
	return c.Role == food.RoleSideStarch
}

func onPlate(p platefit.Plate, id int64) bool {
	if id == 0 {
		return false
	}
	return p.Main.ID == id || slices.ContainsFunc(p.Sides, func(c platefit.Component) bool { return c.ID == id })
}

// field reports whether a component knows one attribute plate fit reads, and
// whether the attribute applies to it (a leaf carries taste, texture, colour
// and fresh only).
type field func(c platefit.Component) (applies, knows bool)

var fields = []field{
	func(c platefit.Component) (bool, bool) { return true, c.Attrs.Taste != nil },
	func(c platefit.Component) (bool, bool) { return true, c.Attrs.Texture != "" },
	func(c platefit.Component) (bool, bool) { return true, c.Attrs.Colour != "" },
	func(c platefit.Component) (bool, bool) { return true, c.Fresh != nil },
	func(c platefit.Component) (bool, bool) { return !c.Leaf, c.Attrs.Temperature != "" },
	func(c platefit.Component) (bool, bool) { return !c.Leaf, c.Attrs.Cuisine != "" },
	func(c platefit.Component) (bool, bool) { return !c.Leaf, c.Attrs.Richness != nil },
	func(c platefit.Component) (bool, bool) {
		return !c.Leaf, c.Starch != nil || c.Role == food.RoleSideStarch
	},
}

// known reports whether cand, added to p (replacing side skip when skip >= 0),
// knows every attribute that all of the plate's other components know: an
// unknown value turns a proven gap into no judgment, which would look like a
// settled gap without being one.
func known(p platefit.Plate, skip int, cand platefit.Component) bool {
	others := []platefit.Component{p.Main}
	for i, s := range p.Sides {
		if i != skip {
			others = append(others, s)
		}
	}
	for _, f := range fields {
		applies, knows := f(cand)
		if !applies || knows {
			continue
		}
		all := true
		for _, o := range others {
			if a, k := f(o); a && !k {
				all = false
				break
			}
		}
		if all {
			return false
		}
	}
	return true
}

// Garnish kinds by name (German and English), from culinary principle: a
// pickle is pickled; a cool element is cucumber or a cultured dairy dip (raw
// cucumber cools, it is not a herb); a finishing acid is citrus or vinegar;
// crunch is a nut, seed or crisp topping; herbs are fresh soft herbs and the
// green toppings used like them (spring onion, chives, cress). A leaf of none
// of these is no garnish tip: silence beats noise. notGarnishRe are the
// cooking forms of those foods (an oil, a butter, a ground nut, a dried herb,
// a spice seed, coconut milk), never scattered on a plate.
var (
	notGarnishRe = regexp.MustCompile(`(?i)\böl\b|öl$|oil\b|butter|mus\b|milk|milch|gemahlen|ground|getrocknet|dried|pesto|paste|nutmeg|muskat|caraway|kümmel|cumin|fennel|fenchel|mustard|senf|pulver|powder`)
	pickleRe     = regexp.MustCompile(`(?i)eingelegt|pickle|gewürzgurke|cornichon|sauerkraut|kimchi`)
	coolRe       = regexp.MustCompile(`(?i)gurke|cucumber|joghurt|yogh?urt|raita|tzatziki|quark|schmand|saure sahne|sour cream|crème fraîche|creme fraiche`)
	acidRe       = regexp.MustCompile(`(?i)zitrone|lemon|limette|lime|essig|vinegar`)
	crunchRe     = regexp.MustCompile(`(?i)nuss|nüsse|nut|mandel|almond|pinienkern|pine nut|cashew|pistazie|pistachio|sesam|kerne|seed|crouton|röstzwiebel|fried onion|semmelbrösel|breadcrumb|panko`)
	herbsRe      = regexp.MustCompile(`(?i)petersilie|parsley|schnittlauch|chive|dill|koriander|coriander|cilantro|basilikum|basil|minze|mint|kerbel|chervil|estragon|tarragon|kresse|cress|frühlingszwiebel|spring onion|scallion`)
)

// garnishKind is the one kind a garnish leaf can be, from its name (the
// first match in the order above); any other leaf is no tip.
func garnishKind(g platefit.Component) Kind {
	switch {
	case notGarnishRe.MatchString(g.Name):
		return ""
	case pickleRe.MatchString(g.Name):
		return KindPickle
	case coolRe.MatchString(g.Name):
		return KindCool
	case acidRe.MatchString(g.Name):
		return KindAcid
	case crunchRe.MatchString(g.Name):
		return KindCrunch
	case herbsRe.MatchString(g.Name):
		return KindHerbs
	}
	return ""
}

// swapKinds are the kinds a swap to side s can be, most specific first.
func swapKinds(s platefit.Component) []Kind {
	ks := []Kind{KindCuisineFix}
	if s.Attrs.Temperature == "cold" {
		ks = append(ks, KindCool)
	}
	if s.Role == food.RoleSauce {
		ks = append(ks, KindSauce)
	}
	return append(ks, KindSideSwap)
}
