package discovery_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func ptr[T any](v T) *T { return &v }

type annotateEnv struct {
	repo      *sqlite.FoodRepo
	annotator *discovery.Annotator
	nutrition *food.NutritionResolver
	leaves    map[string]int64
}

func newAnnotateEnv(t *testing.T) *annotateEnv {
	t.Helper()
	repo := sqlite.NewFoodRepo(testhelper.NewTestDB(t))
	nutrition := food.NewNutritionResolver(repo)
	return &annotateEnv{
		repo:      repo,
		nutrition: nutrition,
		leaves:    map[string]int64{},
		annotator: &discovery.Annotator{Matcher: food.NewResolver(repo, nil, nil, nil), Used: repo, Nutrition: nutrition},
	}
}

func (e *annotateEnv) leaf(t *testing.T, name string, kcal, protein, fat, carbs *float64) int64 {
	t.Helper()
	src := food.SourceManual
	f := &food.Food{Name: name, Kind: food.KindLeaf, Source: &src, Kcal100g: kcal, Protein100g: protein, Fat100g: fat, Carbs100g: carbs}
	require.NoError(t, e.repo.Create(context.Background(), f))
	e.leaves[name] = f.ID
	return f.ID
}

func (e *annotateEnv) recipe(t *testing.T, name string, portions float64, children ...food.FoodComponent) int64 {
	t.Helper()
	f := &food.Food{Name: name, Kind: food.KindComposed, Role: ptr(food.RoleMain), ReferencePortions: &portions, Children: children}
	require.NoError(t, e.repo.Create(context.Background(), f))
	return f.ID
}

func ing(name string, amount float64, unit string) importer.DraftIngredient {
	return importer.DraftIngredient{RawText: name, Name: name, Amount: amount, Unit: unit, Confidence: importer.ConfidenceParsed}
}

// fesenjan seeds the catalogue: six leaves used by a saved recipe, two
// unused, and one draft name with no leaf at all.
func fesenjan(t *testing.T) (*annotateEnv, *importer.Draft) {
	e := newAnnotateEnv(t)
	used := []string{"Walnüsse", "Granatapfelsirup", "Zwiebel", "Hähnchenbrust", "Olivenöl", "Salz"}
	for i, n := range used {
		e.leaf(t, n, ptr(float64(100+i*50)), ptr(float64(5+i)), ptr(float64(3+i)), ptr(float64(10+i)))
	}
	e.leaf(t, "Kurkuma", ptr(300.0), ptr(8.0), ptr(3.0), ptr(60.0))
	e.leaf(t, "Zimt", ptr(250.0), ptr(4.0), ptr(1.0), ptr(80.0))
	var kids []food.FoodComponent
	for _, n := range used {
		kids = append(kids, food.FoodComponent{ChildID: e.leaves[n], Amount: 10, Unit: "g", Grams: 10})
	}
	e.recipe(t, "Hähnchen-Pfanne", 2, kids...)

	onion := ing("Zwiebel", 2, "piece")
	onion.PortionGrams = ptr(120.0)
	return e, &importer.Draft{
		Name: "Fesenjan", ReferencePortions: 4, Language: "de",
		Ingredients: []importer.DraftIngredient{
			ing("Walnüsse", 300, "g"), ing("Granatapfelsirup", 100, "ml"), onion,
			ing("Hähnchenbrust", 600, "g"), ing("Olivenöl", 30, "ml"), ing("Salz", 5, "g"),
			ing("Kurkuma", 2, "g"), ing("Zimt", 1, "g"), ing("Rosenwasser", 1, "ml"),
		},
	}
}

func TestAnnotate_Fesenjan(t *testing.T) {
	e, draft := fesenjan(t)

	got, err := e.annotator.Annotate(context.Background(), draft)
	require.NoError(t, err)

	require.Len(t, got.Lines, 9)
	for i, l := range got.Lines[:6] {
		assert.True(t, l.Known, "line %d used by a recipe", i)
		require.NotNil(t, l.FoodID)
		assert.Equal(t, e.leaves[draft.Ingredients[i].Name], *l.FoodID)
		assert.Equal(t, draft.Ingredients[i].Name, l.Candidate)
	}
	for _, i := range []int{6, 7} {
		assert.False(t, got.Lines[i].Known, "unused leaf is new")
		require.NotNil(t, got.Lines[i].FoodID, "but still preselected")
	}
	assert.False(t, got.Lines[8].Known)
	assert.Nil(t, got.Lines[8].FoodID, "unmatched")
	assert.InDelta(t, 6.0/9.0, got.KnownShare, 1e-9)

	assert.False(t, got.NutritionComplete)
	assert.Equal(t, []string{"Rosenwasser"}, got.Missing)
}

