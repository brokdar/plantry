package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func createTestIngredient(t *testing.T, repo *sqlite.IngredientRepo) *ingredient.Ingredient {
	t.Helper()
	i := &ingredient.Ingredient{Name: "Test Ingredient", Source: "manual"}
	require.NoError(t, repo.Create(context.Background(), i))
	return i
}

func TestPortionCRUD(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()
	ing := createTestIngredient(t, repo)

	// Upsert a portion
	p := &ingredient.Portion{IngredientID: ing.ID, Unit: "cup", Grams: 185}
	require.NoError(t, repo.UpsertPortion(ctx, p))

	// List portions
	portions, err := repo.ListPortions(ctx, ing.ID)
	require.NoError(t, err)
	require.Len(t, portions, 1)
	assert.Equal(t, "cup", portions[0].Unit)
	assert.Equal(t, 185.0, portions[0].Grams)
	assert.Equal(t, ing.ID, portions[0].IngredientID)

	// Upsert again (overwrite)
	p2 := &ingredient.Portion{IngredientID: ing.ID, Unit: "cup", Grams: 200}
	require.NoError(t, repo.UpsertPortion(ctx, p2))

	portions, err = repo.ListPortions(ctx, ing.ID)
	require.NoError(t, err)
	require.Len(t, portions, 1)
	assert.Equal(t, 200.0, portions[0].Grams)

	// Add a second portion
	p3 := &ingredient.Portion{IngredientID: ing.ID, Unit: "tbsp", Grams: 15}
	require.NoError(t, repo.UpsertPortion(ctx, p3))

	portions, err = repo.ListPortions(ctx, ing.ID)
	require.NoError(t, err)
	assert.Len(t, portions, 2)

	// Delete one portion
	require.NoError(t, repo.DeletePortion(ctx, ing.ID, "cup"))

	portions, err = repo.ListPortions(ctx, ing.ID)
	require.NoError(t, err)
	assert.Len(t, portions, 1)
	assert.Equal(t, "tbsp", portions[0].Unit)
}

func TestPortionCascadeDelete(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()
	ing := createTestIngredient(t, repo)

	require.NoError(t, repo.UpsertPortion(ctx, &ingredient.Portion{
		IngredientID: ing.ID, Unit: "cup", Grams: 185,
	}))
	require.NoError(t, repo.UpsertPortion(ctx, &ingredient.Portion{
		IngredientID: ing.ID, Unit: "tbsp", Grams: 15,
	}))

	// Delete the ingredient
	require.NoError(t, repo.Delete(ctx, ing.ID))

	// Portions should be gone
	portions, err := repo.ListPortions(ctx, ing.ID)
	require.NoError(t, err)
	assert.Empty(t, portions)
}

func TestPortionForeignKeyViolation(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	err := repo.UpsertPortion(ctx, &ingredient.Portion{
		IngredientID: 999, Unit: "cup", Grams: 185,
	})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeletePortionNotFound(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()
	ing := createTestIngredient(t, repo)

	err := repo.DeletePortion(ctx, ing.ID, "nonexistent")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListPortionsEmpty(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()
	ing := createTestIngredient(t, repo)

	portions, err := repo.ListPortions(ctx, ing.ID)
	require.NoError(t, err)
	assert.Empty(t, portions)
}
