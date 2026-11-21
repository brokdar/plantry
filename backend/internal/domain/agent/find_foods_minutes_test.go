package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func foodNames(items []any) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.(map[string]any)["name"].(string))
	}
	return out
}

func TestFindFoods_Minutes(t *testing.T) {
	t.Parallel()
	ts, repo := newPayloadToolSet(t)
	repo.foods[3].PrepMinutes = ptr(10)
	repo.foods[3].CookMinutes = ptr(25)

	t.Run("recipe carries minutes, leaf none", func(t *testing.T) {
		items := execJSON(t, ts, "find_foods", `{"ids":[1,3]}`)["items"].([]any)
		var curry, rice map[string]any
		for _, it := range items {
			m := it.(map[string]any)
			if m["name"] == "Curry" {
				curry = m
			} else {
				rice = m
			}
		}
		assert.EqualValues(t, 10, curry["prep_minutes"])
		assert.EqualValues(t, 25, curry["cook_minutes"])
		assert.EqualValues(t, 35, curry["total_minutes"])
		assert.NotContains(t, rice, "total_minutes")
	})

	t.Run("zero value left out", func(t *testing.T) {
		repo.foods[3].PrepMinutes = ptr(0)
		defer func() { repo.foods[3].PrepMinutes = ptr(10) }()
		m := execJSON(t, ts, "find_foods", `{"ids":[3]}`)["items"].([]any)[0].(map[string]any)
		assert.NotContains(t, m, "prep_minutes")
		assert.EqualValues(t, 25, m["total_minutes"])
	})

	t.Run("max_minutes keeps known totals only", func(t *testing.T) {
		assert.Equal(t, []string{"Curry"}, foodNames(execJSON(t, ts, "find_foods", `{"max_minutes":35}`)["items"].([]any)))
		assert.Empty(t, execJSON(t, ts, "find_foods", `{"max_minutes":34}`)["items"])
	})

	t.Run("sort minutes ascending, unknown last", func(t *testing.T) {
		repo.foods[5] = &food.Food{ID: 5, Name: "Quick", Kind: food.KindComposed, ReferencePortions: ptr(1.0), CookMinutes: ptr(5)}
		repo.foods[6] = &food.Food{ID: 6, Name: "Unknown", Kind: food.KindComposed, ReferencePortions: ptr(1.0)}
		defer delete(repo.foods, 5)
		defer delete(repo.foods, 6)
		items := execJSON(t, ts, "find_foods", `{"ids":[6,3,5],"sort":"minutes"}`)["items"].([]any)
		require.Len(t, items, 3)
		assert.Equal(t, []string{"Quick", "Curry", "Unknown"}, foodNames(items))
	})
}

func TestFindFoods_LeafRoles(t *testing.T) {
	t.Parallel()
	ts, repo := newPayloadToolSet(t)
	repo.foods[7] = &food.Food{ID: 7, Name: "Gurke", Kind: food.KindLeaf, ShopSection: ptr(food.SectionProduce)}
	role := food.RoleSauce
	repo.foods[8] = &food.Food{ID: 8, Name: "Gurke Dip", Kind: food.KindLeaf, ShopSection: ptr(food.SectionProduce), Role: &role}

	items := execJSON(t, ts, "find_foods", `{"ids":[7,8],"roles":["side_veg"]}`)["items"].([]any)
	require.Len(t, items, 1)
	m := items[0].(map[string]any)
	assert.Equal(t, "Gurke", m["name"])
	assert.Equal(t, "side_veg", m["role"])

	items = execJSON(t, ts, "find_foods", `{"ids":[7,8],"roles":["sauce"]}`)["items"].([]any)
	assert.Equal(t, []string{"Gurke Dip"}, foodNames(items))
}
