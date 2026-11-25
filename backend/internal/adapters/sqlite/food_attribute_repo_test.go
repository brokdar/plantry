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
