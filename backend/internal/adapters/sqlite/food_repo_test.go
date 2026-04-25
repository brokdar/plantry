package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func newFoodRepo(t *testing.T) *sqlite.FoodRepo {
	t.Helper()
	return sqlite.NewFoodRepo(testhelper.NewTestDB(t))
}

func ptrF(v float64) *float64     { return &v }
func ptrR(r food.Role) *food.Role { return &r }

func ptrS(v string) *string { return &v }

// createTestLeaf is a helper that creates a minimal leaf food and returns its ID.
func createTestLeaf(t *testing.T, repo *sqlite.FoodRepo, name string) int64 {
	t.Helper()
	src := food.SourceManual
	f := &food.Food{Name: name, Kind: food.KindLeaf, Source: &src}
	require.NoError(t, repo.Create(context.Background(), f))
	assert.NotZero(t, f.ID)
	return f.ID
}

func TestFoodRepo_Leaf_RoundTrip(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()

	src := food.SourceManual
	f := &food.Food{
		Name:     "Chicken Breast",
		Kind:     food.KindLeaf,
		Source:   &src,
		Kcal100g: ptrF(165),
	}
	require.NoError(t, repo.Create(ctx, f))
	assert.NotZero(t, f.ID)

	got, err := repo.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, "Chicken Breast", got.Name)
	assert.Equal(t, food.KindLeaf, got.Kind)
	require.NotNil(t, got.Kcal100g)
	assert.InDelta(t, 165.0, *got.Kcal100g, 0.001)

	got.Name = "Chicken Breast (raw)"
	require.NoError(t, repo.Update(ctx, got))

	reloaded, err := repo.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, "Chicken Breast (raw)", reloaded.Name)

	require.NoError(t, repo.Delete(ctx, f.ID))
	_, err = repo.Get(ctx, f.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestFoodRepo_DuplicateName(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()

	src := food.SourceManual
	require.NoError(t, repo.Create(ctx, &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src}))
	src2 := food.SourceManual
	err := repo.Create(ctx, &food.Food{Name: "Rice", Kind: food.KindLeaf, Source: &src2})
	assert.True(t, errors.Is(err, domain.ErrDuplicateName))
}

func TestFoodRepo_DeleteNotFound(t *testing.T) {
	repo := newFoodRepo(t)
	err := repo.Delete(context.Background(), 99999)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestFoodRepo_NullableRoundtrip(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()

	src := food.SourceManual
	f := &food.Food{Name: "Apple", Kind: food.KindLeaf, Source: &src, Kcal100g: ptrF(52)}
	require.NoError(t, repo.Create(ctx, f))

	// Clear the nullable field.
	f.Kcal100g = nil
	require.NoError(t, repo.Update(ctx, f))

	got, err := repo.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Kcal100g)
}

func TestFoodRepo_Composed_RoundTrip(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()

	childID := createTestLeaf(t, repo, "Pasta")

	ref := float64(2)
	parent := &food.Food{
		Name:              "Pasta Carbonara",
		Kind:              food.KindComposed,
		Role:              ptrR(food.RoleMain),
		ReferencePortions: &ref,
		Children: []food.FoodComponent{
			{ChildID: childID, Amount: 200, Unit: "g", Grams: 200},
		},
	}
	require.NoError(t, repo.Create(ctx, parent))
	assert.NotZero(t, parent.ID)

	got, err := repo.Get(ctx, parent.ID)
	require.NoError(t, err)
	assert.Equal(t, "Pasta Carbonara", got.Name)
	assert.Equal(t, food.KindComposed, got.Kind)
	require.Len(t, got.Children, 1)
	assert.Equal(t, childID, got.Children[0].ChildID)
	assert.Equal(t, "Pasta", got.Children[0].ChildName)
	assert.InDelta(t, 200.0, got.Children[0].Amount, 0.001)
}

func TestFoodRepo_Composed_UpdateChildren(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()

	child1 := createTestLeaf(t, repo, "Egg")
	child2 := createTestLeaf(t, repo, "Bacon")

	ref := float64(1)
	parent := &food.Food{
		Name:              "Eggs and Bacon",
		Kind:              food.KindComposed,
		Role:              ptrR(food.RoleMain),
		ReferencePortions: &ref,
		Children: []food.FoodComponent{
			{ChildID: child1, Amount: 60, Unit: "g", Grams: 60},
		},
	}
	require.NoError(t, repo.Create(ctx, parent))

	// Replace children on update.
	parent.Children = []food.FoodComponent{
		{ChildID: child1, Amount: 60, Unit: "g", Grams: 60},
		{ChildID: child2, Amount: 100, Unit: "g", Grams: 100},
	}
	require.NoError(t, repo.Update(ctx, parent))

	got, err := repo.Get(ctx, parent.ID)
	require.NoError(t, err)
	assert.Len(t, got.Children, 2)
}

func TestFoodRepo_List_FilterByKind(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()

	createTestLeaf(t, repo, "Olive Oil")
	createTestLeaf(t, repo, "Butter")

	result, err := repo.List(ctx, food.ListQuery{Kind: food.KindLeaf, Limit: 50})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.Total, 2)
	for _, item := range result.Items {
		assert.Equal(t, food.KindLeaf, item.Kind)
	}
}

func TestFoodRepo_Portions_UpsertAndDelete(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()

	id := createTestLeaf(t, repo, "Rice")

	require.NoError(t, repo.UpsertPortion(ctx, &food.Portion{FoodID: id, Unit: "cup", Grams: 185}))

	portions, err := repo.ListPortions(ctx, id)
	require.NoError(t, err)
	require.Len(t, portions, 1)
	assert.Equal(t, "cup", portions[0].Unit)
	assert.InDelta(t, 185.0, portions[0].Grams, 0.001)

	// Upsert updates existing portion.
	require.NoError(t, repo.UpsertPortion(ctx, &food.Portion{FoodID: id, Unit: "cup", Grams: 200}))
	portions, err = repo.ListPortions(ctx, id)
	require.NoError(t, err)
	require.Len(t, portions, 1)
	assert.InDelta(t, 200.0, portions[0].Grams, 0.001)

	require.NoError(t, repo.DeletePortion(ctx, id, "cup"))
	portions, err = repo.ListPortions(ctx, id)
	require.NoError(t, err)
	assert.Empty(t, portions)
}
