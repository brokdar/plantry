package platefit_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

var devSet = sync.OnceValues(chefsets.LoadDev)

// foods returns the dev-set foods of names as components.
func foods(t *testing.T, names ...string) []platefit.Component {
	t.Helper()
	s, err := devSet()
	require.NoError(t, err)
	out := make([]platefit.Component, 0, len(names))
	for _, n := range names {
		f, ok := s.Foods[n]
		require.True(t, ok, n)
		out = append(out, component(f))
	}
	return out
}

func plate(t *testing.T, names ...string) platefit.Plate {
	t.Helper()
	cs := foods(t, names...)
	return platefit.Plate{Main: cs[0], Sides: cs[1:]}
}

func gaps(r platefit.Reading) map[platefit.Dimension]bool {
	out := map[platefit.Dimension]bool{}
	for _, g := range r.Gaps {
		out[g.Dimension] = g.Minor
	}
	return out
}

func TestRead_BridgePairing(t *testing.T) {
	tests := []struct {
		name  string
		plate []string
		gap   bool
	}{
		{"German roast with only foreign-neutral sides", []string{"Schweinebraten", "Basmatireis", "Gedünsteter Brokkoli"}, true},
		{"a German side bridges", []string{"Schweinebraten", "Basmatireis", "Rotkohl"}, false},
		{"a neutral side from the palette bridges", []string{"Schweinebraten", "Salzkartoffeln", "Gedünsteter Brokkoli"}, false},
		{"a palette leaf bridges", []string{"Schweinebraten", "Basmatireis", "Frischer Dill"}, false},
		{"Italian main, side of the same coherence group", []string{"Spaghetti Bolognese", "Ofengemüse"}, false},
		{"Italian main, steamed broccoli only", []string{"Spaghetti Carbonara", "Gedünsteter Brokkoli"}, true},
		{"French main, a side from another group", []string{"Coq au Vin", "Basmatireis", "Chinesischer Gurkensalat"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := gaps(platefit.Read(plate(t, tt.plate...)))[platefit.Pairing]
			assert.Equal(t, tt.gap, got)
		})
	}
}

func TestRead_ContrastPairing(t *testing.T) {
	assert.Contains(t, gaps(platefit.Read(plate(t, "Linsen-Dal", "Basmatireis"))), platefit.Pairing)
	for _, cool := range []string{"Gurken-Raita", "Limettenspalten"} {
		assert.NotContains(t, gaps(platefit.Read(plate(t, "Linsen-Dal", "Basmatireis", cool))), platefit.Pairing, cool)
	}
}

// An unknown attribute is no judgment: a gap needs every value it reads.
func TestRead_UnknownIsNoJudgment(t *testing.T) {
	tests := []struct {
		name   string
		forget func(c *platefit.Component)
		dims   []platefit.Dimension
	}{
		{"taste", func(c *platefit.Component) { c.Attrs.Taste = nil }, []platefit.Dimension{platefit.TasteBalance}},
		{"fresh", func(c *platefit.Component) { c.Fresh = nil }, []platefit.Dimension{platefit.Fresh}},
		{"texture", func(c *platefit.Component) { c.Attrs.Texture = "" }, []platefit.Dimension{platefit.Texture}},
		{"colour", func(c *platefit.Component) { c.Attrs.Colour = "" }, []platefit.Dimension{platefit.Colour}},
		{"temperature", func(c *platefit.Component) { c.Attrs.Temperature = "" }, []platefit.Dimension{platefit.Temperature}},
		{"starch", func(c *platefit.Component) { c.Starch = nil }, []platefit.Dimension{platefit.SauceStarch}},
	}
	// Each forgotten value belongs to the side; the plate shows every listed
	// gap while it is known.
	base := map[string][]string{
		"taste":       {"Spaghetti Carbonara", "Gedünsteter Brokkoli"},
		"fresh":       {"Linsen-Dal", "Basmatireis"},
		"texture":     {"Pilzrisotto", "Gedünsteter Brokkoli"},
		"colour":      {"Käsespätzle", "Röstzwiebeln"},
		"temperature": {"Linsen-Dal", "Basmatireis"},
		"starch":      {"Coq au Vin", "Salade verte"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := plate(t, base[tt.name]...)
			known := gaps(platefit.Read(p))
			tt.forget(&p.Sides[0])
			unknown := gaps(platefit.Read(p))
			for _, d := range tt.dims {
				assert.Contains(t, known, d, "known values show it")
				assert.NotContains(t, unknown, d, "an unknown value is no judgment")
			}
		})
	}
	t.Run("nothing known: nothing read", func(t *testing.T) {
		r := platefit.Read(platefit.Plate{Main: platefit.Component{Name: "x"}, Sides: []platefit.Component{{Name: "y"}}})
		assert.Equal(t, platefit.Reading{Cuisine: platefit.Neutral}, r)
	})
}

