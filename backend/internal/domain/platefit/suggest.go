package platefit

import (
	"cmp"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Suggestion is a side that fits a main and the dimensions it settles.
type Suggestion struct {
	Side    Component
	Settles []Dimension
}

// sideRoles are the roles a pairing suggestion can have.
var sideRoles = []food.Role{food.RoleSideStarch, food.RoleSideVeg, food.RoleSideProtein, food.RoleSauce}

// Suggest derives the pairing suggestions of main from library on read
// (PLATE-8; nothing is stored). A side that brings a clash is left out; the
// rest rank by how much they improve the main's gaps (a gap weighs 2, a minor
// gap 1; a gap the side opens counts against it), then sides of the main's
// food world (coherence group or palette) first, then by name. Sides that improve nothing are left out; at most n.
func Suggest(main Component, library []Component, n int) []Suggestion {
	cu := CuisineOf(main.Attrs.Cuisine)
	alone := Read(Plate{Main: main})
	type scored struct {
		Suggestion
		score  int
		bridge bool
	}
	var out []scored
	for _, side := range library {
		if side.Leaf || !slices.Contains(sideRoles, side.Role) {
			continue
		}
		with := Read(Plate{Main: main, Sides: []Component{side}})
		if len(with.Clashes) > len(alone.Clashes) {
			continue
		}
		s := scored{Suggestion: Suggestion{Side: side}, score: weight(alone) - weight(with), bridge: bridges(cu, side)}
		for _, g := range alone.Gaps {
			if !slices.ContainsFunc(with.Gaps, func(w Gap) bool { return w.Dimension == g.Dimension }) {
				s.Settles = append(s.Settles, g.Dimension)
			}
		}
		if s.score > 0 {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b scored) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		if a.bridge != b.bridge {
			if a.bridge {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Side.Name, b.Side.Name)
	})
	res := make([]Suggestion, 0, min(n, len(out)))
	for _, s := range out[:min(n, len(out))] {
		res = append(res, s.Suggestion)
	}
	return res
}

// weight sums a reading's gaps: 2 each, 1 for a minor one.
func weight(r Reading) int {
	w := 0
	for _, g := range r.Gaps {
		w += 2
		if g.Minor {
			w--
		}
	}
	return w
}
