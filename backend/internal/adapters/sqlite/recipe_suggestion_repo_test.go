package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestRecipeSuggestionRepo_RoundTrip(t *testing.T) {
	repo := sqlite.NewRecipeSuggestionRepo(testhelper.NewTestDB(t))
	ctx := context.Background()
	query, day := "persian", "2026-11-02"
	id1 := int64(7)
	s := &discovery.Suggestion{
		Mode: discovery.ModeExplore, Query: &query, SourceURL: "https://x.test/fesenjan", Grounded: true, ShownOn: &day,
		Draft: importer.Draft{
			Name: "Fesenjan", SourceURL: "https://x.test/fesenjan", ReferencePortions: 4,
			Ingredients: []importer.DraftIngredient{{RawText: "500 g Walnüsse", Name: "Walnüsse", Amount: 500, Unit: "g"}},
		},
		Annotations: discovery.Annotations{
			Lines: []discovery.Line{{FoodID: &id1, Known: true, Candidate: "Walnüsse"}}, KnownShare: 1,
			NutritionComplete: true, Missing: []string{}, Why: "uses walnuts",
		},
	}
	require.NoError(t, repo.Create(ctx, s))
	assert.NotZero(t, s.ID)
	assert.Equal(t, discovery.StatusPending, s.Status)
	assert.False(t, s.CreatedAt.IsZero())

	got, err := repo.Get(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, discovery.ModeExplore, got.Mode)
	assert.Equal(t, &query, got.Query)
	assert.True(t, got.Grounded)
	assert.Equal(t, &day, got.ShownOn)
	assert.Equal(t, s.Draft, got.Draft)
	assert.Equal(t, s.Annotations, got.Annotations)
	assert.Nil(t, got.ImagePath)
	assert.Nil(t, got.Reason)
	assert.Nil(t, got.DecidedAt)

	path := "suggestions/1.jpg"
	require.NoError(t, repo.SetImagePath(ctx, s.ID, &path))
	got, _ = repo.Get(ctx, s.ID)
	assert.Equal(t, &path, got.ImagePath)
	require.NoError(t, repo.SetImagePath(ctx, s.ID, nil))
	got, _ = repo.Get(ctx, s.ID)
	assert.Nil(t, got.ImagePath, "nullable roundtrip")

	_, err = repo.Get(ctx, 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRecipeSuggestionRepo_SourceURLKnown(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewRecipeSuggestionRepo(db)
	foods := sqlite.NewFoodRepo(db)
	ctx := context.Background()

	known, err := repo.SourceURLKnown(ctx, "https://x.test/a")
	require.NoError(t, err)
	assert.False(t, known)

	require.NoError(t, repo.Create(ctx, &discovery.Suggestion{Mode: discovery.ModeDaily, SourceURL: "https://x.test/a"}))
	known, _ = repo.SourceURLKnown(ctx, "https://x.test/a")
	assert.True(t, known, "an earlier suggestion")

	portions := 2.0
	url := "https://x.test/b"
	require.NoError(t, foods.Create(ctx, &food.Food{Name: "Soup", Kind: food.KindComposed, ReferencePortions: &portions, SourceURL: &url}))
	known, _ = repo.SourceURLKnown(ctx, url)
	assert.True(t, known, "a saved food")

	known, _ = repo.SourceURLKnown(ctx, "https://x.test/c")
	assert.False(t, known)

	// Walkthrough: tracking params, a trailing slash, the fragment and the
	// host's case do not make a saved page new.
	for _, variant := range []string{
		"https://x.test/b?utm_source=newsletter&utm_medium=email",
		"https://x.test/b/",
		"HTTPS://X.Test/b#comments",
		"https://x.test/b?fbclid=abc",
	} {
		known, err = repo.SourceURLKnown(ctx, variant)
		require.NoError(t, err)
		assert.True(t, known, variant)
	}
	saved := "https://x.test/d/?utm_campaign=x"
	require.NoError(t, foods.Create(ctx, &food.Food{Name: "Stew", Kind: food.KindComposed, ReferencePortions: &portions, SourceURL: &saved}))
	known, _ = repo.SourceURLKnown(ctx, "https://x.test/d")
	assert.True(t, known, "the stored URL is normalized for the comparison too")
	known, _ = repo.SourceURLKnown(ctx, "https://x.test/b?page=2")
	assert.False(t, known, "other query params count")
}

func TestFoodRepo_LeafIDsUsedInRecipes(t *testing.T) {
	repo := newFoodRepo(t)
	ctx := context.Background()
	used, unused := createTestLeaf(t, repo, "Used"), createTestLeaf(t, repo, "Unused")
	ref := 1.0
	require.NoError(t, repo.Create(ctx, &food.Food{
		Name: "R", Kind: food.KindComposed, ReferencePortions: &ref,
		Children: []food.FoodComponent{{ChildID: used, Amount: 1, Unit: "g", Grams: 1}},
	}))

	got, err := repo.LeafIDsUsedInRecipes(ctx, []int64{used, unused, 999})
	require.NoError(t, err)
	assert.Equal(t, map[int64]bool{used: true}, got)

	got, err = repo.LeafIDsUsedInRecipes(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
