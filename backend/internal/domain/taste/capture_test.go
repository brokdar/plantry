package taste_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// fakeAttrs serves fixed attributes per food (the real ones are derived from
// ingredients, which these tests do not need).
type fakeAttrs map[int64]flavor.Attributes

func (f fakeAttrs) Get(_ context.Context, id int64) (flavor.View, error) {
	return flavor.View{FoodID: id, Resolved: flavor.Resolved{Values: f[id]}}, nil
}

type rig struct {
	t      *testing.T
	ctx    context.Context
	db     *sql.DB
	now    time.Time
	svc    *taste.Service
	events *foodevent.Service
	store  *sqlite.TasteRepo
	plates *plate.Service
	fb     *feedback.Service
	foods  *food.Service
	dinner int64
	curry  int64 // main: chicken, thai, bowl
	salmon int64 // main: salmon, french, plate
	salad  int64 // side: coriander, thai, salad
}

func newRig(t *testing.T, db *sql.DB) *rig {
	t.Helper()
	r := &rig{t: t, ctx: context.Background(), db: db, now: time.Date(2026, 3, 2, 18, 0, 0, 0, time.UTC), store: sqlite.NewTasteRepo(db)}
	r.dinner = r.id(`SELECT id FROM time_slots WHERE name_key = 'slot.dinner'`)
	r.curry = r.id(`SELECT id FROM foods WHERE name = 'Thai curry'`)
	r.salmon = r.id(`SELECT id FROM foods WHERE name = 'Salmon bake'`)
	r.salad = r.id(`SELECT id FROM foods WHERE name = 'Coriander salad'`)
	clock := func() time.Time { return r.now }
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	r.events = foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, clock)
	r.svc = taste.NewService(taste.Deps{
		Store: r.store, Foods: sqlite.NewFoodRepo(db), Events: r.events, History: sqlite.NewFoodEventRepo(db), Now: clock,
		Attributes: fakeAttrs{
			r.curry:  {Heroes: []string{"Chicken"}, Cuisine: "thai", Format: "bowl"},
			r.salmon: {Heroes: []string{"Salmon"}, Cuisine: "french", Format: "plate"},
			r.salad:  {Heroes: []string{"Coriander"}, Cuisine: "thai", Format: "salad"},
		},
	})
	restorers[foodevent.KindTasteItem] = r.svc
	plates, _ := testsvc.PlanAt(db, nil, clock)
	r.plates = plates.WithTaste(r.svc)
	r.fb = testsvc.Feedback(db).WithTaste(r.svc)
	r.foods = food.NewService(sqlite.NewFoodRepo(db)).WithEvents(r.events).WithFavourites(r.svc)
	return r
}

// seed is a migrated database with a dinner slot and three recipes.
func seed(t *testing.T) *sql.DB {
	t.Helper()
	db := testhelper.NewTestDB(t)
	for _, q := range []string{
		`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`,
		`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Thai curry', 'composed', 'main', 1)`,
		`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Salmon bake', 'composed', 'main', 1)`,
		`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Coriander salad', 'composed', 'side_veg', 1)`,
		`UPDATE user_profile SET household_portions = 1 WHERE id = 1`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
	return db
}

func (r *rig) id(q string) int64 {
	r.t.Helper()
	var id int64
	require.NoError(r.t, r.db.QueryRow(q).Scan(&id), q)
	return id
}

func (r *rig) item(level taste.Level, key string) (taste.Item, bool) {
	r.t.Helper()
	it, ok, err := r.store.ItemByRef(r.ctx, taste.Ref{Level: level, Key: key}, taste.ScopeAlways, time.Time{})
	require.NoError(r.t, err)
	return it, ok
}

func (r *rig) signals(kind taste.SignalKind) int {
	r.t.Helper()
	var n int
	require.NoError(r.t, r.db.QueryRow(`SELECT COUNT(*) FROM taste_signals WHERE kind = ?`, string(kind)).Scan(&n))
	return n
}

func (r *rig) plate(ctx context.Context, day int, foods ...int64) *plate.Plate {
	r.t.Helper()
	in := plate.PlateInput{Date: time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC), SlotID: r.dinner}
	for _, id := range foods {
		one := 1.0
		in.Components = append(in.Components, plate.PlateComponent{FoodID: id, Portions: &one})
	}
	p, err := r.plates.Create(ctx, in)
	require.NoError(r.t, err)
	return p
}

