package food_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// treeRepo serves a fixed food tree for NutritionResolver tests.
type treeRepo struct {
	fakeRepo
	foods    map[int64]*food.Food
	children map[int64][]food.FoodComponent
	// lookups and childLoads record the ids of each repo call, in order.
	lookups    [][]int64
	childLoads [][]int64
}

func (r *treeRepo) LookupByIDs(_ context.Context, ids []int64) (map[int64]*food.Food, error) {
	r.lookups = append(r.lookups, slices.Sorted(slices.Values(ids)))
	out := map[int64]*food.Food{}
	for _, id := range ids {
		if f, ok := r.foods[id]; ok {
			cp := *f
			out[id] = &cp
		}
	}
	return out, nil
}

func (r *treeRepo) ListChildrenByParents(_ context.Context, ids []int64) (map[int64][]food.FoodComponent, error) {
	r.childLoads = append(r.childLoads, slices.Sorted(slices.Values(ids)))
	out := map[int64][]food.FoodComponent{}
	for _, id := range ids {
		if ch, ok := r.children[id]; ok {
			out[id] = ch
		}
	}
	return out, nil
}

func completeLeaf(id int64, name string) *food.Food {
	return &food.Food{
		ID: id, Name: name, Kind: food.KindLeaf,
		Kcal100g: ptrF64(100), Protein100g: ptrF64(10), Fat100g: ptrF64(5), Carbs100g: ptrF64(8),
	}
}

func TestNutritionResolver_PerPortionDetailed_LeafWithoutKcal(t *testing.T) {
	leaf := completeLeaf(1, "Mystery Spice")
	leaf.Kcal100g = nil
	repo := &treeRepo{foods: map[int64]*food.Food{1: leaf}}

	d, err := food.NewNutritionResolver(repo).PerPortionDetailed(context.Background(), 1)
	require.NoError(t, err)
	assert.False(t, d.Complete)
	assert.Equal(t, []food.MissingLeaf{{ID: 1, Name: "Mystery Spice"}}, d.Missing)
	assert.Zero(t, d.Macros.Kcal)
	assert.InDelta(t, 10.0, d.Macros.Protein, 1e-9)
}

func TestNutritionResolver_PerPortionDetailed_RecipeOneIncomplete(t *testing.T) {
	incomplete := completeLeaf(2, "Tofu")
	incomplete.Carbs100g = nil
	repo := &treeRepo{
		foods: map[int64]*food.Food{
			1:  completeLeaf(1, "Rice"),
			2:  incomplete,
			10: {ID: 10, Name: "Bowl", Kind: food.KindComposed},
		},
		children: map[int64][]food.FoodComponent{
			10: {{ChildID: 1, Grams: 100}, {ChildID: 2, Grams: 100}},
		},
	}

	d, err := food.NewNutritionResolver(repo).PerPortionDetailed(context.Background(), 10)
	require.NoError(t, err)
	assert.False(t, d.Complete)
	assert.Equal(t, []food.MissingLeaf{{ID: 2, Name: "Tofu"}}, d.Missing)
	// Incomplete leaves still contribute what they have; missing fields count as 0.
	assert.InDelta(t, 200.0, d.Macros.Kcal, 1e-9)
	assert.InDelta(t, 8.0, d.Macros.Carbs, 1e-9)
}

func TestNutritionResolver_PerPortionDetailed_NestedPropagates(t *testing.T) {
	miso := completeLeaf(3, "Miso")
	miso.Fat100g = nil
	butter := completeLeaf(4, "Butter")
	butter.Protein100g = nil
	repo := &treeRepo{
		foods: map[int64]*food.Food{
			1:  completeLeaf(1, "Rice"),
			3:  miso,
			4:  butter,
			20: {ID: 20, Name: "Sauce", Kind: food.KindComposed},
			10: {ID: 10, Name: "Bowl", Kind: food.KindComposed},
		},
		children: map[int64][]food.FoodComponent{
			20: {{ChildID: 3, Grams: 50}, {ChildID: 4, Grams: 10}},
			// Sauce appears twice and Miso also directly: leaves must stay unique.
			10: {{ChildID: 20, Grams: 30}, {ChildID: 1, Grams: 100}, {ChildID: 3, Grams: 5}, {ChildID: 20, Grams: 10}},
		},
	}

	d, err := food.NewNutritionResolver(repo).PerPortionDetailed(context.Background(), 10)
	require.NoError(t, err)
	assert.False(t, d.Complete)
	assert.Equal(t, []food.MissingLeaf{{ID: 4, Name: "Butter"}, {ID: 3, Name: "Miso"}}, d.Missing)
}

func TestNutritionResolver_PerPortionDetailed_MissingSortedCaseInsensitiveThenID(t *testing.T) {
	incomplete := func(id int64, name string) *food.Food {
		f := completeLeaf(id, name)
		f.Kcal100g = nil
		return f
	}
	repo := &treeRepo{
		foods: map[int64]*food.Food{
			7:  incomplete(7, "basil"),
			5:  incomplete(5, "Salt"),
			9:  incomplete(9, "Apple"),
			6:  incomplete(6, "Salt"), // same name, distinct leaf: kept, ordered by ID
			10: {ID: 10, Name: "Mix", Kind: food.KindComposed},
		},
		children: map[int64][]food.FoodComponent{
			10: {{ChildID: 7, Grams: 1}, {ChildID: 5, Grams: 1}, {ChildID: 9, Grams: 1}, {ChildID: 6, Grams: 1}},
		},
	}

	d, err := food.NewNutritionResolver(repo).PerPortionDetailed(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, []food.MissingLeaf{
		{ID: 9, Name: "Apple"}, {ID: 7, Name: "basil"}, {ID: 5, Name: "Salt"}, {ID: 6, Name: "Salt"},
	}, d.Missing)
}

