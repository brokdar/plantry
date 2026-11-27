package fdclocal

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func TestSearchByName_RanksTheGenericFoodFirst(t *testing.T) {
	c := New()
	tests := []struct {
		query string
		want  int
	}{
		{"cider vinegar", 173469},            // Vinegar, cider: the description's food first
		{"ground pork", 2514745},             // Pork, ground, raw: raw before cooked, fewer words
		{"pork fresh ground cooked", 167903}, // the query asks for cooked
		{"Tomatoes", 170096},                 // plural and case
		{"leeks raw", 169246},                // "raw" ranks, it matches nothing alone
		{"beans black mature seeds canned", 175188},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			cs, err := c.SearchByName(context.Background(), tt.query, 5)
			require.NoError(t, err)
			require.NotEmpty(t, cs)
			assert.Equal(t, tt.want, cs[0].FdcID, cs[0].Name)
			assert.Equal(t, food.SourceFDC, cs[0].Source)
			assert.Equal(t, cs[0].Name, cs[0].SourceName)
			assert.LessOrEqual(t, len(cs), 5)
		})
	}
	for _, q := range []string{"", "glorbnax", "and or with"} {
		cs, err := c.SearchByName(context.Background(), q, 5)
		require.NoError(t, err)
		assert.Empty(t, cs, q)
	}
}

func TestGetByID_NutritionAndPortions(t *testing.T) {
	ctx := context.Background()
	c := New()
	got, err := c.GetByID(ctx, 171610)
	require.NoError(t, err)
	assert.Equal(t, "Sauce, worcestershire", got.Name)
	require.NotNil(t, got.Kcal100g)
	assert.InDelta(t, 77, *got.Kcal100g, 0.01)
	require.NotNil(t, got.Sodium100g)
	assert.InDelta(t, 1.3, *got.Sodium100g, 0.001, "sodium in g like adapters/fdc")

	_, err = c.GetByID(ctx, 1)
	require.ErrorIs(t, err, domain.ErrNotFound)

	ps, err := c.GetFoodPortions(ctx, 171610)
	require.NoError(t, err)
	require.NotEmpty(t, ps)
	tbsp := slices.IndexFunc(ps, func(p food.PortionInfo) bool { return p.Modifier == "tbsp" })
	require.GreaterOrEqual(t, tbsp, 0, "%v", ps)
	assert.InDelta(t, 17, ps[tbsp].GramWeight, 1)
}

// TestSynonymTermsFindTheirFood: every FDC term of the bundled synonym table
// finds, offline, a food naming all its words (or the preferred fragment).
// The listed foods are not in SR Legacy or Foundation.
func TestSynonymTermsFindTheirFood(t *testing.T) {
	absent := map[string]bool{"juniper": true, "pizza_dough": true, "marzipan": true}
	b, err := os.ReadFile("../../domain/food/matchdata/synonyms.json")
	require.NoError(t, err)
	var entries []struct {
		ID     string `json:"id"`
		FDC    string `json:"fdc"`
		Prefer string `json:"fdc_prefer"`
	}
	require.NoError(t, json.Unmarshal(b, &entries))
	states := map[string]bool{"raw": true, "fresh": true, "cooked": true, "canned": true, "dried": true, "dry": true, "frozen": true}
	c := New()
	for _, e := range entries {
		if e.FDC == "" || absent[e.ID] {
			continue
		}
		cs, err := c.SearchByName(context.Background(), e.FDC, 5)
		require.NoError(t, err)
		found := slices.ContainsFunc(cs, func(x food.Candidate) bool {
			d := tokens(x.Name)
			if e.Prefer != "" && slices.ContainsFunc(tokens(e.Prefer), func(w string) bool { return slices.Contains(d, w) }) {
				return true
			}
			for _, w := range tokens(e.FDC) {
				if !states[w] && !slices.Contains(d, w) {
					return false
				}
			}
			return true
		})
		assert.True(t, found, "%s: %q found %v", e.ID, e.FDC, cs)
	}
}

func BenchmarkSearchByName(b *testing.B) {
	c := New()
	ctx := context.Background()
	if _, err := c.SearchByName(ctx, "onions raw", 5); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = c.SearchByName(ctx, "beans black mature seeds canned", 5)
	}
}
