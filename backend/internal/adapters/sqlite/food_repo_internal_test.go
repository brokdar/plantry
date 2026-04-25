package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestFoodRepo_Create_NilGuard(t *testing.T) {
	db := testhelper.NewTestDB(t)
	tx, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	repo := newFoodRepoTx(tx)
	err = repo.Create(context.Background(), &food.Food{Name: "test", Kind: food.KindLeaf})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tx-bound")
}

func TestFoodRepo_Update_NilGuard(t *testing.T) {
	db := testhelper.NewTestDB(t)
	tx, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	repo := newFoodRepoTx(tx)
	err = repo.Update(context.Background(), &food.Food{ID: 1, Name: "test", Kind: food.KindLeaf})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tx-bound")
}
