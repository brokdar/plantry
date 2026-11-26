package flavor_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/eval"
)

// inputOf builds a derivation input from a labelled food: lines and energy
// per portion, as InputOf does from resolved nutrition.
func inputOf(s chefsets.Set, f chefsets.Food) flavor.Input {
	portions := f.Portions
	if portions <= 0 || f.Leaf {
		portions = 1
	}
	in := flavor.Input{Leaf: f.Leaf, Tags: f.Tags}
	for _, ing := range f.Ingredients {
		ref := s.Ingredients[ing.Name]
		g := ing.Grams / portions
		in.Lines = append(in.Lines, flavor.Line{Name: ing.Name, Grams: g})
		in.Kcal += ref.Kcal * g / 100
		in.Fat += ref.Fat * g / 100
	}
	return in
}

// Tolerance of the rule derivation against the DATA-1 dev labels, per
// derivable field (share of the foods compared that must agree):
//   - heroes: every derived hero is an ingredient (always), and the first
//     derived hero is one of the labelled heroes;
//   - richness: within one step of the label (always); exact for half (the
//     labels lean to 2 where the fat share reads 1 or 3);
//   - saucy, colour: equal to the label;
//   - cuisine: equal to the label wherever a cuisine tag exists (always).
var tolerance = map[string]float64{
	"heroes":         0.85,
	"richness":       1.0,
	"richness_exact": 0.5,
	"saucy":          0.85,
	"colour":         0.7,
	"cuisine":        1.0,
}

func TestDerive_MatchesDevLabels(t *testing.T) {
	s, err := chefsets.LoadDev()
	require.NoError(t, err)
	agree, total := map[string]int{}, map[string]int{}
	var misses []string
	count := func(field string, ok bool, name string, got, want any) {
		total[field]++
		if ok {
			agree[field]++
		} else {
			misses = append(misses, fmt.Sprintf("%s %s: got %v, label %v", field, name, got, want))
		}
	}
	for name, f := range s.Foods {
		in := inputOf(s, f)
		d := flavor.Derive(in)
		count("colour", d.Colour == f.Attrs.Colour, name, d.Colour, f.Attrs.Colour)
		if f.Leaf {
			continue
		}
		for _, h := range d.Heroes {
			require.True(t, slices.ContainsFunc(f.Ingredients, func(i eval.RecipeIngredient) bool { return i.Name == h }), "%s: hero %q is not an ingredient", name, h)
		}
		count("heroes", len(d.Heroes) > 0 && slices.Contains(f.Attrs.Heroes, d.Heroes[0]), name, d.Heroes, f.Attrs.Heroes)
		require.NotNil(t, d.Richness, name)
		diff := *d.Richness - f.Attrs.Richness
		count("richness", diff >= -1 && diff <= 1, name, *d.Richness, f.Attrs.Richness)
		count("richness_exact", diff == 0, name, *d.Richness, f.Attrs.Richness)
		require.NotNil(t, d.Saucy, name)
		count("saucy", *d.Saucy == f.Attrs.Saucy, name, *d.Saucy, f.Attrs.Saucy)
		if d.Cuisine != "" || len(f.Tags) > 0 {
			count("cuisine", d.Cuisine == f.Attrs.Cuisine, name, d.Cuisine, f.Attrs.Cuisine)
		}
	}
	slices.Sort(misses)
	t.Logf("misses:\n%s", strings.Join(misses, "\n"))
	for field, min := range tolerance {
		require.Positive(t, total[field], field)
		share := float64(agree[field]) / float64(total[field])
		t.Logf("%s: %d/%d (%.0f%%)", field, agree[field], total[field], share*100)
		assert.GreaterOrEqual(t, share, min, "%s agreement %.2f under tolerance %.2f", field, share, min)
	}
}

func TestDerive_RawSaladColourIgnoresDressing(t *testing.T) {
	t.Parallel()
	in := flavor.Input{Lines: []flavor.Line{{Name: "Cucumber", Grams: 80}, {Name: "Greek yogurt", Grams: 120}, {Name: "Dill", Grams: 2}}}
	assert.Equal(t, "green", flavor.Derive(in).Colour)
	in.Lines = in.Lines[1:2]
	assert.Equal(t, "white", flavor.Derive(in).Colour, "yogurt without a raw vegetable stays white")
}
