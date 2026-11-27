// Package flavor holds a food's recipe attributes (spec §3, ATTR): cuisine,
// dish kind, heroes, texture, temperature, saucy, richness, colour family,
// taste profile, technique and format. Values come from three sources: rules
// over the ingredients (Derive, ATTR-4), the assistant (auto, checked by
// Validate, ATTR-3) and the user, whose value always wins (ATTR-5). Only
// auto and user values are stored; rule values are derived on every read,
// so they follow ingredient edits.
package flavor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Version is the attribute schema version a stored record is written under.
const Version = 1

// Source is where an attribute value came from.
type Source string

// Sources, weakest first.
const (
	SourceRule Source = "rule" // derived from the ingredients (ATTR-4)
	SourceAuto Source = "auto" // set by the assistant (ATTR-2)
	SourceUser Source = "user" // set by the user (ATTR-5)
)

// Field names one attribute.
type Field string

// Fields.
const (
	FieldCuisine     Field = "cuisine"
	FieldKind        Field = "kind"
	FieldHeroes      Field = "heroes"
	FieldTexture     Field = "texture"
	FieldTemperature Field = "temperature"
	FieldSaucy       Field = "saucy"
	FieldRichness    Field = "richness"
	FieldColour      Field = "colour"
	FieldTaste       Field = "taste"
	FieldTechnique   Field = "technique"
	FieldFormat      Field = "format"
)

// Fields are a recipe's attributes in display order.
var Fields = []Field{
	FieldCuisine, FieldKind, FieldHeroes, FieldTexture, FieldTemperature, FieldSaucy,
	FieldRichness, FieldColour, FieldTaste, FieldTechnique, FieldFormat,
}

// LeafFields are the subset a leaf (ingredient) carries (ATTR-1).
var LeafFields = []Field{FieldTaste, FieldTexture, FieldColour}

// FieldsFor returns the fields a food of the given kind carries.
func FieldsFor(leaf bool) []Field {
	if leaf {
		return LeafFields
	}
	return Fields
}

// Vocabularies. Cuisines are the PLATE-3 list plus neutral.
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
)

// MaxHeroes is the most hero ingredients a recipe names.
const MaxHeroes = 3

// Taste is the 0-3 taste profile.
type Taste struct {
	Salt   int `json:"salt"`
	Acid   int `json:"acid"`
	Sweet  int `json:"sweet"`
	Bitter int `json:"bitter"`
	Umami  int `json:"umami"`
	Heat   int `json:"heat"`
}

func (t Taste) values() []int { return []int{t.Salt, t.Acid, t.Sweet, t.Bitter, t.Umami, t.Heat} }