func TestRead_MinorGaps(t *testing.T) {
	t.Run("an acid element makes a missing fresh element minor", func(t *testing.T) {
		g := gaps(platefit.Read(plate(t, "Schweinebraten", "Salzkartoffeln", "Rotkohl")))
		assert.Equal(t, true, g[platefit.Fresh])
		assert.Equal(t, false, gaps(platefit.Read(plate(t, "Linsen-Dal", "Basmatireis")))[platefit.Fresh])
	})
	t.Run("a soft plate is minor where soft plates are the norm", func(t *testing.T) {
		assert.Equal(t, true, gaps(platefit.Read(plate(t, "Schweinefilet mit Champignons", "Salzkartoffeln")))[platefit.Texture])
		assert.Equal(t, false, gaps(platefit.Read(plate(t, "Pilzrisotto", "Gedünsteter Brokkoli")))[platefit.Texture])
	})
	t.Run("lime makes a missing cool element minor", func(t *testing.T) {
		g := gaps(platefit.Read(plate(t, "Grünes Thai-Curry", "Basmatireis", "Limettenspalten")))
		assert.Equal(t, true, g[platefit.Temperature])
	})
	t.Run("minor gaps are not raised", func(t *testing.T) {
		r := platefit.Read(plate(t, "Schweinebraten", "Salzkartoffeln", "Rotkohl"))
		assert.NotEmpty(t, r.Gaps)
		assert.Empty(t, r.Raised())
	})
}

func TestRead_Clashes(t *testing.T) {
	tests := []struct {
		name  string
		plate []string
		want  []platefit.Dimension
	}{
		{"two starches", []string{"Spaghetti Bolognese", "Salzkartoffeln"}, []platefit.Dimension{platefit.Proportions}},
		{"two mains", []string{"Frikadellen", "Schweinebraten"}, []platefit.Dimension{platefit.Proportions}},
		{"rich main beside a rich gratin", []string{"Coq au Vin", "Gratin Dauphinois"}, []platefit.Dimension{platefit.Richness}},
		{"pasta with a Japanese side", []string{"Spaghetti Bolognese", "Sunomono Gurkensalat"}, []platefit.Dimension{platefit.CuisineFit}},
		{"West-Mediterranean cuisines mix", []string{"Spaghetti Bolognese", "Salade verte", "Ofengemüse"}, nil},
		{"neutral mixes with everything", []string{"Grünes Thai-Curry", "Basmatireis", "Gedünsteter Brokkoli"}, nil},
		{"leaves carry no cuisine or starch", []string{"Spaghetti Bolognese", "Geröstete Erdnüsse", "Sesam geröstet"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, platefit.Read(plate(t, tt.plate...)).Clashes)
		})
	}
}

func TestCuisineOf(t *testing.T) {
	for _, c := range flavor.Cuisines {
		cu := platefit.CuisineOf(c)
		assert.Equal(t, c, cu.Name, "every PLATE-3 cuisine has a row")
		if c != platefit.Neutral {
			assert.NotEmpty(t, cu.Style, c)
			assert.NotEmpty(t, cu.Palette, c)
		}
	}
	for _, other := range []string{"", "korean"} {
		assert.Equal(t, platefit.CuisineOf(platefit.Neutral), platefit.CuisineOf(other), "fallback for %q", other)
	}
	p := plate(t, "Schweinebraten", "Basmatireis", "Gedünsteter Brokkoli")
	p.Main.Attrs.Cuisine = "korean"
	r := platefit.Read(p)
	assert.Equal(t, platefit.Neutral, r.Cuisine)
	assert.NotContains(t, gaps(r), platefit.Pairing, "the neutral fallback judges no pairing")
}