// TASTE-2: plate events teach only when the user made them: a plate added
// by hand counts for its recipes, a recipe the user removed from a draft
// against them; the assistant's and the generator's plates teach nothing,
// nor does removing a confirmed plate or a quiet (tip) change.
func TestCapture_PlateEventsByActor(t *testing.T) {
	r := newRig(t, seed(t))
	agent := actor.With(r.ctx, actor.Agent, 0)

	r.plate(r.ctx, 3, r.curry, r.salad)
	chicken, ok := r.item(taste.LevelHero, "chicken")
	require.True(t, ok)
	assert.InDelta(t, taste.WeightPlateAdded*taste.MainShare*taste.HeroShare, chicken.Evidence.Pos, 1e-9, "the main takes 70 %")
	coriander, _ := r.item(taste.LevelHero, "coriander")
	assert.InDelta(t, taste.WeightPlateAdded*taste.SidesShare*taste.HeroShare, coriander.Evidence.Pos, 1e-9, "the side 30 %")

	draft := r.plate(agent, 4, r.salmon)
	assert.Equal(t, plate.StateDraft, draft.State)
	_, ok = r.item(taste.LevelHero, "salmon")
	assert.False(t, ok, "the assistant's plate teaches nothing")

	_, err := r.plates.Delete(r.ctx, draft.ID)
	require.NoError(t, err)
	salmon, ok := r.item(taste.LevelHero, "salmon")
	require.True(t, ok)
	assert.InDelta(t, taste.WeightPlateRemoved*taste.HeroShare, salmon.Evidence.Neg, 1e-9, "removed from a draft")

	own := r.plate(r.ctx, 5, r.salmon)
	_, err = r.plates.Delete(r.ctx, own.ID)
	require.NoError(t, err)
	salmon, _ = r.item(taste.LevelHero, "salmon")
	assert.InDelta(t, taste.WeightPlateRemoved*taste.HeroShare, salmon.Evidence.Neg, 1e-9, "a confirmed plate removed teaches nothing")

	before := r.signals(taste.SignalPlateAdded)
	r.plate(plate.WithoutTaste(r.ctx), 6, r.curry)
	assert.Equal(t, before, r.signals(taste.SignalPlateAdded), "a quiet change teaches nothing")

	var never int
	require.NoError(t, r.db.QueryRow(`SELECT COUNT(*) FROM never_items`).Scan(&never))
	assert.Zero(t, never, "no signal makes a never item (TASTE-7)")
}

// TASTE-2: ratings, favourites and tips.
func TestCapture_RatingsFavouritesTips(t *testing.T) {
	r := newRig(t, seed(t))
	p := r.plate(r.ctx, 3, r.salmon)

	_, err := r.fb.RecordFeedback(r.ctx, p.ID, feedback.StatusLoved, nil)
	require.NoError(t, err)
	_, err = r.fb.RecordFeedback(r.ctx, p.ID, feedback.StatusLoved, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, r.signals(taste.SignalLoved), "one rating (hero, cuisine, format), not two")
	_, err = r.fb.RecordFeedback(r.ctx, p.ID, feedback.StatusNotEaten, nil)
	require.NoError(t, err)
	_, err = r.fb.RecordFeedback(r.ctx, p.ID, feedback.StatusDisliked, nil)
	require.NoError(t, err)
	salmon, _ := r.item(taste.LevelHero, "salmon")
	assert.InDelta(t, taste.WeightDisliked*taste.HeroShare, salmon.Evidence.Neg, 1e-9)

	for _, on := range []bool{true, false, true} {
		_, err := r.foods.SetFavorite(r.ctx, r.curry, on)
		require.NoError(t, err)
	}
	chicken, _ := r.item(taste.LevelHero, "chicken")
	assert.InDelta(t, taste.WeightFavourite*taste.HeroShare, chicken.Evidence.Pos, 1e-9,
		"one favourite: unstarring and starring again add nothing")

	require.NoError(t, r.svc.Tip(r.ctx, "fresh", true))
	require.NoError(t, r.svc.Tip(r.ctx, "fresh", false))
	fresh, _ := r.item(taste.LevelFlavour, "fresh")
	assert.InDelta(t, taste.WeightTipApplied, fresh.Evidence.Pos, 1e-9)
	assert.InDelta(t, taste.WeightTipDismissed, fresh.Evidence.Neg, 1e-9)
}

// The C2 hooks: a review swap and Not for me by reason.
func TestCapture_ReviewAndNotForMe(t *testing.T) {
	r := newRig(t, seed(t))
	require.NoError(t, r.svc.ReviewSwap(r.ctx, r.salmon, r.curry))
	salmon, _ := r.item(taste.LevelHero, "salmon")
	assert.InDelta(t, taste.WeightReviewRemoved*taste.HeroShare, salmon.Evidence.Neg, 1e-9)
	chicken, _ := r.item(taste.LevelHero, "chicken")
	assert.InDelta(t, taste.WeightReviewAdded*taste.HeroShare, chicken.Evidence.Pos, 1e-9)

	require.NoError(t, r.svc.NotForMe(r.ctx, r.salad, taste.ReasonTooLong, nil))
	_, ok := r.item(taste.LevelHero, "coriander")
	assert.False(t, ok, "too long is time, not taste")
	require.NoError(t, r.svc.NotForMe(r.ctx, r.salad, taste.ReasonIngredients, nil))
	coriander, _ := r.item(taste.LevelHero, "coriander")
	assert.InDelta(t, taste.WeightNotForMe, coriander.Evidence.Neg, 1e-9, "the recipe's only hero takes it all")
	thai, _ := r.item(taste.LevelCuisine, "thai")
	assert.Zero(t, thai.Evidence.Neg, "ingredients blame no cuisine")
}

