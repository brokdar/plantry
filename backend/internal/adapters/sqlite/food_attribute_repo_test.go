package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestFoodAttributeRepo_RoundTripAndCascade(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	foods := sqlite.NewFoodRepo(db)
	repo := sqlite.NewFoodAttributeRepo(db)
	a, b := createTestLeaf(t, foods, "Tomaten"), createTestLeaf(t, foods, "Gurken")

	empty, err := repo.Get(ctx, a)
	require.NoError(t, err)
	assert.True(t, empty.Empty())

	rec := flavor.Record{
		FoodID: a, Version: flavor.Version,
		Values:  flavor.Attributes{Texture: "soft", Taste: &flavor.Taste{Acid: 2}},
		Sources: map[flavor.Field]flavor.Source{flavor.FieldTexture: flavor.SourceUser, flavor.FieldTaste: flavor.SourceAuto},
	}
	require.NoError(t, repo.Put(ctx, rec))
	got, err := repo.Get(ctx, a)
	require.NoError(t, err)
	assert.Equal(t, rec, got)

	rec.Values.Texture = "crisp"
	require.NoError(t, repo.Put(ctx, rec)) // upsert
	many, err := repo.Many(ctx, []int64{a, b})
	require.NoError(t, err)
	assert.Equal(t, map[int64]flavor.Record{a: rec}, many)

	require.NoError(t, foods.Delete(ctx, a))
	many, err = repo.Many(ctx, []int64{a})
	require.NoError(t, err)
	assert.Empty(t, many, "deleting the food deletes its attributes")

	require.NoError(t, repo.Put(ctx, flavor.Record{FoodID: b, Version: 1, Sources: map[flavor.Field]flavor.Source{flavor.FieldColour: flavor.SourceUser}, Values: flavor.Attributes{Colour: "green"}}))
	require.NoError(t, repo.Delete(ctx, b))
	got, err = repo.Get(ctx, b)
	require.NoError(t, err)
	assert.True(t, got.Empty())
}

// FIND-15: the library's cuisines are the stored cuisines of its live
// recipes, the most recipes first; neutral, archived recipes and leaves add none.
func TestFoodAttributeRepo_LibraryCuisines(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodAttributeRepo(db)
	label := func(id int64, cuisine string) {
		require.NoError(t, repo.Put(ctx, flavor.Record{
			FoodID: id, Version: flavor.Version,
			Values: flavor.Attributes{Cuisine: cuisine}, Sources: map[flavor.Field]flavor.Source{flavor.FieldCuisine: flavor.SourceAuto},
		}))
	}
	put := func(name string, archived int, cuisine string) {
		res, err := db.Exec(`INSERT INTO foods (name, kind, role, reference_portions, archived) VALUES (?, 'composed', 'main', 2, ?)`, name, archived)
		require.NoError(t, err)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		label(id, cuisine)
	}
	put("Pasta", 0, "italian")
	put("Lasagne", 0, "italian")
	put("Curry", 0, "indian")
	put("Rice", 0, "neutral")
	put("Old taco", 1, "mexican")
	label(createTestLeaf(t, sqlite.NewFoodRepo(db), "Basil"), "thai")

	got, err := repo.LibraryCuisines(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"italian", "indian"}, got, "the most recipes first")
}
