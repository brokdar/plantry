package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func TestFoodRepo_KeepDaysAndFreezable_RoundTrip(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()
	leafID := createTestLeaf(t, repo, "Beans")
	keep, freezable := 5, false
	chili := &food.Food{
		Name: "Chili", Kind: food.KindComposed, Role: ptrR(food.RoleMain), ReferencePortions: ptrF(4),
		Children: []food.FoodComponent{{ChildID: leafID, Amount: 400, Unit: "g", Grams: 400}},
		KeepDays: &keep, Freezable: &freezable,
	}
	require.NoError(t, repo.Create(ctx, chili))

	got, err := repo.Get(ctx, chili.ID)
	require.NoError(t, err)
	require.NotNil(t, got.KeepDays)
	assert.Equal(t, 5, *got.KeepDays)
	require.NotNil(t, got.Freezable)
	assert.False(t, *got.Freezable)

	listed, err := repo.List(ctx, food.ListQuery{Kind: food.KindComposed, Limit: 10})
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	require.NotNil(t, listed.Items[0].KeepDays, "the squirrel scan reads the new columns")
	assert.Equal(t, 5, *listed.Items[0].KeepDays)

	got.KeepDays, got.Freezable = nil, nil
	require.NoError(t, repo.Update(ctx, got))
	got, err = repo.Get(ctx, chili.ID)
	require.NoError(t, err)
	assert.Nil(t, got.KeepDays)
	assert.Nil(t, got.Freezable)
}
