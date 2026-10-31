package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func TestFoodRepo_SetShopSection_RoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newFoodRepo(t)
	id := createTestLeaf(t, repo, "Erbsen")

	frozen := food.SectionFrozen
	f, err := repo.SetShopSection(ctx, id, &frozen)
	require.NoError(t, err)
	require.NotNil(t, f.ShopSection)
	assert.Equal(t, food.SectionFrozen, *f.ShopSection)

	got, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got.ShopSection)
	assert.Equal(t, food.SectionFrozen, *got.ShopSection)

	byID, err := repo.LookupByIDs(ctx, []int64{id})
	require.NoError(t, err)
	require.NotNil(t, byID[id].ShopSection)
	assert.Equal(t, food.SectionFrozen, *byID[id].ShopSection)

	listed, err := repo.List(ctx, food.ListQuery{Kind: food.KindLeaf, Limit: 10})
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	require.NotNil(t, listed.Items[0].ShopSection)

	f, err = repo.SetShopSection(ctx, id, nil)
	require.NoError(t, err)
	assert.Nil(t, f.ShopSection)
	got, err = repo.Get(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, got.ShopSection)
}

func TestFoodRepo_SetShopSection_NotFound(t *testing.T) {
	_, err := newFoodRepo(t).SetShopSection(context.Background(), 999, nil)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
