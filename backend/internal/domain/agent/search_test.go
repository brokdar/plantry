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
	assert.Equal(t, []int64{3}, ids("herbst suppe"), "every word must match")
	assert.Empty(t, ids("zebra"))
	page := searchPage(res, "e", 2, 1)
	assert.Equal(t, 4, page.Total)
	assert.Len(t, page.Items, 2)
}
