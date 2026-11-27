package taste

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
)

// Blame shares (TASTE-3): a plate signal goes MainShare to the main and
// SidesShare to the sides (split evenly; all of it to the main without
// sides, to the sides without a main); within a recipe HeroShare to its hero
// ingredients (split evenly) and AttrShare each to cuisine, flavour,
// technique and format. A share of an unknown attribute is dropped, not
// moved to the others.
const (
	MainShare  = 0.7
	SidesShare = 0.3
	HeroShare  = 0.4
	AttrShare  = 0.15
	// FlavourMin is the taste value (0-3) from which a taste dimension is
	// part of a recipe's flavour.
	FlavourMin = 2
)

// Blamed is an item ref and the share of a signal it takes.
type Blamed struct {
	Ref
	Share float64
}

// Blame spreads share over the attributes of a recipe. The neutral cuisine
// is no taste and takes nothing.
func Blame(a flavor.Attributes, share float64) []Blamed {
	var out []Blamed
	add := func(l Level, key string, s float64) {
		if k := NormKey(key); k != "" {
			out = append(out, Blamed{Ref{l, k}, s})
		}
	}
	for _, h := range a.Heroes {
		add(LevelHero, h, share*HeroShare/float64(len(a.Heroes)))
	}
	if a.Cuisine != "neutral" {
		add(LevelCuisine, a.Cuisine, share*AttrShare)
	}
	if dims := flavourDims(a.Taste); len(dims) > 0 {
		for _, d := range dims {
			add(LevelFlavour, d, share*AttrShare/float64(len(dims)))
		}
	}
	add(LevelTechnique, a.Technique, share*AttrShare)
	add(LevelFormat, a.Format, share*AttrShare)
	return out
}

// flavourDims are the taste dimensions at FlavourMin or more.
func flavourDims(t *flavor.Taste) []string {
	if t == nil {
		return nil
	}
	var out []string
	for _, d := range []struct {
		name string
		v    int
	}{{"salt", t.Salt}, {"acid", t.Acid}, {"sweet", t.Sweet}, {"bitter", t.Bitter}, {"umami", t.Umami}, {"heat", t.Heat}} {
		if d.v >= FlavourMin {
			out = append(out, d.name)
		}
	}
	return out
}

// PlateShares are the shares of a plate signal for the main and for each
// of sides sides (TASTE-3).
func PlateShares(hasMain bool, sides int) (main, side float64) {
	switch {
	case sides == 0:
		return 1, 0
	case !hasMain:
		return 0, 1 / float64(sides)
	}
	return MainShare, SidesShare / float64(sides)
}

// Profile is the scores the planner reads: every item that holds in a
// window and leans one way.
type Profile struct {
	scores map[Ref]float64
}

// NewProfile builds the profile of items at now for the window starting at
// window. A learned item counts only once it leans one way (Effective), so
// evidence below the threshold never moves the planner; a window item
// outranks an always item of the same ref.
func NewProfile(items []Item, now, window time.Time) Profile {
	p := Profile{scores: map[Ref]float64{}}
	windowed := map[Ref]bool{}
	for _, it := range items {
		if !it.Active(window) || it.Effective(now) == "" {
			continue
		}
		ref := Ref{it.Level, it.Key}
		if windowed[ref] && it.Scope != ScopeWindow {
			continue
		}
		p.scores[ref] = it.Score(now)
		windowed[ref] = windowed[ref] || it.Scope == ScopeWindow
	}
	return p
}

// Empty reports whether the profile holds nothing.
func (p Profile) Empty() bool { return len(p.scores) == 0 }

// Recipe is a recipe's taste score: its items' scores weighted by their
// blame shares (-1..1; 0 without items).
func (p Profile) Recipe(a flavor.Attributes) float64 {
	if p.Empty() {
		return 0
	}
	var s float64
	for _, b := range Blame(a, 1) {
		s += b.Share * p.scores[b.Ref]
	}
	return s
}
