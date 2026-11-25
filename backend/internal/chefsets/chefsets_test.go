package chefsets

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// plate reading predicates: the mechanical form of each PLATE-1 dimension, used
// to prove a plate's labels agree with the flaws it is tagged with. They are
// the set authors' convention, not the plate-fit engine (A4a owns that).

var contrastCuisines = []string{"indian", "chinese", "japanese", "thai", "mexican", "middle_eastern"}

// westMed is one coherence group: Italian, Mediterranean and French sides mix.
var westMed = []string{"italian", "mediterranean", "french"}

type plateView struct {
	foods []Food
}

func (s Set) view(p Plate) plateView {
	var v plateView
	for _, c := range p.Components {
		v.foods = append(v.foods, s.Foods[c])
	}
	return v
}

func (v plateView) any(f func(Food) bool) bool { return slices.ContainsFunc(v.foods, f) }

func (v plateView) count(f func(Food) bool) int {
	n := 0
	for _, x := range v.foods {
		if f(x) {
			n++
		}
	}
	return n
}

func (v plateView) main() Food { return v.foods[0] }

func heavy(f Food) bool { return f.Role == "main" || f.Role == "side_starch" }

// reading returns, per dimension, whether the labels show the flaw.
func (v plateView) reading() map[string]bool {
	maxRich, maxHeat, maxAcid := 0, 0, 0
	for _, f := range v.foods {
		maxRich = max(maxRich, f.Attrs.Richness)
		maxHeat = max(maxHeat, f.Attrs.Taste.Heat)
		maxAcid = max(maxAcid, f.Attrs.Taste.Acid)
	}
	cold := v.any(func(f Food) bool { return f.Attrs.Temperature == "cold" })
	cuisines := map[string]bool{}
	for _, f := range v.foods {
		if c := f.Attrs.Cuisine; c != "" && c != "neutral" {
			if slices.Contains(westMed, c) {
				c = "west_med"
			}
			cuisines[c] = true
		}
	}
	richHeavy, richSum := 0, 0
	for _, f := range v.foods {
		if heavy(f) && f.Attrs.Richness >= 2 {
			richHeavy++
			richSum += f.Attrs.Richness
		}
	}
	m := v.main().Attrs
	return map[string]bool{
		"taste_balance": maxRich >= 2 && maxAcid == 0,
		"fresh":         !v.any(func(f Food) bool { return f.Attrs.Fresh }),
		"texture":       !v.any(func(f Food) bool { return f.Attrs.Texture == "crisp" || f.Attrs.Texture == "crunchy" }),
		"colour": !v.any(func(f Food) bool {
			return !slices.Contains([]string{"beige", "brown", "white"}, f.Attrs.Colour)
		}),
		"temperature":  maxHeat >= 2 && !cold,
		"cuisine":      len(cuisines) > 1,
		"pairing":      slices.Contains(contrastCuisines, m.Cuisine) && !cold && maxAcid < 2,
		"sauce_starch": m.Saucy && !v.any(func(f Food) bool { return f.Attrs.HasStarch }),
		"proportions": v.count(func(f Food) bool { return f.Attrs.HasStarch }) >= 2 ||
			v.count(func(f Food) bool { return f.Role == "main" }) >= 2,
		"richness": richHeavy >= 2 && richSum >= 5,
	}
}

func checkSet(t *testing.T, s Set) {
	t.Helper()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	clashAndStrong := []string{"cuisine", "proportions", "richness", "sauce_starch", "taste_balance", "pairing"}
	for _, p := range s.Plates {
		r := s.view(p).reading()
		switch p.Kind {
		case Flawed:
			for _, fl := range p.Flaws {
				if !r[fl.Dimension] {
					t.Errorf("%s: tagged %s but its labels do not show it", p.ID, fl.Dimension)
				}
			}
		case Classic:
			for _, d := range clashAndStrong {
				if r[d] {
					t.Errorf("%s: a classic plate shows %s", p.ID, d)
				}
			}
		case Good:
			for _, d := range Dimensions {
				if r[d] {
					t.Errorf("%s: a good plate shows %s", p.ID, d)
				}
			}
		}
	}
}

func coverage(t *testing.T, s Set, minPlates int) {
	t.Helper()
	dims := map[string]int{}
	classic := map[string]int{}
	kinds := map[string]int{}
	for _, p := range s.Plates {
		kinds[p.Kind]++
		if p.Kind == Classic {
			classic[p.Cuisine]++
		}
		for _, f := range p.Flaws {
			dims[f.Dimension]++
		}
	}
	for _, d := range Dimensions {
		if dims[d] == 0 {
			t.Errorf("no flawed plate for dimension %s", d)
		}
	}
	for _, c := range Cuisines {
		if c != "neutral" && classic[c] == 0 {
			t.Errorf("no classic plate for cuisine %s", c)
		}
	}
	if n := len(s.Plates); n < minPlates {
		t.Errorf("%d plates, want at least %d", n, minPlates)
	}
	for _, k := range []string{Good, Flawed, Classic} {
		if kinds[k] == 0 {
			t.Errorf("no %s plates", k)
		}
	}
}

