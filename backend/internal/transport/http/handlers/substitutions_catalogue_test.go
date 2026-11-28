package handlers_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdclocal"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/recipechange"
	"github.com/jaltszeimer/plantry/backend/internal/domain/substitution"
)

// catalogueService is the substitution service with the catalogue path and a
// household, on the environment's database.
func (e *subEnv) catalogueService(h gates.Household) *substitution.Service {
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(e.db), sqlite.NewFoodEventRepo(e.db), restorers, nil)
	recipes := recipechange.NewService(e.foods, nil, nil, sqlite.NewRecipeChangeRepo(e.db), nil, events)
	svc := substitution.NewService(e.foods, e.sessions, sqlite.NewSubstitutionRepo(e.db), recipes, events).
		WithCatalogue(food.NewMatcher(e.foods, fdclocal.New(), nil), sqlite.NewFoodOriginRepo(e.db)).
		WithHousehold(func(context.Context) (gates.Household, error) { return h, nil })
	svc.Now = func() time.Time { return *e.now }
	return svc
}

func optionNames(opts []substitution.Option) []string {
	out := []string{}
	for _, o := range opts {
		out = append(out, o.Name)
	}
	return out
}

// SUB-1: a table substitute the catalogue lacks is added the way an import
// adds a food (offline USDA catalogue), once, with its origin.
func TestSubstitution_MissingSubstituteIsAddedToCatalogue(t *testing.T) {
	e := newSubEnv(t)
	ctx := context.Background()
	svc := e.catalogueService(gates.Household{})

	opts, _, err := svc.Options(ctx, e.onion.ID, []int64{e.b})
	require.NoError(t, err)
	require.Len(t, opts, 3, "the catalogue held one of the table's four substitutes")
	assert.Equal(t, "Schalotte", opts[0].Name)
	for _, o := range opts[1:] {
		f, err := e.foods.Get(ctx, o.FoodID)
		require.NoError(t, err)
		assert.Equal(t, food.KindLeaf, f.Kind)
		require.NotNil(t, f.Kcal100g, "%s has nutrition", o.Name)
		var kind, ref string
		require.NoError(t, e.db.QueryRowContext(ctx, `SELECT kind, ref FROM food_origins WHERE food_id = ?`, o.FoodID).Scan(&kind, &ref))
		assert.Equal(t, food.OriginImport, kind)
		assert.Contains(t, ref, "substitute:")
	}

	again, _, err := svc.Options(ctx, e.onion.ID, []int64{e.b})
	require.NoError(t, err)
	assert.Equal(t, optionNames(opts), optionNames(again), "the second ask reuses the added leaves")

	// Apply takes the added leaf though its USDA name is not the table's.
	_, err = svc.Apply(ctx, e.b, e.onion.ID, opts[1].FoodID)
	require.NoError(t, err)
}

// SUB-3: an option the household cannot eat is not offered, and it is not
// added to the catalogue either. Spring onions are an onion for "never onion".
func TestSubstitution_HouseholdFiltersOptions(t *testing.T) {
	e := newSubEnv(t)
	ctx := context.Background()
	svc := e.catalogueService(gates.Household{NeverNames: []string{"onion"}})

	opts, _, err := svc.Options(ctx, e.onion.ID, []int64{e.b})
	require.NoError(t, err)
	assert.NotContains(t, optionNames(opts), "Spring onions")
	assert.NotEmpty(t, opts)
	var n int
	require.NoError(t, e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM foods WHERE LOWER(name) LIKE '%spring onion%'`).Scan(&n))
	assert.Zero(t, n, "a refused substitute is not added")
}
