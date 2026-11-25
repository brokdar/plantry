package flavor_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

type serviceRig struct {
	svc    *flavor.Service
	events *foodevent.Service
	foods  *sqlite.FoodRepo
	store  *sqlite.FoodAttributeRepo
	leaves map[string]int64
}

func newServiceRig(t *testing.T) *serviceRig {
	t.Helper()
	db := testhelper.NewTestDB(t)
	r := &serviceRig{foods: sqlite.NewFoodRepo(db), store: sqlite.NewFoodAttributeRepo(db), leaves: map[string]int64{}}
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	r.events = foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, nil)
	r.svc = flavor.NewService(r.store, r.foods, food.NewNutritionResolver(r.foods), r.events)
	restorers[foodevent.KindAttribute] = r.svc
	for _, l := range []struct {
		name      string
		kcal, fat float64
	}{{"Hähnchenbrust", 110, 1.5}, {"Tomaten", 18, 0.2}, {"Sahne", 300, 30}, {"Zwiebeln", 40, 0.1}, {"Spinat", 23, 0.4}} {
		src := food.SourceManual
		f := &food.Food{
			Name: l.name, Kind: food.KindLeaf, Source: &src, Kcal100g: &l.kcal, Fat100g: &l.fat,
			Protein100g: new(float64), Carbs100g: new(float64),
		}
		require.NoError(t, r.foods.Create(context.Background(), f))
		r.leaves[l.name] = f.ID
	}
	return r
}

// recipe creates a composed food of one portion from name -> grams.
func (r *serviceRig) recipe(t *testing.T, name string, tags []string, grams map[string]float64) *food.Food {
	t.Helper()
	role := food.RoleMain
	portions := 1.0
	f := &food.Food{Name: name, Kind: food.KindComposed, Role: &role, ReferencePortions: &portions, Tags: tags}
	i := 0
	for n, g := range grams {
		f.Children = append(f.Children, food.FoodComponent{ChildID: r.leaves[n], Amount: g, Unit: "g", Grams: g, SortOrder: i})
		i++
	}
	require.NoError(t, r.foods.Create(context.Background(), f))
	return f
}

func TestService_UserEditSurvivesRederivation(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	f := r.recipe(t, "Curry", []string{"Indisch"}, map[string]float64{"Hähnchenbrust": 300, "Tomaten": 200, "Sahne": 100, "Zwiebeln": 50})

	v, err := r.svc.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, "indian", v.Values.Cuisine)
	assert.Equal(t, flavor.SourceRule, v.Sources[flavor.FieldCuisine])
	assert.Zero(t, v.Version, "nothing stored")

	v, undo, err := r.svc.Set(ctx, f.ID, flavor.Patch{
		Set:    flavor.Attributes{Cuisine: "thai", Kind: "curry"},
		Fields: []flavor.Field{flavor.FieldCuisine, flavor.FieldKind},
	})
	require.NoError(t, err)
	assert.NotZero(t, undo.FirstEventID)
	assert.Equal(t, "thai", v.Values.Cuisine)
	assert.Equal(t, flavor.SourceUser, v.Sources[flavor.FieldCuisine])
	assert.Equal(t, flavor.Version, v.Version)

	// Change the ingredients: derived values follow, the user's stay.
	f.Children = f.Children[:0]
	f.Children = append(f.Children, food.FoodComponent{ChildID: r.leaves["Spinat"], Amount: 400, Unit: "g", Grams: 400})
	require.NoError(t, r.foods.Update(ctx, f))
	v, err = r.svc.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Spinat"}, v.Values.Heroes)
	assert.Equal(t, "green", v.Values.Colour)
	assert.Equal(t, "thai", v.Values.Cuisine)
	assert.Equal(t, "curry", v.Values.Kind)
}

