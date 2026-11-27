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
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
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
	require.NoError(t, testsvc.Settings(e.db).Set(e.ctx, "chef.new_dishes_per_week", "1"))
	e.lentil = e.insert(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g) VALUES ('Linsen', 'leaf', 'manual', 116, 9, 0.4, 20)`)
	foods := sqlite.NewFoodRepo(e.db)
	e.store = sqlite.NewFindRepo(e.db)
	e.finds = finds.NewService(finds.Deps{
		Store: e.store, Tx: sqlite.NewTxRunner(e.db),
		Enqueue: func(context.Context, int64, bool) error { return nil },
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
	return e.readyFindFrom(name, "https://example.org/"+name)
}

// readyFindFrom is readyFind from the source page url, with tags.
func (e *findsEnv) readyFindFrom(name, url string, tags ...string) int64 {
	e.t.Helper()
	grams := 600.0
	d := &importer.Draft{
		Name: name, ReferencePortions: 4, Instructions: []string{"Cook the lentils."}, Tags: tags,
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
	_, err = e.store.Finish(e.ctx, f.ID, finds.Outcome{State: finds.StateReady, Draft: d, SourceURL: url})
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

// CP3b item 1: a ready find that cannot fill a lunch or dinner main (a side
// dish) stays ready for later, a fitting newer one is placed instead, and the
// run's notes say why the side was not used.
func TestGenerate_ReadyFindThatCannotFillAMainWaits(t *testing.T) {
	e := newFindsEnv(t, true)
	side := e.readyFindFrom("Lentil salad", "https://example.org/lentil-salad", "Side Dish")
	main := e.readyFind("Dal")

	id := e.generate(e.req())

	assert.Equal(t, finds.StateReady, e.find(side).State, "the side waits")
	placed := e.find(main)
	require.Equal(t, finds.StateInLibrary, placed.State)
	assert.True(t, e.eaten(*placed.FoodID))
	assert.Contains(t, e.runs.end(id).Notes, "The ready new dish Lentil salad waits for a later week: it is not a main dish.")
}

// CP3b item 3: of two ready finds from one source page (normalised URL)
// only one is placed.
func TestGenerate_SameSourceReadyFindsPlaceOnce(t *testing.T) {
	e := newFindsEnv(t, true)
	require.NoError(t, testsvc.Settings(e.db).Set(e.ctx, "chef.new_dishes_per_week", "2"))
	first := e.readyFindFrom("Dal", "https://example.org/dal/")
	second := e.readyFindFrom("Dal", "https://example.org/dal?utm_source=x")

	e.generate(e.req())

	assert.Equal(t, finds.StateInLibrary, e.find(first).State)
	assert.Equal(t, finds.StateReady, e.find(second).State, "the same page is not placed twice")
	var recipes int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM foods WHERE name = 'Dal'`).Scan(&recipes))
	assert.Equal(t, 1, recipes)
}

// ON-1: with the Chef off a generation places no find.
func TestGenerate_ChefOffPlacesNoFind(t *testing.T) {
	e := newFindsEnv(t, false)
	id := e.readyFind("Dal")
	e.generate(e.req())
	assert.Equal(t, finds.StateReady, e.find(id).State)
}

type countingSink struct{ calls int }

func (c *countingSink) PlateChanged(context.Context, plate.Action, *plate.Plate, *plate.Plate) error {
	c.calls++
	return nil
}

// C3 review: Undo of a placed find is no taste (no plate_removed dislike),
// and the activity list shows only finds whose recipe still sits on a draft
// plate of the window.
func TestGenerate_UndoTeachesNothingAndActivitiesFollowThePlates(t *testing.T) {
	e := newFindsEnv(t, true)
	first := e.readyFind("Dal")
	e.generate(e.req())
	placed := e.find(first)
	require.NotNil(t, placed.FoodID)

	sink := &countingSink{}
	e.plates.WithTaste(sink)
	require.NoError(t, e.finds.UndoPlaced(e.ctx, first))
	assert.Zero(t, sink.calls, "an Undo is not a hand edit of a meal")

	// A placed find whose plates are gone is no longer listed.
	e.generate(e.req()) // the undone find is ready again and placed again
	again := e.find(first)
	require.Equal(t, finds.StateInLibrary, again.State)
	list, _, err := e.finds.Activities(e.ctx, e.start, 5)
	require.NoError(t, err)
	require.Len(t, list, 1)
	for _, p := range e.window() {
		for _, c := range p.Components {
			if c.FoodID == *again.FoodID {
				_, err := e.plates.RemoveMeals(plate.WithoutTaste(e.ctx), []int64{p.ID})
				require.NoError(t, err)
			}
		}
	}
	list, more, err := e.finds.Activities(e.ctx, e.start, 5)
	require.NoError(t, err)
	assert.Empty(t, list)
	assert.Zero(t, more)
}

// COLD-3: "Fill my library" starts ten requested finds spread over distinct
// dishes; a second tap while they search starts none.
func TestFillLibrary_TenRequestedFinds(t *testing.T) {
	e := newFindsEnv(t, true)
	got, err := e.finds.FillLibrary(e.ctx)
	require.NoError(t, err)
	require.Len(t, got, 10)
	dishes := map[string]bool{}
	for _, f := range got {
		assert.Equal(t, finds.TriggerFill, f.Trigger)
		assert.True(t, f.Trigger.Requested(), "requested work, within the cap")
		dishes[f.Dish] = true
	}
	assert.Len(t, dishes, 10)

	again, err := e.finds.FillLibrary(e.ctx)
	require.NoError(t, err)
	assert.Len(t, again, 10, "the searching ones, no new batch")
	all, err := e.finds.List(e.ctx, nil)
	require.NoError(t, err)
	assert.Len(t, all, 10)
}

// A partial failure leaves fewer than ten fill finds open: the next tap starts
// only the dishes without one.
func TestFillLibrary_TopsUpAPartialBatch(t *testing.T) {
	e := newFindsEnv(t, true)
	got, err := e.finds.FillLibrary(e.ctx)
	require.NoError(t, err)
	require.Len(t, got, 10)
	for _, f := range got[:3] {
		e.finds.Abandon(e.ctx, f.ID, "error")
	}
	again, err := e.finds.FillLibrary(e.ctx)
	require.NoError(t, err)
	assert.Len(t, again, 10)
	dishes := map[string]bool{}
	for _, f := range again {
		dishes[f.Dish] = true
	}
	assert.Len(t, dishes, 10, "ten distinct dishes open")
}
