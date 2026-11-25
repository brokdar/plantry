package platefit_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

func TestSuggest(t *testing.T) {
	s, err := devSet()
	require.NoError(t, err)
	var library []platefit.Component
	for _, f := range s.Foods {
		library = append(library, component(f))
	}
	names := func(ss []platefit.Suggestion) []string {
		var out []string
		for _, x := range ss {
			out = append(out, x.Side.Name)
		}
		return out
	}

	t.Run("a dal wants something cool and fresh first, then rice", func(t *testing.T) {
		main := foods(t, "Linsen-Dal")[0]
		got := platefit.Suggest(main, library, 50)
		require.NotEmpty(t, got)
		assert.Equal(t, "Kachumber", got[0].Side.Name)
		assert.Equal(t, []platefit.Dimension{platefit.Fresh, platefit.Texture, platefit.Temperature, platefit.Pairing}, got[0].Settles)
		assert.Contains(t, names(got), "Basmatireis")
		for _, x := range got {
			r := platefit.Read(platefit.Plate{Main: main, Sides: []platefit.Component{x.Side}})
			assert.Empty(t, r.Clashes, "%s clashes", x.Side.Name)
			assert.NotEqual(t, "main", string(x.Side.Role), x.Side.Name)
			assert.False(t, x.Side.Leaf, x.Side.Name)
		}
		assert.NotContains(t, names(got), "Gurkensalat", "a German side clashes with an Indian main")
	})

	t.Run("deterministic, limited, library order does not matter", func(t *testing.T) {
		main := foods(t, "Schweinebraten")[0]
		first := platefit.Suggest(main, library, 3)
		assert.Len(t, first, 3)
		reversed := make([]platefit.Component, len(library))
		for i, c := range library {
			reversed[len(library)-1-i] = c
		}
		assert.Equal(t, first, platefit.Suggest(main, reversed, 3))
	})

	t.Run("a blank side settles nothing and never outranks one that closes the gap", func(t *testing.T) {
		main := foods(t, "Linsen-Dal")[0]
		blank := platefit.Component{ID: 9999, Name: "Aaa blank side", Role: food.RoleSideVeg}
		got := platefit.Suggest(main, append(slices.Clone(library), blank), 50)
		require.NotEmpty(t, got)
		assert.NotContains(t, names(got), blank.Name, "a side with no known values improves nothing")
		assert.Equal(t, "Kachumber", got[0].Side.Name)
	})

	t.Run("a German roast prefers German sides on a tie", func(t *testing.T) {
		got := platefit.Suggest(foods(t, "Schweinebraten")[0], library, 50)
		require.NotEmpty(t, got)
		assert.Equal(t, "Gurkensalat", got[0].Side.Name)
	})
}
