package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Every food reader of the SQLite adapter honours the archive flag (LIB-4):
// archived recipes leave lists and candidate counts, stay readable by id, and
// stay in the source-url duplicate check.
func TestFoodRepo_ArchivedReaders(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := db.Exec(q, args...)
		require.NoError(t, err, q)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	repo := sqlite.NewFoodRepo(db)
	group := exec(`INSERT INTO variant_groups (name) VALUES ('g')`)
	recipe := func(name string, archived int) int64 {
		return exec(`INSERT INTO foods (name, kind, role, reference_portions, archived, variant_group_id, source_url) VALUES (?, 'composed', 'main', 1, ?, ?, ?)`,
			name, archived, group, "https://example.com/"+name)
	}
	keep, hidden := recipe("Keep", 0), recipe("Hidden", 1)
	onlyHidden := exec(`INSERT INTO foods (name, kind, source) VALUES ('Only in hidden', 'leaf', 'manual')`)
	shared := exec(`INSERT INTO foods (name, kind, source) VALUES ('Shared', 'leaf', 'manual')`)
	for _, c := range [][2]int64{{keep, shared}, {hidden, shared}, {hidden, onlyHidden}} {
		exec(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams, sort_order) VALUES (?, ?, 1, 'g', 100, 0)`, c[0], c[1])
	}

	t.Run("List default keeps active, only and all pick the rest", func(t *testing.T) {
		ids := func(q food.ListQuery) []int64 {
			q.Kind, q.Limit, q.Today = food.KindComposed, 50, time.Now()
			res, err := repo.List(ctx, q)
			require.NoError(t, err)
			out := []int64{}
			for _, f := range res.Items {
				out = append(out, f.ID)
			}
			assert.Equal(t, len(out), res.Total)
			return out
		}
		assert.Equal(t, []int64{keep}, ids(food.ListQuery{}))
		assert.Equal(t, []int64{hidden}, ids(food.ListQuery{Archive: food.ArchiveOnly}))
		assert.ElementsMatch(t, []int64{keep, hidden}, ids(food.ListQuery{Archive: food.ArchiveAll}))
		assert.Equal(t, []int64{keep}, ids(food.ListQuery{Search: "e"}))
	})
	t.Run("UneatenRecipes", func(t *testing.T) {
		got, err := repo.UneatenRecipes(ctx, time.Now().AddDate(0, 0, -30), time.Now())
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, keep, got[0].ID)
	})
	t.Run("RecipeNames", func(t *testing.T) {
		got, err := repo.RecipeNames(ctx, 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"Keep"}, got)
	})
	t.Run("CountComposedFoods", func(t *testing.T) {
		n, err := repo.CountComposedFoods(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)
	})
	t.Run("LeafNamesByRecipeUse", func(t *testing.T) {
		got, err := repo.LeafNamesByRecipeUse(ctx, 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"Shared"}, got)
	})
	t.Run("LeafIDsUsedInRecipes", func(t *testing.T) {
		got, err := repo.LeafIDsUsedInRecipes(ctx, []int64{onlyHidden, shared})
		require.NoError(t, err)
		assert.Equal(t, map[int64]bool{shared: true}, got)
	})
	t.Run("Siblings", func(t *testing.T) {
		got, err := repo.Siblings(ctx, group, 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, keep, got[0].ID)
	})
	t.Run("UnlabelledRecipes", func(t *testing.T) {
		got, err := sqlite.NewFoodAttributeRepo(db).UnlabelledRecipes(ctx)
		require.NoError(t, err)
		assert.Equal(t, []int64{keep}, got)
	})
	t.Run("Get and LookupByIDs still read an archived recipe", func(t *testing.T) {
		f, err := repo.Get(ctx, hidden)
		require.NoError(t, err)
		assert.True(t, f.Archived)
		m, err := repo.LookupByIDs(ctx, []int64{hidden})
		require.NoError(t, err)
		assert.True(t, m[hidden].Archived)
	})
	t.Run("the source-url duplicate check still sees an archived recipe", func(t *testing.T) {
		got, err := sqlite.NewRecipeSuggestionRepo(db).SourceURLs(ctx)
		require.NoError(t, err)
		assert.Contains(t, got, "https://example.com/Hidden")
	})
	t.Run("SetArchived round trip, a leaf is not found", func(t *testing.T) {
		f, err := repo.SetArchived(ctx, keep, true)
		require.NoError(t, err)
		assert.True(t, f.Archived)
		f, err = repo.SetArchived(ctx, keep, false)
		require.NoError(t, err)
		assert.False(t, f.Archived)
		_, err = repo.SetArchived(ctx, shared, true)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}