// ── Simulated weeks ─────────────────────────────────────────────────────

// weeksFixture is a database after simulatedWeeks weeks of signals: every
// week the household loves a Thai curry, dislikes a coriander salad side
// with salmon, and once a month adds a curry by hand. Built once per
// process; each test copies it.
var weeksFixture = struct {
	once sync.Once
	snap []byte
	end  time.Time
	// afterOne is GET /api/taste after the first week.
	afterOne taste.View
	err      error
}{}

const simulatedWeeks = 8

func simulatedDB(t *testing.T) (*rig, taste.View) {
	t.Helper()
	weeksFixture.once.Do(func() {
		db := seed(t)
		r := newRig(t, db)
		for w := range simulatedWeeks {
			r.now = time.Date(2026, 3, 2+7*w, 19, 0, 0, 0, time.UTC)
			curry := &plate.Plate{ID: int64(100 + w), Components: []plate.PlateComponent{{FoodID: r.curry, Portions: ptr(1), Position: plate.PositionMain}}}
			weeksFixture.err = r.svc.Rating(r.ctx, curry, "", "loved")
			if weeksFixture.err != nil {
				return
			}
			fish := &plate.Plate{ID: int64(200 + w), Components: []plate.PlateComponent{
				{FoodID: r.salmon, Portions: ptr(1), Position: plate.PositionMain},
				{FoodID: r.salad, Portions: ptr(1), Position: plate.PositionSide},
			}}
			if weeksFixture.err = r.svc.Rating(r.ctx, fish, "", "disliked"); weeksFixture.err != nil {
				return
			}
			if w == 0 {
				if weeksFixture.afterOne, weeksFixture.err = r.svc.View(r.ctx, time.Time{}); weeksFixture.err != nil {
					return
				}
			}
		}
		weeksFixture.end = r.now
		weeksFixture.snap = testhelper.SnapshotDB(t, db)
	})
	require.NoError(t, weeksFixture.err)
	require.NotNil(t, weeksFixture.snap)
	r := newRig(t, testhelper.NewTestDBFrom(t, weeksFixture.snap))
	r.now = weeksFixture.end
	return r, weeksFixture.afterOne
}

func ptr(f float64) *float64 { return &f }

func polarities(v taste.View) map[taste.Ref]taste.Polarity {
	out := map[taste.Ref]taste.Polarity{}
	for _, it := range v.Items {
		out[taste.Ref{Level: it.Level, Key: it.Key}] = it.Polarity
	}
	return out
}

// After one week nothing leans yet; after eight the household loves chicken
// and bowls and would rather not have salmon or coriander, while Thai (loved
// curries, disliked Thai sides) stays neutral. A year later the likes have
// faded and the dislikes, which fade slower, still hold.
func TestSimulatedWeeks_LearnedItems(t *testing.T) {
	r, afterOne := simulatedDB(t)
	assert.Empty(t, afterOne.Items, "one week of signals is not enough")

	v, err := r.svc.View(r.ctx, time.Time{})
	require.NoError(t, err)
	got := polarities(v)
	assert.Equal(t, taste.Love, got[taste.Ref{Level: taste.LevelHero, Key: "chicken"}])
	assert.Equal(t, taste.Love, got[taste.Ref{Level: taste.LevelFormat, Key: "bowl"}])
	assert.Equal(t, taste.RatherNot, got[taste.Ref{Level: taste.LevelHero, Key: "salmon"}])
	assert.Equal(t, taste.RatherNot, got[taste.Ref{Level: taste.LevelHero, Key: "coriander"}])
	assert.NotContains(t, got, taste.Ref{Level: taste.LevelCuisine, Key: "thai"}, "disliked Thai sides hold the loved curries back")
	for _, it := range v.Items {
		assert.Equal(t, taste.SourceLearned, it.Source)
	}

	// A year without signals: the likes fade first.
	r.now = r.now.AddDate(1, 0, 0)
	v, err = r.svc.View(r.ctx, time.Time{})
	require.NoError(t, err)
	got = polarities(v)
	assert.NotContains(t, got, taste.Ref{Level: taste.LevelHero, Key: "chicken"})
	assert.Equal(t, taste.RatherNot, got[taste.Ref{Level: taste.LevelHero, Key: "salmon"}])
}

