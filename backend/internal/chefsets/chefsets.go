// Package chefsets holds the hand-labelled test sets of the Chef feature:
// recipe attributes for a 100-recipe fixture and good, flawed and classic
// plates (see README.md). It has no production caller; tests of plate fit and
// attribute derivation load it.
package chefsets

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/eval"
)

//go:embed dev/*.json
var devFS embed.FS

// SealedDirEnv names the directory of the sealed split. The orchestrator
// sets it; the sealed split is never committed.
const SealedDirEnv = "PLANTRY_CHEF_SETS_SEALED"

// Fixed vocabularies. Cuisines are the PLATE-3 list plus the neutral fallback.
var (
	Cuisines = []string{
		"german", "italian", "french", "mediterranean", "indian",
		"chinese", "japanese", "thai", "mexican", "middle_eastern", "neutral",
	}
	Kinds = []string{
		"stew", "curry", "pasta", "roast", "pan", "bake", "salad", "soup",
		"bowl", "grain", "veg", "egg", "breakfast", "sauce", "pickle", "bread", "fish", "wrap",
	}
	Textures     = []string{"soft", "creamy", "chewy", "crisp", "crunchy"}
	Temperatures = []string{"hot", "warm", "cold"}
	Colours      = []string{"green", "red", "orange", "beige", "brown", "white", "purple", "mixed"}
	Techniques   = []string{"braise", "roast", "fry", "saute", "grill", "boil", "bake", "raw", "steam", "simmer"}
	Formats      = []string{"plate", "bowl", "one_pot", "casserole", "wrap", "soup", "salad"}
	// Roles are the plate roles of the planner plus garnish for leaves.
	Roles = []string{"main", "side_starch", "side_veg", "side_protein", "sauce", "standalone", "garnish"}
	// Dimensions are the ten PLATE-1 dimensions.
	Dimensions = []string{
		"taste_balance", "fresh", "texture", "colour", "temperature",
		"cuisine", "pairing", "sauce_starch", "proportions", "richness",
	}
	// DerivableFields are attributes ATTR-4 computes from data; the rest need
	// judgment (cuisine is derivable only for a food that carries a cuisine
	// tag, see Food.Tags).
	DerivableFields = []string{"heroes", "richness", "saucy", "colour", "fresh", "has_starch"}
	JudgmentFields  = []string{"cuisine", "kind", "texture", "temperature", "taste", "technique", "format"}
)

// Taste is the 0-3 taste profile; fat is covered by Richness.
type Taste struct {
	Salt   int `json:"salt,omitempty"`
	Acid   int `json:"acid,omitempty"`
	Sweet  int `json:"sweet,omitempty"`
	Bitter int `json:"bitter,omitempty"`
	Umami  int `json:"umami,omitempty"`
	Heat   int `json:"heat,omitempty"`
}

// Attrs are the recipe attributes of spec section 3. A leaf (garnish) carries
// only Taste, Texture, Colour and Fresh.
type Attrs struct {
	Cuisine     string   `json:"cuisine"`
	Kind        string   `json:"kind"`
	Heroes      []string `json:"heroes"`
	Texture     string   `json:"texture"`
	Temperature string   `json:"temperature"`
	Saucy       bool     `json:"saucy"`
	Richness    int      `json:"richness"`
	Colour      string   `json:"colour"`
	Taste       Taste    `json:"taste"`
	Fresh       bool     `json:"fresh"`
	HasStarch   bool     `json:"has_starch"`
	Technique   string   `json:"technique"`
	Format      string   `json:"format"`
}

// Food is a labelled recipe or leaf.
type Food struct {
	eval.Recipe
	Attrs Attrs `json:"attrs"`
	Leaf  bool  `json:"leaf"`
}

// Label is the attribute set of a recipe of the built-in eval library.
type Label struct {
	Name  string `json:"name"`
	Attrs Attrs  `json:"attrs"`
}

