package flavor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
)

func ptr[T any](v T) *T { return &v }

// curry is a tomato and cream curry: chicken 150 g, tomatoes 100 g, cream
// 50 g, onion 40 g per portion; 400 kcal, 20 g fat (45% fat energy).
func curry() flavor.Input {
	return flavor.Input{
		Lines: []flavor.Line{
			{Name: "Hähnchenbrust", Grams: 150},
			{Name: "Tomaten", Grams: 100},
			{Name: "Sahne", Grams: 50},
			{Name: "Zwiebeln", Grams: 40},
		},
		Kcal: 400, Fat: 20, Tags: []string{"Indisch", "schnell"},
	}
}

func TestDerive_Recipe(t *testing.T) {
	d := flavor.Derive(curry())
	assert.Equal(t, []string{"Hähnchenbrust", "Tomaten"}, d.Heroes)
	assert.Equal(t, ptr(2), d.Richness)
	assert.Equal(t, ptr(true), d.Saucy)
	assert.Equal(t, "orange", d.Colour)
	assert.Equal(t, "indian", d.Cuisine)
	assert.Empty(t, d.Kind, "judgment fields stay unknown")
	assert.Nil(t, d.Taste)
}

func TestDerive_Edges(t *testing.T) {
	t.Run("leaf gets only its colour", func(t *testing.T) {
		d := flavor.Derive(flavor.Input{Leaf: true, Lines: []flavor.Line{{Name: "Limette", Grams: 100}}, Kcal: 30, Fat: 0.2})
		assert.Equal(t, flavor.Attributes{Colour: "green"}, d)
	})
	t.Run("no ingredients, no energy: nothing", func(t *testing.T) {
		assert.Equal(t, flavor.Attributes{}, flavor.Derive(flavor.Input{}))
	})
	t.Run("light portion is at most richness 1", func(t *testing.T) {
		d := flavor.Derive(flavor.Input{Lines: []flavor.Line{{Name: "Kopfsalat", Grams: 100}, {Name: "Olivenöl", Grams: 10}}, Kcal: 100, Fat: 10})
		assert.Equal(t, ptr(1), d.Richness)
	})
	t.Run("raw salad tomatoes are not liquid", func(t *testing.T) {
		d := flavor.Derive(flavor.Input{Lines: []flavor.Line{{Name: "Tomaten", Grams: 100}, {Name: "Gurken", Grams: 100}}, Kcal: 40, Fat: 0.4})
		assert.Equal(t, ptr(false), d.Saucy)
	})
	t.Run("conflicting cuisine tags: unknown", func(t *testing.T) {
		in := curry()
		in.Tags = []string{"Indisch", "Thai"}
		assert.Empty(t, flavor.Derive(in).Cuisine)
	})
	t.Run("only base ingredients: no hero", func(t *testing.T) {
		d := flavor.Derive(flavor.Input{Lines: []flavor.Line{{Name: "Zwiebeln", Grams: 100}, {Name: "Olivenöl", Grams: 10}}, Kcal: 130, Fat: 10})
		assert.Nil(t, d.Heroes)
	})
	t.Run("at most three heroes", func(t *testing.T) {
		d := flavor.Derive(flavor.Input{Lines: []flavor.Line{
			{Name: "Zucchini", Grams: 100}, {Name: "Paprika", Grams: 100}, {Name: "Aubergine", Grams: 100}, {Name: "Tomaten", Grams: 100},
		}, Kcal: 100, Fat: 1})
		assert.Len(t, d.Heroes, flavor.MaxHeroes)
	})
}