func TestDevSet(t *testing.T) {
	s, err := LoadDev()
	if err != nil {
		t.Fatal(err)
	}
	checkSet(t, s)
	coverage(t, s, 40)
}

// TestSealedSet runs the sealed split (dev foods plus the sealed directory) when
// PLANTRY_CHEF_SETS_SEALED is set; the two splits hold at least 100 recipes
// (dev grew after the sealed split was cut).
func TestSealedSet(t *testing.T) {
	s, ok, err := LoadSealed()
	if !ok {
		t.Skipf("%s not set", SealedDirEnv)
	}
	if err != nil {
		t.Fatal(err)
	}
	checkSet(t, s)
	checkRichness(t, s)
	coverage(t, s, 80)
	n := 0
	for _, f := range s.Foods {
		if !f.Leaf {
			n++
		}
	}
	if n < 100 {
		t.Errorf("%d labelled recipes across both splits, want at least 100", n)
	}
}

func TestFixtureSize(t *testing.T) {
	s, _ := LoadDev()
	n := 0
	for _, f := range s.Foods {
		if !f.Leaf {
			n++
		}
	}
	if n < 60 {
		t.Errorf("dev split has %d recipes, want at least 60", n)
	}
}

// TestRichnessFollowsFatShare keeps the richness labels within one step of the
// fat share of energy (ATTR-3), so A1's derivation can be compared with them. A
// dish under 150 kcal a portion (a dressed salad, roast vegetables) is light
// whatever its fat share, so its band is capped at 1.
func TestRichnessFollowsFatShare(t *testing.T) {
	s, _ := LoadDev()
	checkRichness(t, s)
}

func checkRichness(t *testing.T, s Set) {
	t.Helper()
	var report []string
	for _, f := range s.Foods {
		if f.Leaf {
			continue
		}
		var kcal, fat float64
		for _, ing := range f.Ingredients {
			in := s.Ingredients[ing.Name]
			kcal += in.Kcal * ing.Grams / 100
			fat += in.Fat * ing.Grams / 100
		}
		share := fat * 9 / kcal
		band := 0
		switch {
		case share >= 0.5:
			band = 3
		case share >= 0.35:
			band = 2
		case share >= 0.2:
			band = 1
		}
		if kcal/f.Portions < 150 {
			band = min(band, 1)
		}
		if d := band - f.Attrs.Richness; d > 1 || d < -1 {
			report = append(report, fmt.Sprintf("%s: label %d, fat share %.0f%% (band %d)", f.Name, f.Attrs.Richness, share*100, band))
		}
	}
	if len(report) > 0 {
		slices.Sort(report)
		t.Errorf("richness off by more than one band:\n%s", strings.Join(report, "\n"))
	}
}

func TestValidateRejects(t *testing.T) {
	dev, err := LoadDev()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(s *Set){
		"hero not an ingredient": func(s *Set) {
			f := s.Foods["Shakshuka"]
			f.Attrs.Heroes = []string{"Zimt"}
			s.Foods["Shakshuka"] = f
		},
		"unknown ingredient": func(s *Set) {
			f := s.Foods["Kachumber"]
			f.Ingredients = append(slices.Clone(f.Ingredients), f.Ingredients[0])
			f.Ingredients[len(f.Ingredients)-1].Name = "Zimt"
			s.Foods["Kachumber"] = f
		},
		"plate with unknown food": func(s *Set) {
			s.Plates = append(slices.Clone(s.Plates), Plate{ID: "x", Kind: Classic, Cuisine: "german", Components: []string{"Frikadellen", "Nichts"}})
		},
		"cuisine outside the PLATE-3 list": func(s *Set) {
			s.Plates = slices.Clone(s.Plates)
			s.Plates[0].Cuisine = "korean"
		},
		"flaw without a dimension": func(s *Set) {
			s.Plates = slices.Clone(s.Plates)
			for i, p := range s.Plates {
				if p.Kind == Flawed {
					s.Plates[i].Flaws = []Flaw{{Dimension: "vibes", Type: "gap", Expect: "x"}}
					return
				}
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := Set{Ingredients: dev.Ingredients, Foods: map[string]Food{}, Plates: dev.Plates}
			for k, v := range dev.Foods {
				s.Foods[k] = v
			}
			mutate(&s)
			if s.Validate() == nil {
				t.Fatal("Validate accepted it")
			}
		})
	}
}

// TestFieldClasses keeps the derivable/judgment split complete: every attribute
// is in exactly one class.
func TestFieldClasses(t *testing.T) {
	var tags []string
	rt := reflect.TypeOf(Attrs{})
	for i := range rt.NumField() {
		tags = append(tags, strings.Split(rt.Field(i).Tag.Get("json"), ",")[0])
	}
	want := slices.Concat(DerivableFields, JudgmentFields)
	slices.Sort(tags)
	slices.Sort(want)
	if !slices.Equal(tags, want) {
		t.Errorf("attribute fields %v, classified %v", tags, want)
	}
}