func TestService_ClearAndUndo(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	f := r.recipe(t, "Curry", []string{"Indisch"}, map[string]float64{"Hähnchenbrust": 300, "Tomaten": 200})

	_, first, err := r.svc.Set(ctx, f.ID, flavor.Patch{Set: flavor.Attributes{Colour: "red"}, Fields: []flavor.Field{flavor.FieldColour}})
	require.NoError(t, err)

	// Setting the same value again writes nothing.
	_, same, err := r.svc.Set(ctx, f.ID, flavor.Patch{Set: flavor.Attributes{Colour: "red"}, Fields: []flavor.Field{flavor.FieldColour}})
	require.NoError(t, err)
	assert.Zero(t, same.FirstEventID)

	v, cleared, err := r.svc.Set(ctx, f.ID, flavor.Patch{Clear: []flavor.Field{flavor.FieldColour}})
	require.NoError(t, err)
	assert.Equal(t, flavor.SourceRule, v.Sources[flavor.FieldColour])
	rec, err := r.store.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.True(t, rec.Empty(), "clearing the last value removes the row")

	// Undo the clear: the user's colour is back.
	subjects, err := r.events.Revert(ctx, cleared)
	require.NoError(t, err)
	assert.Equal(t, []foodevent.Subject{{Kind: foodevent.KindAttribute, SubjectID: f.ID}}, subjects)
	v, err = r.svc.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, "red", v.Values.Colour)
	assert.Equal(t, flavor.SourceUser, v.Sources[flavor.FieldColour])

	// The first edit has a later change now: its undo is stale.
	_, err = r.events.Revert(ctx, first)
	require.ErrorIs(t, err, domain.ErrPlanChangeStale)
}

func TestService_UndoOfFirstEditRemovesRow(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	f := r.recipe(t, "Curry", nil, map[string]float64{"Hähnchenbrust": 300})
	_, undo, err := r.svc.Set(ctx, f.ID, flavor.Patch{Set: flavor.Attributes{Saucy: ptr(true)}, Fields: []flavor.Field{flavor.FieldSaucy}})
	require.NoError(t, err)
	_, err = r.events.Revert(ctx, undo)
	require.NoError(t, err)
	rec, err := r.store.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.True(t, rec.Empty())
}

func TestService_SetRejects(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	f := r.recipe(t, "Curry", nil, map[string]float64{"Hähnchenbrust": 300, "Tomaten": 200})
	cases := []struct {
		name string
		p    flavor.Patch
	}{
		{"value outside vocabulary", flavor.Patch{Set: flavor.Attributes{Cuisine: "martian"}, Fields: []flavor.Field{flavor.FieldCuisine}}},
		{"hero not an ingredient", flavor.Patch{Set: flavor.Attributes{Heroes: []string{"Lachs"}}, Fields: []flavor.Field{flavor.FieldHeroes}}},
		{"too many heroes", flavor.Patch{Set: flavor.Attributes{Heroes: []string{"Tomaten", "Tomaten", "Tomaten", "Tomaten"}}, Fields: []flavor.Field{flavor.FieldHeroes}}},
		{"richness out of range", flavor.Patch{Set: flavor.Attributes{Richness: ptr(4)}, Fields: []flavor.Field{flavor.FieldRichness}}},
		{"taste out of range", flavor.Patch{Set: flavor.Attributes{Taste: &flavor.Taste{Heat: -1}}, Fields: []flavor.Field{flavor.FieldTaste}}},
		{"empty value", flavor.Patch{Fields: []flavor.Field{flavor.FieldKind}}},
		{"unknown field", flavor.Patch{Clear: []flavor.Field{"spiciness"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := r.svc.Set(ctx, f.ID, tc.p)
			require.ErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
	t.Run("leaf takes only its subset", func(t *testing.T) {
		_, _, err := r.svc.Set(ctx, r.leaves["Spinat"], flavor.Patch{Set: flavor.Attributes{Kind: "salad"}, Fields: []flavor.Field{flavor.FieldKind}})
		require.ErrorIs(t, err, domain.ErrInvalidInput)
		v, _, err := r.svc.Set(ctx, r.leaves["Spinat"], flavor.Patch{Set: flavor.Attributes{Texture: "soft"}, Fields: []flavor.Field{flavor.FieldTexture}})
		require.NoError(t, err)
		assert.Equal(t, flavor.Attributes{Texture: "soft", Colour: "green"}, v.Values)
	})
	t.Run("missing food", func(t *testing.T) {
		_, err := r.svc.Get(ctx, 9999)
		require.ErrorIs(t, err, domain.ErrNotFound)
		_, _, err = r.svc.Set(ctx, 9999, flavor.Patch{Clear: []flavor.Field{flavor.FieldKind}})
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}
