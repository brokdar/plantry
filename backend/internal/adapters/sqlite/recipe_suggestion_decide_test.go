package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func seedDiscoveryRun(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO ai_runs (kind, started_at, status) VALUES ('discovery', datetime('now'), 'ok')`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func runOutcome(t *testing.T, db *sql.DB, id int64) *string {
	t.Helper()
	var out sql.NullString
	require.NoError(t, db.QueryRow(`SELECT outcome FROM ai_runs WHERE id = ?`, id).Scan(&out))
	if !out.Valid {
		return nil
	}
	return &out.String
}

func createSuggestion(t *testing.T, repo *sqlite.RecipeSuggestionRepo, url string, runID *int64) int64 {
	t.Helper()
	s := &discovery.Suggestion{
		Mode: discovery.ModeDaily, SourceURL: url, RunID: runID,
		Draft: importer.Draft{Name: "Recipe " + url, SourceURL: url},
	}
	require.NoError(t, repo.Create(context.Background(), s))
	return s.ID
}

func TestRecipeSuggestionRepo_DecideMovesRunOutcome(t *testing.T) {
	ctx := context.Background()
	tooLong := discovery.ReasonTooLong
	accept := func(foodID int64) discovery.Decision {
		return discovery.Decision{Status: discovery.StatusAccepted, FoodID: &foodID}
	}
	reject := discovery.Decision{Status: discovery.StatusRejected, Reason: &tooLong}

	t.Run("any accepted is accepted", func(t *testing.T) {
		db := testhelper.NewTestDB(t)
		repo := sqlite.NewRecipeSuggestionRepo(db)
		run := seedDiscoveryRun(t, db)
		a := createSuggestion(t, repo, "https://x.test/a", &run)
		b := createSuggestion(t, repo, "https://x.test/b", &run)
		assert.Nil(t, runOutcome(t, db, run), "undecided run keeps NULL")

		got, err := repo.Decide(ctx, a, reject)
		require.NoError(t, err)
		assert.Equal(t, discovery.StatusRejected, got.Status)
		assert.Equal(t, &tooLong, got.Reason)
		assert.NotNil(t, got.DecidedAt)
		assert.Equal(t, "open", *runOutcome(t, db, run), "one of two rejected")

		foodID := seedRecipeRow(t, db, "Saved")
		got, err = repo.Decide(ctx, b, accept(foodID))
		require.NoError(t, err)
		assert.Equal(t, discovery.StatusAccepted, got.Status)
		assert.Equal(t, &foodID, got.FoodID)
		assert.Equal(t, "accepted", *runOutcome(t, db, run))
	})

	t.Run("all rejected is discarded", func(t *testing.T) {
		db := testhelper.NewTestDB(t)
		repo := sqlite.NewRecipeSuggestionRepo(db)
		run := seedDiscoveryRun(t, db)
		a := createSuggestion(t, repo, "https://x.test/a", &run)
		b := createSuggestion(t, repo, "https://x.test/b", &run)
		_, err := repo.Decide(ctx, a, reject)
		require.NoError(t, err)
		got, err := repo.Decide(ctx, b, discovery.Decision{Status: discovery.StatusRejected})
		require.NoError(t, err)
		assert.Nil(t, got.Reason, "reason is optional")
		assert.Equal(t, "discarded", *runOutcome(t, db, run))
	})

	t.Run("decided twice and missing", func(t *testing.T) {
		db := testhelper.NewTestDB(t)
		repo := sqlite.NewRecipeSuggestionRepo(db)
		id := createSuggestion(t, repo, "https://x.test/a", nil)
		_, err := repo.Decide(ctx, id, reject)
		require.NoError(t, err, "a suggestion without a run")
		_, err = repo.Decide(ctx, id, accept(seedRecipeRow(t, db, "X")))
		assert.ErrorIs(t, err, domain.ErrDiscoveryDecided)
		_, err = repo.Decide(ctx, 999, reject)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestRecipeSuggestionRepo_List(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewRecipeSuggestionRepo(db)
	ctx := context.Background()
	run := seedDiscoveryRun(t, db)
	a := createSuggestion(t, repo, "https://x.test/a", &run)
	b := createSuggestion(t, repo, "https://x.test/b", nil)
	c := createSuggestion(t, repo, "https://x.test/c", &run)
	for _, id := range []int64{c, a} { // a decided last
		_, err := repo.Decide(ctx, id, discovery.Decision{Status: discovery.StatusRejected})
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE recipe_suggestions SET decided_at = datetime('now', ?) WHERE id = ?`,
			map[int64]string{c: "-2 minutes", a: "-1 minutes"}[id], id)
		require.NoError(t, err)
	}
	ids := func(ss []discovery.Suggestion) []int64 {
		out := []int64{}
		for _, s := range ss {
			out = append(out, s.ID)
		}
		return out
	}
	rejected := discovery.StatusRejected
	pending := discovery.StatusPending

	got, err := repo.List(ctx, discovery.ListFilter{Status: &rejected, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []int64{a, c}, ids(got), "last decided first")
	assert.Equal(t, "Recipe https://x.test/a", got[0].Draft.Name)

	got, err = repo.List(ctx, discovery.ListFilter{Status: &rejected, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, []int64{a}, ids(got))

	got, err = repo.List(ctx, discovery.ListFilter{Status: &pending, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []int64{b}, ids(got))

	got, err = repo.List(ctx, discovery.ListFilter{RunID: &run, Limit: 10})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{a, c}, ids(got))
}

func TestRecipeSuggestionRepo_DeletePendingBefore(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewRecipeSuggestionRepo(db)
	ctx := context.Background()
	old := createSuggestion(t, repo, "https://x.test/old", nil)
	oldRejected := createSuggestion(t, repo, "https://x.test/old-rejected", nil)
	fresh := createSuggestion(t, repo, "https://x.test/fresh", nil)
	_, err := repo.Decide(ctx, oldRejected, discovery.Decision{Status: discovery.StatusRejected})
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE recipe_suggestions SET created_at = datetime('now', '-31 days') WHERE id IN (?, ?)`, old, oldRejected)
	require.NoError(t, err)

	ids, err := repo.DeletePendingBefore(ctx, time.Now().AddDate(0, 0, -30))
	require.NoError(t, err)
	assert.Equal(t, []int64{old}, ids)
	_, err = repo.Get(ctx, old)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	for _, id := range []int64{oldRejected, fresh} {
		_, err = repo.Get(ctx, id)
		assert.NoError(t, err, "decided and fresh rows stay")
	}
}

func TestFoodRepo_DiscoveryNames(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()
	rice, onion := createTestLeaf(t, repo, "Rice"), createTestLeaf(t, repo, "Onion")
	createTestLeaf(t, repo, "Unused")
	ref := 1.0
	recipe := func(name string, leaves ...int64) {
		f := &food.Food{Name: name, Kind: food.KindComposed, ReferencePortions: &ref}
		for _, id := range leaves {
			f.Children = append(f.Children, food.FoodComponent{ChildID: id, Amount: 1, Unit: "g", Grams: 1})
		}
		require.NoError(t, repo.Create(ctx, f))
	}
	recipe("Pilaf", rice, onion)
	recipe("Soup", onion)

	names, err := repo.RecipeNames(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"Soup", "Pilaf"}, names, "newest first")

	leaves, err := repo.LeafNamesByRecipeUse(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"Onion", "Rice"}, leaves, "most used first, unused left out")
	leaves, err = repo.LeafNamesByRecipeUse(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"Onion"}, leaves)
}

func seedRecipeRow(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO foods (name, kind, role, reference_portions) VALUES (?, 'composed', 'main', 4)`, name)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}