// Flaw is one expected reading of a flawed plate.
type Flaw struct {
	Dimension string `json:"dimension"`
	Type      string `json:"type"` // gap or clash
	Expect    string `json:"expect"`
}

// Plate kinds.
const (
	Good    = "good"
	Flawed  = "flawed"
	Classic = "classic"
)

// Plate is a main (first component) with its sides and finishes.
type Plate struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Cuisine    string   `json:"cuisine"`
	Components []string `json:"components"`
	Note       string   `json:"note"`
	Flaws      []Flaw   `json:"flaws"`
}

type dataFile struct {
	Ingredients []eval.Ingredient `json:"ingredients"`
	Labels      []Label           `json:"labels"`
	Foods       []Food            `json:"foods"`
	Leaves      []Food            `json:"leaves"`
}

type plateFile struct {
	Plates []Plate `json:"plates"`
}

// Set is a loaded split with the eval library's ingredients and recipes.
type Set struct {
	Ingredients map[string]eval.Ingredient
	Foods       map[string]Food
	Plates      []Plate
}

// LoadDev loads the committed dev split.
func LoadDev() (Set, error) {
	sub, err := fs.Sub(devFS, "dev")
	if err != nil {
		return Set{}, err
	}
	return Load(sub)
}

// LoadSealed loads the dev split plus the sealed split in the directory named
// by SealedDirEnv; ok is false when the variable is unset. Sealed plates may
// use dev foods.
func LoadSealed() (set Set, ok bool, err error) {
	dir := os.Getenv(SealedDirEnv)
	if dir == "" {
		return Set{}, false, nil
	}
	dev, err := LoadDev()
	if err != nil {
		return Set{}, true, err
	}
	sealed, err := Load(os.DirFS(dir))
	if err != nil {
		return Set{}, true, fmt.Errorf("sealed %s: %w", dir, err)
	}
	merged, err := Merge(dev, sealed)
	return merged, true, err
}

// Load reads data.json and plates.json from fsys and the eval library.
func Load(fsys fs.FS) (Set, error) {
	lib, err := eval.LoadLibrary()
	if err != nil {
		return Set{}, err
	}
	var df dataFile
	if err := decode(fsys, "data.json", &df); err != nil {
		return Set{}, err
	}
	var pf plateFile
	if err := decode(fsys, "plates.json", &pf); err != nil {
		return Set{}, err
	}
	s := Set{Ingredients: map[string]eval.Ingredient{}, Foods: map[string]Food{}, Plates: pf.Plates}
	for _, in := range lib.Ingredients {
		s.Ingredients[in.Name] = in
	}
	for _, in := range df.Ingredients {
		if _, dup := s.Ingredients[in.Name]; dup {
			return Set{}, fmt.Errorf("duplicate ingredient %q", in.Name)
		}
		s.Ingredients[in.Name] = in
	}
	base := map[string]eval.Recipe{}
	for _, r := range lib.Recipes {
		base[r.Name] = r
	}
	for _, l := range df.Labels {
		r, ok := base[l.Name]
		if !ok {
			return Set{}, fmt.Errorf("label for %q: not in the eval library", l.Name)
		}
		s.Foods[l.Name] = Food{Recipe: r, Attrs: l.Attrs}
	}
	for _, f := range slices.Concat(df.Foods, df.Leaves) {
		if _, dup := s.Foods[f.Name]; dup {
			return Set{}, fmt.Errorf("duplicate food %q", f.Name)
		}
		s.Foods[f.Name] = f
	}
	return s, nil
}

// Merge combines splits; a name in both is an error.
func Merge(a, b Set) (Set, error) {
	out := Set{Ingredients: map[string]eval.Ingredient{}, Foods: map[string]Food{}, Plates: slices.Concat(a.Plates, b.Plates)}
	for k, v := range a.Ingredients {
		out.Ingredients[k] = v
	}
	for k, v := range b.Ingredients {
		out.Ingredients[k] = v // the eval library's ingredients appear in both
	}
	for k, v := range a.Foods {
		out.Foods[k] = v
	}
	for k, v := range b.Foods {
		if _, dup := out.Foods[k]; dup {
			return Set{}, fmt.Errorf("food %q is in both splits", k)
		}
		out.Foods[k] = v
	}
	return out, nil
}