// Attributes are a food's attribute values. An empty string, nil slice or
// nil pointer is unknown: rules make no judgment on it.
type Attributes struct {
	Cuisine     string   `json:"cuisine,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Heroes      []string `json:"heroes,omitempty"`
	Texture     string   `json:"texture,omitempty"`
	Temperature string   `json:"temperature,omitempty"`
	Saucy       *bool    `json:"saucy,omitempty"`
	Richness    *int     `json:"richness,omitempty"`
	Colour      string   `json:"colour,omitempty"`
	Taste       *Taste   `json:"taste,omitempty"`
	Technique   string   `json:"technique,omitempty"`
	Format      string   `json:"format,omitempty"`
}

// Has reports whether f has a value.
func (a Attributes) Has(f Field) bool {
	switch f {
	case FieldCuisine:
		return a.Cuisine != ""
	case FieldKind:
		return a.Kind != ""
	case FieldHeroes:
		return len(a.Heroes) > 0
	case FieldTexture:
		return a.Texture != ""
	case FieldTemperature:
		return a.Temperature != ""
	case FieldSaucy:
		return a.Saucy != nil
	case FieldRichness:
		return a.Richness != nil
	case FieldColour:
		return a.Colour != ""
	case FieldTaste:
		return a.Taste != nil
	case FieldTechnique:
		return a.Technique != ""
	case FieldFormat:
		return a.Format != ""
	}
	return false
}

// copyField sets dst's f to src's f (unknown when src has none).
func copyField(dst *Attributes, src Attributes, f Field) {
	switch f {
	case FieldCuisine:
		dst.Cuisine = src.Cuisine
	case FieldKind:
		dst.Kind = src.Kind
	case FieldHeroes:
		dst.Heroes = slices.Clone(src.Heroes)
	case FieldTexture:
		dst.Texture = src.Texture
	case FieldTemperature:
		dst.Temperature = src.Temperature
	case FieldSaucy:
		dst.Saucy = clonePtr(src.Saucy)
	case FieldRichness:
		dst.Richness = clonePtr(src.Richness)
	case FieldColour:
		dst.Colour = src.Colour
	case FieldTaste:
		dst.Taste = clonePtr(src.Taste)
	case FieldTechnique:
		dst.Technique = src.Technique
	case FieldFormat:
		dst.Format = src.Format
	}
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// checkField reports why a's f is outside its vocabulary or range; "" when
// it is unknown or valid.
func checkField(a Attributes, f Field) string {
	in := func(v string, set []string) string {
		if v == "" || slices.Contains(set, v) {
			return ""
		}
		return fmt.Sprintf("%s %q is not one of %s", f, v, strings.Join(set, ", "))
	}
	scale := func(v int) bool { return v >= 0 && v <= 3 }
	switch f {
	case FieldCuisine:
		return in(a.Cuisine, Cuisines)
	case FieldKind:
		return in(a.Kind, Kinds)
	case FieldTexture:
		return in(a.Texture, Textures)
	case FieldTemperature:
		return in(a.Temperature, Temperatures)
	case FieldColour:
		return in(a.Colour, Colours)
	case FieldTechnique:
		return in(a.Technique, Techniques)
	case FieldFormat:
		return in(a.Format, Formats)
	case FieldHeroes:
		if len(a.Heroes) > MaxHeroes {
			return fmt.Sprintf("at most %d heroes", MaxHeroes)
		}
	case FieldRichness:
		if a.Richness != nil && !scale(*a.Richness) {
			return "richness must be 0-3"
		}
	case FieldTaste:
		if a.Taste != nil && slices.ContainsFunc(a.Taste.values(), func(v int) bool { return !scale(v) }) {
			return "taste values must be 0-3"
		}
	}
	return ""
}

// Record is a food's stored attributes: the auto and user values, each with
// its source in Sources (a field without a source is not stored).
type Record struct {
	FoodID  int64            `json:"-"`
	Version int              `json:"version"`
	Values  Attributes       `json:"values"`
	Sources map[Field]Source `json:"sources"`
}

// Empty reports whether the record stores no value.
func (r Record) Empty() bool { return len(r.Sources) == 0 }

// Resolved are a food's effective attributes and the source of each known
// value.
type Resolved struct {
	Values  Attributes       `json:"attributes"`
	Sources map[Field]Source `json:"sources"`
}

// Resolve merges the rule values derived from in with the stored record:
// a user value wins, then an auto value that passes Validate, then the rule
// value. User heroes that are no longer ingredients are dropped. Fields the
// food's kind does not carry are left out.
func Resolve(in Input, rec Record) Resolved {
	derived := Derive(in)
	auto := Validate(rec.Values, in)
	user := rec.Values
	user.Heroes = in.ingredientsOf(user.Heroes)
	out := Resolved{Sources: map[Field]Source{}}
	for _, f := range FieldsFor(in.Leaf) {
		switch {
		case rec.Sources[f] == SourceUser && user.Has(f):
			copyField(&out.Values, user, f)
			out.Sources[f] = SourceUser
		case rec.Sources[f] == SourceAuto && auto.Has(f):
			copyField(&out.Values, auto, f)
			out.Sources[f] = SourceAuto
		case derived.Has(f):
			copyField(&out.Values, derived, f)
			out.Sources[f] = SourceRule
		}
	}
	return out
}

// Validate drops the values the ingredients of in do not support (ATTR-3):
// a value outside its vocabulary, a hero that is not an ingredient, an acid
// taste without an acid ingredient, a richness more than one step from the
// fat share of energy. Nothing is invented.
func Validate(a Attributes, in Input) Attributes {
	var out Attributes
	for _, f := range Fields {
		if checkField(a, f) == "" {
			copyField(&out, a, f)
		}
	}
	out.Heroes = in.ingredientsOf(out.Heroes)
	if out.Taste != nil && out.Taste.Acid > 0 && !in.any(isAcid) {
		t := *out.Taste
		t.Acid = 0
		out.Taste = &t
	}
	if out.Richness != nil {
		if band, ok := richnessBand(in); ok && (*out.Richness-band > 1 || band-*out.Richness > 1) {
			out.Richness = nil
		}
	}
	return out
}

// Patch is a user edit: Set's values for the fields in Fields, and the
// fields in Clear reset to their automatic value.
type Patch struct {
	Set    Attributes
	Fields []Field
	Clear  []Field
}

// check validates p for a food with input in: known fields of its kind,
// values in their vocabularies, heroes among its ingredients.
func (p Patch) check(in Input) error {
	allowed := FieldsFor(in.Leaf)
	for _, f := range slices.Concat(p.Fields, p.Clear) {
		if !slices.Contains(allowed, f) {
			return fmt.Errorf("%w: field %q", domain.ErrInvalidInput, f)
		}
	}
	for _, f := range p.Fields {
		if !p.Set.Has(f) {
			return fmt.Errorf("%w: %s has no value", domain.ErrInvalidInput, f)
		}
		if msg := checkField(p.Set, f); msg != "" {
			return fmt.Errorf("%w: %s", domain.ErrInvalidInput, msg)
		}
	}
	if slices.Contains(p.Fields, FieldHeroes) {
		for _, h := range p.Set.Heroes {
			if !in.hasIngredient(h) {
				return fmt.Errorf("%w: hero %q is not an ingredient", domain.ErrInvalidInput, h)
			}
		}
	}
	return nil
}

// apply returns rec with p applied: set fields become user values, cleared
// fields are no longer stored.
func (p Patch) apply(rec Record) Record {
	out := Record{FoodID: rec.FoodID, Version: Version, Sources: map[Field]Source{}}
	for f, s := range rec.Sources {
		out.Sources[f] = s
		copyField(&out.Values, rec.Values, f)
	}
	for _, f := range p.Clear {
		delete(out.Sources, f)
		copyField(&out.Values, Attributes{}, f)
	}
	for _, f := range p.Fields {
		out.Sources[f] = SourceUser
		copyField(&out.Values, p.Set, f)
	}
	return out
}

// DecodePatch reads {field: value | null}; an unknown field or a value of
// the wrong type is not ok.
func DecodePatch(b []byte) (Patch, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil || raw == nil {
		return Patch{}, false
	}
	var p Patch
	set := map[string]json.RawMessage{}
	for k, v := range raw {
		f := Field(k)
		if !slices.Contains(Fields, f) {
			return Patch{}, false
		}
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			p.Clear = append(p.Clear, f)
			continue
		}
		p.Fields = append(p.Fields, f)
		set[k] = v
	}
	enc, err := json.Marshal(set)
	if err != nil {
		return Patch{}, false
	}
	dec := json.NewDecoder(bytes.NewReader(enc))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p.Set); err != nil {
		return Patch{}, false
	}
	slices.Sort(p.Fields)
	slices.Sort(p.Clear)
	return p, true
}
