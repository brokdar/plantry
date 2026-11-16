package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func TestSearchPage_SynonymsTagsAndUmlauts(t *testing.T) {
	res := &food.ListResult{Items: []food.Food{
		{ID: 1, Name: "Kartoffelgratin"},
		{ID: 2, Name: "Hähnchen-Curry"},
		{ID: 3, Name: "Linsensuppe", Tags: []string{"Herbst", "vegan"}},
		{ID: 4, Name: "Reispfanne"},
	}, Total: 4}
	ids := func(q string) []int64 {
		var out []int64
		for _, f := range searchPage(res, q, 50, 0).Items {
			out = append(out, f.ID)
		}
		return out
	}
	assert.Equal(t, []int64{1}, ids("potato"), "English finds the German name")
	assert.Equal(t, []int64{2}, ids("HAEHNCHEN"), "case and umlaut spelling do not matter")
	assert.Equal(t, []int64{2}, ids("chicken"))
	assert.Equal(t, []int64{4}, ids("rice"))
	assert.Equal(t, []int64{3}, ids("vegan"), "tags are searched")
	assert.Equal(t, []int64{3}, ids("herbst linsen"), "every word must match")
	assert.Empty(t, ids("zebra"))
	assert.Equal(t, []int64{3}, ids("ensuppe"), "a word outside the synonym table may end a compound")
	page := searchPage(res, "", 2, 1)
	assert.Equal(t, 4, page.Total)
	assert.Len(t, page.Items, 2)
}

// Synonym members match word prefixes, not substrings.
func TestSearchPage_NoSubstringFalsePositives(t *testing.T) {
	res := &food.ListResult{Items: []food.Food{
		{ID: 1, Name: "Peanut Butter"},
		{ID: 2, Name: "Peach Cake"},
		{ID: 3, Name: "Pea Soup"},
		{ID: 4, Name: "Eggplant Curry"},
		{ID: 5, Name: "Scrambled Eggs"},
		{ID: 6, Name: "Preiselbeeren"},
		{ID: 7, Name: "Reispfanne"},
		{ID: 8, Name: "Kokosmilch"},
		{ID: 9, Name: "Linsen mit Erbsen"},
		{ID: 10, Name: "Peanut Curry"},
	}, Total: 10}
	ids := func(q string) []int64 {
		var out []int64
		for _, f := range searchPage(res, q, 50, 0).Items {
			out = append(out, f.ID)
		}
		return out
	}
	assert.Equal(t, []int64{3, 9}, ids("pea"), "pea: not Peanut or Peach; erbse is a synonym")
	assert.Equal(t, []int64{5}, ids("egg"), "egg: not Eggplant")
	assert.Equal(t, []int64{7}, ids("rice"), "reis: not Preiselbeeren")
	assert.Equal(t, []int64{7}, ids("reis"))
	assert.Equal(t, []int64{1, 10}, ids("peanut"), "peanut does not pull in the pea group")
	assert.Empty(t, ids("milch"), "milch: not Kokosmilch")
}