func TestValidate_DropsUnsupportedValues(t *testing.T) {
	in := curry()
	got := flavor.Validate(flavor.Attributes{
		Kind:     "curry",
		Heroes:   []string{"Hähnchenbrust", "Lachs"},
		Richness: ptr(0),
		Colour:   "turquoise",
		Taste:    &flavor.Taste{Acid: 2, Heat: 2},
	}, in)
	assert.Equal(t, "curry", got.Kind)
	assert.Equal(t, []string{"Hähnchenbrust"}, got.Heroes, "a hero must be an ingredient")
	assert.Nil(t, got.Richness, "0 is two steps from a 45% fat share")
	assert.Empty(t, got.Colour, "outside the vocabulary")
	assert.Equal(t, &flavor.Taste{Acid: 2, Heat: 2}, got.Taste, "tomatoes are an acid ingredient")

	noAcid := flavor.Input{Lines: []flavor.Line{{Name: "Reis", Grams: 100}}, Kcal: 130, Fat: 0.3}
	got = flavor.Validate(flavor.Attributes{Taste: &flavor.Taste{Acid: 2, Salt: 1}, Heroes: []string{"Lachs"}}, noAcid)
	assert.Equal(t, &flavor.Taste{Salt: 1}, got.Taste, "acid needs an acid ingredient")
	assert.Nil(t, got.Heroes)
}

func TestResolve_UserWinsThenAutoThenRule(t *testing.T) {
	in := curry()
	rec := flavor.Record{
		Version: flavor.Version,
		Values: flavor.Attributes{
			Colour: "red", Kind: "curry", Heroes: []string{"Lachs"}, Cuisine: "thai",
		},
		Sources: map[flavor.Field]flavor.Source{
			flavor.FieldColour:  flavor.SourceUser,
			flavor.FieldKind:    flavor.SourceAuto,
			flavor.FieldHeroes:  flavor.SourceAuto, // invalid: falls back to the rule value
			flavor.FieldCuisine: flavor.SourceUser,
		},
	}
	got := flavor.Resolve(in, rec)
	assert.Equal(t, "red", got.Values.Colour)
	assert.Equal(t, "thai", got.Values.Cuisine, "a user value beats the tag")
	assert.Equal(t, "curry", got.Values.Kind)
	assert.Equal(t, []string{"Hähnchenbrust", "Tomaten"}, got.Values.Heroes)
	assert.Equal(t, map[flavor.Field]flavor.Source{
		flavor.FieldColour: flavor.SourceUser, flavor.FieldCuisine: flavor.SourceUser,
		flavor.FieldKind: flavor.SourceAuto, flavor.FieldHeroes: flavor.SourceRule,
		flavor.FieldRichness: flavor.SourceRule, flavor.FieldSaucy: flavor.SourceRule,
	}, got.Sources)
}

func TestResolve_LeafCarriesItsSubset(t *testing.T) {
	leaf := flavor.Input{Leaf: true, Lines: []flavor.Line{{Name: "Zitrone", Grams: 100}}}
	rec := flavor.Record{
		Values:  flavor.Attributes{Kind: "salad", Texture: "soft"},
		Sources: map[flavor.Field]flavor.Source{flavor.FieldKind: flavor.SourceUser, flavor.FieldTexture: flavor.SourceUser},
	}
	got := flavor.Resolve(leaf, rec)
	assert.Equal(t, flavor.Attributes{Texture: "soft", Colour: "beige"}, got.Values)
}

func TestResolve_UserHeroesFollowIngredients(t *testing.T) {
	user := func(heroes ...string) flavor.Record {
		return flavor.Record{
			Values:  flavor.Attributes{Heroes: heroes, Colour: "red"},
			Sources: map[flavor.Field]flavor.Source{flavor.FieldHeroes: flavor.SourceUser, flavor.FieldColour: flavor.SourceUser},
		}
	}
	t.Run("a removed ingredient is no longer a hero", func(t *testing.T) {
		got := flavor.Resolve(curry(), user("Sahne", "Lachs"))
		assert.Equal(t, []string{"Sahne"}, got.Values.Heroes)
		assert.Equal(t, flavor.SourceUser, got.Sources[flavor.FieldHeroes])
		assert.Equal(t, "red", got.Values.Colour, "the user still wins for every other field")
	})
	t.Run("no user hero left: the rule value", func(t *testing.T) {
		got := flavor.Resolve(curry(), user("Lachs"))
		assert.Equal(t, []string{"Hähnchenbrust", "Tomaten"}, got.Values.Heroes)
		assert.Equal(t, flavor.SourceRule, got.Sources[flavor.FieldHeroes])
	})
}
