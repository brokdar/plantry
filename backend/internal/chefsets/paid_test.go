package chefsets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaidSets_LoadAndHaveTheBriefedSize(t *testing.T) {
	finds, err := LoadFinds()
	require.NoError(t, err)
	assert.Len(t, finds, 40)
	themes, constraints := map[string]bool{}, map[string]bool{}
	for _, f := range finds {
		themes[f.Theme], constraints[f.Constraint] = true, true
	}
	assert.GreaterOrEqual(t, len(themes), 5)
	for _, c := range []string{"vegetarian", "no_pork", "mild_child", "max_30min", "german"} {
		assert.True(t, constraints[c], c)
	}

	ic, err := LoadImportCheck()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(ic.Planted), 20)
	assert.GreaterOrEqual(t, len(ic.Recipes), 10, "every base recipe is a clean case")
	kinds := map[string]int{}
	for _, p := range ic.Planted {
		kinds[p.Kind]++
	}
	assert.Len(t, kinds, len(plantKinds), "every planted kind is used")

	subs, err := LoadSubs()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(subs), 20)
	var allergen, diet int
	for _, c := range subs {
		if len(c.Allergens) > 0 {
			allergen++
		}
		if c.Diet != "" {
			diet++
		}
	}
	assert.Positive(t, allergen)
	assert.Positive(t, diet)

	h, err := LoadHelp()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(h.Questions), 10)
}
