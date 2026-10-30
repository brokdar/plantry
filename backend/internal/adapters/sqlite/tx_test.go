package sqlite_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func leafFood(name string) *food.Food {
	src := food.SourceManual
	return &food.Food{Name: name, Kind: food.KindLeaf, Source: &src}
}

func countFoods(t *testing.T, repo *sqlite.FoodRepo) int {
	t.Helper()
	res, err := repo.List(context.Background(), food.ListQuery{Limit: 100})
	require.NoError(t, err)
	return res.Total
}

func TestTxRunner_CommitsOnSuccess(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	err := sqlite.NewTxRunner(db).InTx(context.Background(), func(ctx context.Context) error {
		return repo.Create(ctx, leafFood("Rice"))
	})
	require.NoError(t, err)
	assert.Equal(t, 1, countFoods(t, repo))
}

// Every repository joins the unit of work through ctx: a failure after two
// writes from two repositories rolls both back.
func TestTxRunner_RollsBackEveryRepoOnError(t *testing.T) {
	db := testhelper.NewTestDB(t)
	foods := sqlite.NewFoodRepo(db)
	slots := sqlite.NewSlotRepo(db)
	boom := errors.New("boom")

	err := sqlite.NewTxRunner(db).InTx(context.Background(), func(ctx context.Context) error {
		require.NoError(t, foods.Create(ctx, leafFood("Rice")))
		ok, err := slots.Exists(ctx, 1)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, 1, countFoodsCtx(t, ctx, foods), "the write is visible inside the unit of work")
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Equal(t, 0, countFoods(t, foods))
}

func countFoodsCtx(t *testing.T, ctx context.Context, repo *sqlite.FoodRepo) int {
	t.Helper()
	res, err := repo.List(ctx, food.ListQuery{Limit: 100})
	require.NoError(t, err)
	return res.Total
}

// A nested InTx joins the outer transaction: the inner success is rolled
// back with the outer failure.
func TestTxRunner_NestedJoinsOuter(t *testing.T) {
	db := testhelper.NewTestDB(t)
	tx := sqlite.NewTxRunner(db)
	repo := sqlite.NewFoodRepo(db)
	boom := errors.New("boom")

	err := tx.InTx(context.Background(), func(ctx context.Context) error {
		require.NoError(t, tx.InTx(ctx, func(ctx context.Context) error {
			return repo.Create(ctx, leafFood("Rice"))
		}))
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Equal(t, 0, countFoods(t, repo))
}

// B1-AC13: no repository has a tx-only constructor and the per-domain tx
// helpers are gone. Every repo resolves its queries through queriesFor(ctx).
func TestNoTxOnlyRepoConstructors(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	// Split so the B1-AC13 grep does not match this line.
	banned := regexp.MustCompile(`Run` + `In\w*Tx|new\w*RepoTx|sqlcgen\.New\(db\)`)
	for _, f := range files {
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		if loc := banned.FindIndex(src); loc != nil {
			t.Errorf("%s: %q", f, src[loc[0]:loc[1]])
		}
	}
}
