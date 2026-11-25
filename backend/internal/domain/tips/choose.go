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

// Change is a candidate tip for one plate: add the garnish Add, or replace
// the side at Swap (an index into Plate.Sides) with With.
type Change struct {
	Kind Kind
	// Dimension is the first raised dimension the change settles; Settles
	// lists every one, in dimension order.
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

// Candidates returns the changes that settle at least one dimension p raises
// and raise none it does not, best first: most dimensions settled, then a
// garnish before a swap, then kind order, then name. garnishes are leaves a
// plate can gain with no cooking; sides are recipes that can replace one of
// p's dish sides. A candidate that would hide a gap behind an unknown
// attribute is never offered (known).
func Candidates(p platefit.Plate, garnishes, sides []platefit.Component) []Change {
	raised := platefit.Read(p).Raised()
	if len(raised) == 0 {
		return nil
	}
	var out []Change
	try := func(next platefit.Plate, ch Change) {
		after := platefit.Read(next).Raised()
		for _, d := range after {
			if !slices.Contains(raised, d) {
				return
			}
		}
		for _, d := range raised {
			if !slices.Contains(after, d) {
				ch.Settles = append(ch.Settles, d)
			}
		}
		if len(ch.Settles) == 0 {
			return
		}
		ch.Dimension = ch.Settles[0]
		ch.Kind = kindOf(ch)
		out = append(out, ch)
	}
	for _, g := range garnishes {
		if !g.Leaf || onPlate(p, g.ID) || !known(p, -1, g) {
			continue
		}
		next := platefit.Plate{Main: p.Main, Sides: append(slices.Clone(p.Sides), g)}
		try(next, Change{Add: &g})
	}
	for i, old := range p.Sides {
		if old.Leaf {
			continue
		}
		for _, s := range sides {
			if s.Leaf || !slices.Contains(sideRoles, s.Role) || onPlate(p, s.ID) || !known(p, i, s) {
				continue
			}
			next := platefit.Plate{Main: p.Main, Sides: slices.Clone(p.Sides)}
			next.Sides[i] = s
			try(next, Change{Swap: i, With: s})
		}
	}
	slices.SortStableFunc(out, func(a, b Change) int {
		if c := cmp.Compare(len(b.Settles), len(a.Settles)); c != 0 {
			return c
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

// pickleRe names pickled garnishes (German and English).
var pickleRe = regexp.MustCompile(`(?i)eingelegt|pickle|gewürzgurke|cornichon|sauerkraut|kimchi`)

// kindOf names a change by what it settles first, then by the food it
// brings.
func kindOf(ch Change) Kind {
	settles := func(ds ...platefit.Dimension) bool {
		return slices.ContainsFunc(ds, func(d platefit.Dimension) bool { return slices.Contains(ch.Settles, d) })
	}
	if ch.Add == nil {
		w := ch.With
		switch {
		case settles(platefit.CuisineFit):
			return KindCuisineFix
		case settles(platefit.Proportions, platefit.Richness, platefit.SauceStarch):
			return KindSideSwap
		case w.Attrs.Temperature == "cold" && settles(platefit.Temperature, platefit.Pairing):
			return KindCool
		case w.Role == food.RoleSauce:
			return KindSauce
		}
		return KindSideSwap
	}
	g := *ch.Add
	acid := g.Attrs.Taste != nil && g.Attrs.Taste.Acid >= 2
	fresh := g.Fresh != nil && *g.Fresh
	switch {
	case pickleRe.MatchString(g.Name):
		return KindPickle
	case acid && settles(platefit.TasteBalance, platefit.Temperature, platefit.Pairing):
		return KindAcid
	case (g.Attrs.Texture == "crisp" || g.Attrs.Texture == "crunchy") && settles(platefit.Texture):
		return KindCrunch
	case fresh && settles(platefit.Fresh, platefit.Colour):
		return KindHerbs
	case acid:
		return KindAcid
	case settles(platefit.Texture):
		return KindCrunch
	}
	return KindHerbs
}