func decode(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Validate checks internal consistency: vocabularies, heroes are ingredients,
// every referenced ingredient and food exists, plate shapes.
func (s Set) Validate() error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	in := func(v string, set []string) bool { return slices.Contains(set, v) }
	for name, f := range s.Foods {
		a := f.Attrs
		if !in(f.Role, Roles) {
			bad("%s: role %q", name, f.Role)
		}
		if f.Leaf != (f.Role == "garnish") {
			bad("%s: leaf and garnish role must agree", name)
		}
		names := map[string]bool{}
		for _, ing := range f.Ingredients {
			if _, ok := s.Ingredients[ing.Name]; !ok {
				bad("%s: unknown ingredient %q", name, ing.Name)
			}
			names[ing.Name] = true
		}
		if a.Texture != "" && !in(a.Texture, Textures) || a.Colour != "" && !in(a.Colour, Colours) {
			bad("%s: texture %q or colour %q", name, a.Texture, a.Colour)
		}
		for _, v := range []int{a.Taste.Salt, a.Taste.Acid, a.Taste.Sweet, a.Taste.Bitter, a.Taste.Umami, a.Taste.Heat, a.Richness} {
			if v < 0 || v > 3 {
				bad("%s: scale value %d outside 0-3", name, v)
			}
		}
		if f.Leaf {
			if a.Texture == "" || a.Colour == "" {
				bad("%s: a leaf needs texture and colour", name)
			}
			continue
		}
		if !in(a.Cuisine, Cuisines) || !in(a.Kind, Kinds) || !in(a.Temperature, Temperatures) ||
			!in(a.Technique, Techniques) || !in(a.Format, Formats) || !in(a.Texture, Textures) || !in(a.Colour, Colours) {
			bad("%s: attribute outside its vocabulary: %+v", name, a)
		}
		if n := len(a.Heroes); n < 1 || n > 3 {
			bad("%s: %d heroes, want 1-3", name, n)
		}
		for _, h := range a.Heroes {
			if !names[h] {
				bad("%s: hero %q is not an ingredient", name, h)
			}
		}
	}
	ids := map[string]bool{}
	for _, p := range s.Plates {
		if ids[p.ID] {
			bad("duplicate plate id %q", p.ID)
		}
		ids[p.ID] = true
		if !in(p.Cuisine, Cuisines) {
			bad("%s: cuisine %q is not in the PLATE-3 list", p.ID, p.Cuisine)
		}
		if len(p.Components) < 2 && p.Kind != Flawed || len(p.Components) == 0 {
			bad("%s: too few components", p.ID)
		}
		for i, c := range p.Components {
			f, ok := s.Foods[c]
			if !ok {
				bad("%s: unknown food %q", p.ID, c)
			} else if i == 0 && f.Role != "main" {
				bad("%s: first component %q is not a main", p.ID, c)
			}
		}
		switch p.Kind {
		case Good, Classic:
			if len(p.Flaws) != 0 {
				bad("%s: a %s plate carries no flaws", p.ID, p.Kind)
			}
		case Flawed:
			if len(p.Flaws) == 0 {
				bad("%s: a flawed plate needs a flaw", p.ID)
			}
		default:
			bad("%s: kind %q", p.ID, p.Kind)
		}
		for _, fl := range p.Flaws {
			if !in(fl.Dimension, Dimensions) || !in(fl.Type, []string{"gap", "clash"}) || fl.Expect == "" {
				bad("%s: flaw %+v", p.ID, fl)
			}
		}
	}
	return errors.Join(errs...)
}
