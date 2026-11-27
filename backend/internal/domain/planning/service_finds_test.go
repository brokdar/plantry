package planning_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// findsEnv is the service env with the Chef on and a find service placing
// ready finds (C3).
type findsEnv struct {
	*svcEnv
	store  *sqlite.FindRepo
	finds  *finds.Service
	lentil int64
}

func newFindsEnv(t *testing.T, chefOn bool) *findsEnv {
	t.Helper()
	e := &findsEnv{svcEnv: newSvcEnv(t)}
	require.NoError(t, testsvc.Settings(e.db).Set(e.ctx, "plan.explore_per_week", "1"))
	e.lentil = e.insert(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g) VALUES ('Linsen', 'leaf', 'manual', 116, 9, 0.4, 20)`)
	foods := sqlite.NewFoodRepo(e.db)
	e.store = sqlite.NewFindRepo(e.db)
	e.finds = finds.NewService(finds.Deps{
		Store: e.store, Tx: sqlite.NewTxRunner(e.db),
		Planning: finds.Planning{
			Library: finds.NewLibrary(finds.LibraryDeps{
				Foods: foods, Catalogue: foods, Origins: sqlite.NewFoodOriginRepo(e.db),
				Finalizer: importer.NewService(nil, nil, nil, nil, foods),
				Plates:    e.plates,
			}),
			ChefOn: func(context.Context) (bool, error) { return chefOn, nil },
			Now:    time.Now,
		},
	})
	e.deps.Finds = e.finds
	e.svc = planning.NewService(e.deps)
	return e
}

// readyFind stores a ready find of one matched lentil line.
func (e *findsEnv) readyFind(name string) int64 {
	e.t.Helper()
	grams := 600.0
	d := &importer.Draft{
		Name: name, ReferencePortions: 4, Instructions: []string{"Cook the lentils."},
		Ingredients: []importer.DraftIngredient{{
			Name: "lentils", Amount: 600, Unit: "g",
			Match: &food.LineMatch{
				Status: food.MatchMatched, Pick: 0, Grams: &grams,
				Candidates: []food.Candidate{{Name: "Linsen", ExistingID: &e.lentil}},
			},
		}},
	}
	f, err := e.store.Create(e.ctx, name, finds.TriggerChat)
	require.NoError(e.t, err)
	_, err = e.store.Finish(e.ctx, f.ID, finds.Outcome{State: finds.StateReady, Draft: d, SourceURL: "https://example.org/" + name})
	require.NoError(e.t, err)
	return f.ID
}

func (e *findsEnv) find(id int64) *finds.Find {
	e.t.Helper()
	f, err := e.store.Get(e.ctx, id)
	require.NoError(e.t, err)
	return f
}

// eaten reports whether a window plate eats foodID.
func (e *findsEnv) eaten(foodID int64) bool {
	for _, p := range e.window() {
		for _, c := range p.Components {
			if c.FoodID == foodID {
				return true
			}
		}
	}
	return false
}

// FIND-3, FIND-4: generating a window with a new dish places a ready find
// (ready -> in_library, placed in the window); one the quota leaves out
// stays ready and leaves no recipe behind.
func TestGenerate_PlacesReadyFinds(t *testing.T) {
	e := newFindsEnv(t, true)
	first := e.readyFind("Dal")
	second := e.readyFind("Lentil stew")

	e.generate(e.req())

	placed := e.find(first)
	require.Equal(t, finds.StateInLibrary, placed.State)
	require.NotNil(t, placed.FoodID)
	require.NotNil(t, placed.PlacedOn)
	assert.Equal(t, e.start.Format(time.DateOnly), *placed.PlacedOn)
	assert.True(t, e.eaten(*placed.FoodID), "the oldest ready find is on the plan")
	recipe, err := sqlite.NewFoodRepo(e.db).Get(e.ctx, *placed.FoodID)
	require.NoError(t, err)
	assert.Equal(t, "Dal", recipe.Name)
	assert.Contains(t, recipe.Tags, planning.ExploreTag)

	left := e.find(second)
	assert.Equal(t, finds.StateReady, left.State, "the quota is 1: the other stays ready")
	assert.Nil(t, left.FoodID)
	var recipes int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM foods WHERE name = 'Lentil stew'`).Scan(&recipes))
	assert.Zero(t, recipes, "an unplaced find leaves no recipe")

	list, more, err := e.finds.Activities(e.ctx, e.start, 5)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Zero(t, more)
	assert.Equal(t, finds.Activity{Kind: finds.ActivityKindFindPlaced, FindID: first, FoodID: *placed.FoodID, Name: "Dal"}, list[0])

	// PRO-5 Undo: the find leaves the drafts and the library and is ready.
	require.NoError(t, e.finds.UndoPlaced(e.ctx, first))
	assert.False(t, e.eaten(*placed.FoodID))
	back := e.find(first)
	assert.Equal(t, finds.StateReady, back.State)
	assert.Nil(t, back.PlacedOn)
	_, err = sqlite.NewFoodRepo(e.db).Get(e.ctx, *placed.FoodID)
	assert.Error(t, err, "the recipe is gone")
}

// ON-1: with the Chef off a generation places no find.
func TestGenerate_ChefOffPlacesNoFind(t *testing.T) {
	e := newFindsEnv(t, false)
	id := e.readyFind("Dal")
	e.generate(e.req())
	assert.Equal(t, finds.StateReady, e.find(id).State)
}