// "Recently learned" lists what leaned in the last RecentDays, with an
// Undo: reverting it puts the item back as it was before it leaned, and it
// leaves the list. Weeks-old learning is not recent.
func TestSimulatedWeeks_UndoLearned(t *testing.T) {
	r, _ := simulatedDB(t)
	v, err := r.svc.View(r.ctx, time.Time{})
	require.NoError(t, err)
	keys := func(v taste.View) []string {
		var out []string
		for _, l := range v.Recent {
			out = append(out, l.Item.Key)
		}
		return out
	}
	assert.Equal(t, []string{"bowl"}, keys(v), "bowls leaned two weeks ago; chicken and salmon in the first weeks")
	for range 6 { // this week: six crunch tips applied
		require.NoError(t, r.svc.Tip(r.ctx, "crunch", true))
	}
	v, err = r.svc.View(r.ctx, time.Time{})
	require.NoError(t, err)
	var learned *taste.Learned
	for i := range v.Recent {
		if v.Recent[i].Item.Key == "crunch" {
			learned = &v.Recent[i]
		}
	}
	require.NotNil(t, learned)
	_, err = r.events.Revert(r.ctx, learned.Undo)
	require.NoError(t, err)
	v, err = r.svc.View(r.ctx, time.Time{})
	require.NoError(t, err)
	assert.NotContains(t, polarities(v), taste.Ref{Level: taste.LevelFlavour, Key: "crunch"})
	assert.Equal(t, []string{"bowl"}, keys(v))
}

// AC-13 / TASTE-5: a statement flips a learned item; the user can flip and
// remove items, each with an Undo.
func TestSimulatedWeeks_StatementFlipRemove(t *testing.T) {
	r, _ := simulatedDB(t)
	salmon := taste.Ref{Level: taste.LevelHero, Key: "salmon"}
	it, _, err := r.svc.State(r.ctx, salmon, taste.Love, taste.ScopeAlways, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, taste.SourceStatement, it.Source)
	v, err := r.svc.View(r.ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, taste.Love, polarities(v)[salmon], "the statement flips the learned dislike")

	chicken, _ := r.item(taste.LevelHero, "chicken")
	view, undo, err := r.svc.SetPolarity(r.ctx, chicken.ID, taste.RatherNot)
	require.NoError(t, err)
	assert.Equal(t, taste.RatherNot, view.Polarity)
	assert.Equal(t, taste.SourceUser, view.Source)
	_, err = r.events.Revert(r.ctx, undo)
	require.NoError(t, err)
	chicken, _ = r.item(taste.LevelHero, "chicken")
	assert.Equal(t, taste.SourceLearned, chicken.Source)
	assert.Equal(t, taste.Love, chicken.Effective(r.now))

	undo, err = r.svc.Remove(r.ctx, chicken.ID)
	require.NoError(t, err)
	_, ok := r.item(taste.LevelHero, "chicken")
	assert.False(t, ok)
	_, err = r.events.Revert(r.ctx, undo)
	require.NoError(t, err)
	back, ok := r.item(taste.LevelHero, "chicken")
	require.True(t, ok)
	assert.Equal(t, chicken.ID, back.ID)
	assert.InDelta(t, chicken.Evidence.Pos, back.Evidence.Pos, 1e-9)
}

// The planner reads the learned taste: the builder scores every recipe from
// the items that lean (the loved curry up, the disliked salmon bake down).
func TestSimulatedWeeks_PlannerReadsTaste(t *testing.T) {
	r, _ := simulatedDB(t)
	for id, values := range map[int64]string{
		r.curry:  `{"heroes":["Chicken"],"cuisine":"thai","format":"bowl"}`,
		r.salmon: `{"heroes":["Salmon"],"cuisine":"french","format":"plate"}`,
	} {
		_, err := r.db.Exec(`INSERT INTO food_attributes (food_id, version, values_json, sources_json) VALUES (?, 1, ?, '{"heroes":"user","cuisine":"user","format":"user"}')`, id, values)
		require.NoError(t, err)
	}
	_, avail := testsvc.Plan(r.db)
	deps := testsvc.BuilderDeps(r.db, avail)
	deps.Taste = r.svc
	start := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	w := window.Window{Start: start, End: start.AddDate(0, 0, 6)}
	s, err := planning.NewBuilder(deps).Build(r.ctx, w, start, w.Start, w.End)
	require.NoError(t, err)
	assert.Greater(t, s.Foods[r.curry].Taste, 0.0)
	assert.Less(t, s.Foods[r.salmon].Taste, 0.0)
	assert.Zero(t, s.Foods[r.salad].Taste, "no stored attributes: nothing to score")
}