func TestNutritionResolver_PerPortionDetailed_AllComplete(t *testing.T) {
	ref := float64(2)
	repo := &treeRepo{
		foods: map[int64]*food.Food{
			1:  completeLeaf(1, "Rice"),
			2:  completeLeaf(2, "Beans"),
			10: {ID: 10, Name: "Bowl", Kind: food.KindComposed, ReferencePortions: &ref},
		},
		children: map[int64][]food.FoodComponent{
			10: {{ChildID: 1, Grams: 100}, {ChildID: 2, Grams: 100}},
		},
	}
	r := food.NewNutritionResolver(repo)

	d, err := r.PerPortionDetailed(context.Background(), 10)
	require.NoError(t, err)
	assert.True(t, d.Complete)
	assert.NotNil(t, d.Missing)
	assert.Empty(t, d.Missing)
	assert.InDelta(t, 100.0, d.Macros.Kcal, 1e-9)

	m, err := r.PerPortion(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, d.Macros, m)
}

// sharedLeafTree: Bowl (10, 2 portions) = 100 g Rice + 100 g Tofu (no carbs);
// Curry (11, 1 portion) = 200 g Rice + 100 g Sauce (12, composed of 50 g
// Tofu + 50 g Rice). Rice is shared by both recipes and the nested sauce.
func sharedLeafTree() *treeRepo {
	tofu := completeLeaf(2, "Tofu")
	tofu.Carbs100g = nil
	return &treeRepo{
		foods: map[int64]*food.Food{
			1:  completeLeaf(1, "Rice"),
			2:  tofu,
			10: {ID: 10, Name: "Bowl", Kind: food.KindComposed, ReferencePortions: ptrF64(2)},
			11: {ID: 11, Name: "Curry", Kind: food.KindComposed, ReferencePortions: ptrF64(1)},
			12: {ID: 12, Name: "Sauce", Kind: food.KindComposed},
		},
		children: map[int64][]food.FoodComponent{
			10: {{ChildID: 1, Grams: 100}, {ChildID: 2, Grams: 100}},
			11: {{ChildID: 1, Grams: 200}, {ChildID: 12, Grams: 100}},
			12: {{ChildID: 2, Grams: 50}, {ChildID: 1, Grams: 50}},
		},
	}
}

func TestNutritionResolver_PerPortionDetailedMany_SharedLeaf(t *testing.T) {
	repo := sharedLeafTree()
	got, err := food.NewNutritionResolver(repo).PerPortionDetailedMany(context.Background(), []int64{10, 11})
	require.NoError(t, err)
	require.Len(t, got, 2)

	bowl := got[10]
	assert.InDelta(t, 100.0, bowl.Macros.Kcal, 1e-9, "200 g at 100 kcal/100 g over 2 portions")
	assert.InDelta(t, 4.0, bowl.Macros.Carbs, 1e-9, "Tofu adds no carbs")
	assert.False(t, bowl.Complete)
	assert.Equal(t, []food.MissingLeaf{{ID: 2, Name: "Tofu"}}, bowl.Missing)

	curry := got[11]
	assert.InDelta(t, 300.0, curry.Macros.Kcal, 1e-9)
	assert.InDelta(t, 20.0, curry.Macros.Carbs, 1e-9, "200 g rice + 50 g rice in the sauce")
	assert.Equal(t, []food.MissingLeaf{{ID: 2, Name: "Tofu"}}, curry.Missing, "missing leaves are tracked per recipe, through the sauce")

	// The batch agrees with the single-food resolve.
	for _, id := range []int64{10, 11} {
		one, err := food.NewNutritionResolver(sharedLeafTree()).PerPortionDetailed(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, one, got[id])
	}
}

func TestNutritionResolver_PerPortionDetailedMany_LoadsPerLevelNotPerRecipe(t *testing.T) {
	repo := sharedLeafTree()
	_, err := food.NewNutritionResolver(repo).PerPortionDetailedMany(context.Background(), []int64{10, 11})
	require.NoError(t, err)

	assert.Equal(t, [][]int64{{10, 11}, {1, 2, 12}}, repo.lookups, "one lookup per tree level; each food once")
	assert.Equal(t, [][]int64{{10, 11}, {12}}, repo.childLoads, "one child load per level of composed foods")
}

func TestNutritionResolver_PerPortionDetailedMany_CompleteRecipeAndEmpty(t *testing.T) {
	repo := sharedLeafTree()
	repo.children[13] = []food.FoodComponent{{ChildID: 1, Grams: 100}}
	repo.foods[13] = &food.Food{ID: 13, Name: "Plain", Kind: food.KindComposed}
	got, err := food.NewNutritionResolver(repo).PerPortionDetailedMany(context.Background(), []int64{13})
	require.NoError(t, err)
	assert.True(t, got[13].Complete)
	assert.NotNil(t, got[13].Missing)
	assert.Empty(t, got[13].Missing)

	empty := sharedLeafTree()
	none, err := food.NewNutritionResolver(empty).PerPortionDetailedMany(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, none)
	assert.Empty(t, empty.lookups, "no ids, no queries")
}

func TestNutritionResolver_PerPortionDetailedMany_UnknownFood(t *testing.T) {
	_, err := food.NewNutritionResolver(sharedLeafTree()).PerPortionDetailedMany(context.Background(), []int64{10, 99})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestNutritionResolver_PerPortionDetailedMany_CycleIsRejected(t *testing.T) {
	repo := sharedLeafTree()
	repo.children[12] = append(repo.children[12], food.FoodComponent{ChildID: 11, Grams: 10})
	_, err := food.NewNutritionResolver(repo).PerPortionDetailedMany(context.Background(), []int64{11})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}
