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

func TestCreateAndGet(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	i := &ingredient.Ingredient{
		Name:        "Chicken Breast",
		Source:      "manual",
		Kcal100g:    165,
		Protein100g: 31,
		Fat100g:     3.6,
		Carbs100g:   0,
		Fiber100g:   0,
		Sodium100g:  74,
	}
	err := repo.Create(ctx, i)
	require.NoError(t, err)
	assert.NotZero(t, i.ID)
	assert.False(t, i.CreatedAt.IsZero())
	assert.False(t, i.UpdatedAt.IsZero())

	got, err := repo.Get(ctx, i.ID)
	require.NoError(t, err)
	assert.Equal(t, i.Name, got.Name)
	assert.Equal(t, i.Source, got.Source)
	assert.Equal(t, i.Kcal100g, got.Kcal100g)
	assert.Equal(t, i.Protein100g, got.Protein100g)
	assert.Nil(t, got.Barcode)
}

func TestCreateDuplicateName(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	_ = repo.Create(ctx, &ingredient.Ingredient{Name: "Tofu", Source: "manual"})
	err := repo.Create(ctx, &ingredient.Ingredient{Name: "Tofu", Source: "manual"})
	assert.ErrorIs(t, err, domain.ErrDuplicateName)
}

func TestUpdate(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	i := &ingredient.Ingredient{Name: "Rice", Source: "manual", Kcal100g: 130}
	require.NoError(t, repo.Create(ctx, i))

	i.Name = "Brown Rice"
	i.Kcal100g = 112
	barcode := "1234567890"
	i.Barcode = &barcode
	require.NoError(t, repo.Update(ctx, i))

	got, err := repo.Get(ctx, i.ID)
	require.NoError(t, err)
	assert.Equal(t, "Brown Rice", got.Name)
	assert.Equal(t, 112.0, got.Kcal100g)
	assert.NotNil(t, got.Barcode)
	assert.Equal(t, "1234567890", *got.Barcode)
}

func TestUpdateNotFound(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	err := repo.Update(context.Background(), &ingredient.Ingredient{ID: 999, Name: "X", Source: "manual"})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDelete(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	i := &ingredient.Ingredient{Name: "Butter", Source: "manual"}
	require.NoError(t, repo.Create(ctx, i))
	require.NoError(t, repo.Delete(ctx, i.ID))

	_, err := repo.Get(ctx, i.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteNotFound(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	err := repo.Delete(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListPagination(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	names := []string{"Apple", "Banana", "Carrot", "Date", "Eggplant"}
	for _, n := range names {
		require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: n, Source: "manual"}))
	}

	result, err := repo.List(ctx, ingredient.ListQuery{Limit: 2, Offset: 0, SortBy: "name"})
	require.NoError(t, err)
	assert.Equal(t, 5, result.Total)
	assert.Len(t, result.Items, 2)
}

func TestListFTSSearch(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: "Chicken Breast", Source: "manual"}))
	require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: "Chicken Thigh", Source: "manual"}))
	require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: "Tofu", Source: "manual"}))

	result, err := repo.List(ctx, ingredient.ListQuery{Search: "chicken", Limit: 50, SortBy: "name"})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Len(t, result.Items, 2)
}

func TestListSort(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: "Banana", Source: "manual"}))
	require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: "Apple", Source: "manual"}))
	require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: "Cherry", Source: "manual"}))

	// ascending
	asc, err := repo.List(ctx, ingredient.ListQuery{Limit: 50, SortBy: "name", SortDesc: false})
	require.NoError(t, err)
	assert.Equal(t, "Apple", asc.Items[0].Name)
	assert.Equal(t, "Cherry", asc.Items[2].Name)

	// descending
	desc, err := repo.List(ctx, ingredient.ListQuery{Limit: 50, SortBy: "name", SortDesc: true})
	require.NoError(t, err)
	assert.Equal(t, "Cherry", desc.Items[0].Name)
	assert.Equal(t, "Apple", desc.Items[2].Name)
}

func TestFTSSearchSpecialChars(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: "Chicken Breast", Source: "manual"}))
	require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: "Tofu", Source: "manual"}))

	// FTS5 reserved words and special characters must not cause errors
	cases := []string{
		"chicken AND",
		"chicken OR tofu",
		"NOT chicken",
		`chi"cken`,
		"chicken*",
		"chicken^",
	}
	for _, search := range cases {
		result, err := repo.List(ctx, ingredient.ListQuery{Search: search, Limit: 50, SortBy: "name"})
		assert.NoError(t, err, "search=%q should not error", search)
		assert.NotNil(t, result, "search=%q should return a result", search)
	}
}

func TestUpdateDuplicateName(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	a := &ingredient.Ingredient{Name: "Alpha", Source: "manual"}
	b := &ingredient.Ingredient{Name: "Beta", Source: "manual"}
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))

	b.Name = "Alpha"
	err := repo.Update(ctx, b)
	assert.ErrorIs(t, err, domain.ErrDuplicateName)
}

func TestListOffsetBeyondTotal(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	for _, n := range []string{"A", "B", "C"} {
		require.NoError(t, repo.Create(ctx, &ingredient.Ingredient{Name: n, Source: "manual"}))
	}

	result, err := repo.List(ctx, ingredient.ListQuery{Limit: 50, Offset: 100, SortBy: "name"})
	require.NoError(t, err)
	assert.Empty(t, result.Items)
	assert.Equal(t, 3, result.Total)
}

func TestNullableFieldsRoundtrip(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	barcode := "1234567890"
	i := &ingredient.Ingredient{Name: "Test", Source: "manual", Barcode: &barcode}
	require.NoError(t, repo.Create(ctx, i))

	got, err := repo.Get(ctx, i.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Barcode)
	assert.Equal(t, "1234567890", *got.Barcode)

	// Update to nil
	i.Barcode = nil
	require.NoError(t, repo.Update(ctx, i))

	got, err = repo.Get(ctx, i.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Barcode)
}
