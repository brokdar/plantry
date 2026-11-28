package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/libreview"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// The library review store reads active recipes with their ingredients and
// the plans a recipe was taken off, groups variants and keeps the gap queue.
func TestLibraryReviewRepo(t *testing.T) {
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
	repo := sqlite.NewLibraryReviewRepo(db)
	leaf := func(name string) int64 {
		return exec(`INSERT INTO foods (name, kind, source) VALUES (?, 'leaf', 'manual')`, name)
	}
	recipe := func(name string, archived int) int64 {
		return exec(`INSERT INTO foods (name, kind, role, reference_portions, archived, prep_minutes) VALUES (?, 'composed', 'main', 1, ?, 10)`, name, archived)
	}
	rice, egg := leaf("Rice"), leaf("Egg")
	a, b, gone := recipe("A", 0), recipe("B", 0), recipe("Archived", 1)
	for _, c := range [][2]int64{{a, egg}, {a, rice}, {a, rice}, {b, rice}, {gone, rice}} {
		exec(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams, sort_order) VALUES (?, ?, 1, 'g', 10, 0)`, c[0], c[1])
	}
	exec(`INSERT INTO food_instructions (food_id, step_number, text) VALUES (?, 1, 'cook')`, a)

	t.Run("recipes", func(t *testing.T) {
		rs, err := repo.Recipes(ctx)
		require.NoError(t, err)
		require.Len(t, rs, 2, "archived recipes are left out")
		assert.Equal(t, libreview.Recipe{ID: a, Name: "A", Role: "main", Prep: 10, Steps: 1, Ingredients: []int64{rice, egg}}, rs[0])
		assert.Equal(t, []int64{rice}, rs[1].Ingredients)
	})

	t.Run("replaced counts distinct plates and swaps in the window", func(t *testing.T) {
		now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
		sig := func(kind string, w float64, at time.Time, food int64, plate any) {
			exec(`INSERT INTO taste_signals (kind, level, key, weight, share, at, food_id, plate_id) VALUES (?, 'flavour', 'x', ?, 1, ?, ?, ?)`,
				kind, w, at.Format("2006-01-02 15:04:05"), food, plate)
		}
		for i := 0; i < 2; i++ { // two attributes blamed for one removal: one event
			sig("plate_removed", -0.3, now.Add(-time.Hour), a, 7)
		}
		sig("plate_removed", -0.3, now.Add(-2*time.Hour), a, 8)
		sig("review_swap", -0.5, now.Add(-3*time.Hour), a, nil)
		sig("review_swap", 0.5, now.Add(-4*time.Hour), a, nil) // swapped in: not a replacement
		sig("plate_removed", -0.3, now.AddDate(0, 0, -200), a, 9)
		sig("plate_removed", -0.3, now.Add(-time.Hour), b, 10)
		got, err := repo.Replaced(ctx, now.AddDate(0, 0, -90))
		require.NoError(t, err)
		assert.Equal(t, map[int64]int{a: 3, b: 1}, got)
	})

	t.Run("state", func(t *testing.T) {
		st, err := repo.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, libreview.State{}, st)
		want := libreview.State{Month: "2026-11", CardDismissed: true, Findings: []libreview.Finding{
			{Key: "data", Kind: libreview.KindData, Recipes: []libreview.RecipeRef{{ID: a, Name: "A", Missing: []string{"times"}}}, Dismissed: true},
		}}
		require.NoError(t, repo.SaveState(ctx, want))
		st, err = repo.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, st)
	})

	t.Run("group variants", func(t *testing.T) {
		require.NoError(t, repo.GroupVariants(ctx, a, b))
		var ga, gb int64
		require.NoError(t, db.QueryRow(`SELECT variant_group_id FROM foods WHERE id = ?`, a).Scan(&ga))
		require.NoError(t, db.QueryRow(`SELECT variant_group_id FROM foods WHERE id = ?`, b).Scan(&gb))
		assert.NotZero(t, ga)
		assert.Equal(t, ga, gb)
		c := recipe("C", 0)
		require.NoError(t, repo.GroupVariants(ctx, c, b))
		var gc int64
		require.NoError(t, db.QueryRow(`SELECT variant_group_id FROM foods WHERE id = ?`, c).Scan(&gc))
		assert.Equal(t, ga, gc, "joins the group the other already has")
	})

	t.Run("gap queue", func(t *testing.T) {
		at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, repo.QueueGap(ctx, libreview.Gap{Kind: "quick_main", Dish: "quick"}, at))
		require.NoError(t, repo.QueueGap(ctx, libreview.Gap{Kind: "quick_main", Dish: "other"}, at.Add(time.Hour)))
		require.NoError(t, repo.QueueGap(ctx, libreview.Gap{Kind: "few_mains", Dish: "mains"}, at.Add(time.Minute)))
		gaps, err := repo.Gaps(ctx)
		require.NoError(t, err)
		require.Len(t, gaps, 2, "one gap per kind")
		assert.Equal(t, "quick_main", gaps[0].Kind)
		assert.Equal(t, "quick", gaps[0].Dish)
		f, err := sqlite.NewFindRepo(db).Create(ctx, "quick", finds.TriggerPlanning)
		require.NoError(t, err)
		find := f.ID
		require.NoError(t, repo.StartGap(ctx, "quick_main", find))
		gaps, _ = repo.Gaps(ctx)
		assert.Equal(t, find, gaps[0].FindID)
		assert.True(t, gaps[0].Offered)
		require.NoError(t, repo.FreeGap(ctx, "quick_main"))
		require.NoError(t, repo.NewGapSession(ctx))
		gaps, _ = repo.Gaps(ctx)
		assert.Zero(t, gaps[0].FindID)
		assert.False(t, gaps[0].Offered)
		require.NoError(t, repo.DeleteGap(ctx, "quick_main"))
		gaps, _ = repo.Gaps(ctx)
		require.Len(t, gaps, 1)
	})
}