func TestAnnotate_MacrosEqualSavedRecipe(t *testing.T) {
	e, draft := fesenjan(t)
	draft.Ingredients = draft.Ingredients[:8] // every line matched
	got, err := e.annotator.Annotate(context.Background(), draft)
	require.NoError(t, err)
	assert.True(t, got.NutritionComplete)
	assert.Empty(t, got.Missing)

	// The same recipe saved: the review would store these grams.
	grams := []float64{300, 100, 240, 600, 30, 5, 2, 1}
	units := []string{"g", "ml", "piece", "g", "ml", "g", "g", "g"}
	var kids []food.FoodComponent
	for i, in := range draft.Ingredients {
		kids = append(kids, food.FoodComponent{ChildID: e.leaves[in.Name], Amount: in.Amount, Unit: units[i], Grams: grams[i]})
	}
	id := e.recipe(t, "Fesenjan", 4, kids...)
	want, err := e.nutrition.PerPortion(context.Background(), id)
	require.NoError(t, err)

	assert.InDelta(t, want.Kcal, got.PerPortion.Kcal, 1e-6)
	assert.InDelta(t, want.Protein, got.PerPortion.Protein, 1e-6)
	assert.InDelta(t, want.Fat, got.PerPortion.Fat, 1e-6)
	assert.InDelta(t, want.Carbs, got.PerPortion.Carbs, 1e-6)
	assert.Positive(t, got.PerPortion.Kcal)
}

func TestAnnotate_Incomplete(t *testing.T) {
	e := newAnnotateEnv(t)
	e.leaf(t, "Mehl", ptr(350.0), ptr(10.0), ptr(1.0), ptr(70.0))
	e.leaf(t, "Geheimzutat", nil, nil, nil, nil) // matched but no macros
	draft := &importer.Draft{ReferencePortions: 2, Ingredients: []importer.DraftIngredient{
		ing("Mehl", 200, "g"), ing("Geheimzutat", 10, "g"), ing("Quark", 100, "g"),
		{RawText: "nach Geschmack Pfeffer", Name: "Pfeffer"}, // no amount
	}}

	got, err := e.annotator.Annotate(context.Background(), draft)
	require.NoError(t, err)
	assert.False(t, got.NutritionComplete)
	assert.ElementsMatch(t, []string{"Geheimzutat", "Quark", "Pfeffer"}, got.Missing)
	assert.InDelta(t, 350.0, got.PerPortion.Kcal, 1e-6, "200 g flour over 2 portions; missing lines add 0")
	assert.Zero(t, got.KnownShare, "no recipe uses any leaf")
}

func TestAnnotate_EmptyDraft(t *testing.T) {
	e := newAnnotateEnv(t)
	got, err := e.annotator.Annotate(context.Background(), &importer.Draft{})
	require.NoError(t, err)
	assert.Empty(t, got.Lines)
	assert.Zero(t, got.KnownShare)
	assert.True(t, got.NutritionComplete)
}

// countingLeaves counts the leaf listings the resolver makes.
type countingLeaves struct {
	food.Repository
	leafLists atomic.Int32
}

func (c *countingLeaves) List(ctx context.Context, q food.ListQuery) (*food.ListResult, error) {
	if q.Kind == food.KindLeaf && q.Offset == 0 {
		c.leafLists.Add(1)
	}
	return c.Repository.List(ctx, q)
}

// DoD S4: the leaf catalogue is listed once per draft, not per ingredient.
func TestAnnotate_ListsCatalogueOnce(t *testing.T) {
	e, draft := fesenjan(t)
	counting := &countingLeaves{Repository: e.repo}
	e.annotator.Matcher = food.NewResolver(counting, nil, nil, nil)

	_, err := e.annotator.Annotate(context.Background(), draft)
	require.NoError(t, err)
	assert.Equal(t, int32(1), counting.leafLists.Load())
}
