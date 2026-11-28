package finds_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/libreview"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// sessionRig is a find service with a settable clock that counts the finds
// it queues (each would be a capped run).
type sessionRig struct {
	svc      *finds.Service
	store    *sqlite.FindRepo
	gaps     *sqlite.LibraryReviewRepo
	now      time.Time
	cuisines []string
	house    finds.Household
	queued   []string
}

func newSessionRig(t *testing.T, chefOn bool, newDishes int) *sessionRig {
	t.Helper()
	db := testhelper.NewTestDB(t)
	r := &sessionRig{store: sqlite.NewFindRepo(db), gaps: sqlite.NewLibraryReviewRepo(db), now: time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC), cuisines: []string{"thai"}}
	r.house = finds.Household{Kitchen: profile.Kitchen{MinutesWeekday: 30, MinutesWeekend: 60}}
	r.svc = finds.NewService(finds.Deps{
		Store: r.store,
		Enqueue: func(_ context.Context, id int64, requested bool) error {
			f, err := r.store.Get(context.Background(), id)
			require.NoError(t, err)
			require.False(t, requested, "planning finds are autonomous")
			r.queued = append(r.queued, f.Dish)
			return nil
		},
		Household: func(context.Context) (finds.Household, error) {
			return r.house, nil
		},
		Planning: finds.Planning{
			Gaps:      r.gaps,
			ChefOn:    func(context.Context) (bool, error) { return chefOn, nil },
			NewDishes: func(context.Context) (int, error) { return newDishes, nil },
			Cuisines:  func(context.Context) ([]string, error) { return r.cuisines, nil },
			Now:       func() time.Time { return r.now },
		},
	})
	return r
}

var nextWindow = time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)

// AC-9, P11, FIND-2: a week with no planning session queues no find; the
// plan page on a window that is neither a draft nor the next one opens no
// session.
func TestPlanningSession_IdleWeekQueuesNothing(t *testing.T) {
	r := newSessionRig(t, true, 2)
	ctx := context.Background()
	for range 7 * 24 {
		require.NoError(t, r.svc.Prefetch(ctx, nextWindow))
		require.NoError(t, r.svc.Opened(ctx, nextWindow.AddDate(0, 0, -7), false, false))
		r.now = r.now.Add(time.Hour)
	}
	assert.Empty(t, r.queued, "no capped run without a planning session")
}

// FIND-1a, FIND-3: opening the next window opens a session and prefetches
// the new dishes the ready finds do not cover; the session closes 30 min
// after its last trigger.
func TestPlanningSession_NextWindowPrefetches(t *testing.T) {
	r := newSessionRig(t, true, 2)
	ctx := context.Background()
	f, err := r.store.Create(ctx, "curry", finds.TriggerChat)
	require.NoError(t, err)
	_, err = r.store.Finish(ctx, f.ID, finds.Outcome{State: finds.StateReady})
	require.NoError(t, err)

	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	require.Len(t, r.queued, 1, "2 new dishes, 1 ready: 1 search")
	assert.True(t, strings.HasPrefix(r.queued[0], "thai "), r.queued[0])
	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	assert.Len(t, r.queued, 1, "the searching one counts")

	r.now = r.now.Add(finds.SessionLength - time.Minute)
	open, err := r.svc.InSession(ctx)
	require.NoError(t, err)
	assert.True(t, open)
	r.now = r.now.Add(2 * time.Minute)
	open, err = r.svc.InSession(ctx)
	require.NoError(t, err)
	assert.False(t, open, "closed 30 min after the last trigger")

	// A draft window keeps a session open but prefetches nothing by itself.
	require.NoError(t, r.svc.Opened(ctx, nextWindow.AddDate(0, 0, -7), true, false))
	open, err = r.svc.InSession(ctx)
	require.NoError(t, err)
	assert.True(t, open)
	assert.Len(t, r.queued, 1)
}

// FIND-15: a household with no cuisine in scope (no library cuisine, no
// love, no Cuisines to explore, no home cuisine for its locale) has nothing
// to search for: opening the next window starts no find and fails nothing.
func TestPlanningSession_NoCuisineInScopeStartsNoFind(t *testing.T) {
	r := newSessionRig(t, true, 1)
	r.cuisines = nil
	require.NoError(t, r.svc.Opened(context.Background(), nextWindow, false, true))
	assert.Empty(t, r.queued)
}

// FIND-1d: a theme starts finds outside a session; with the Chef off
// nothing opens a session or starts a find (ON-1).
func TestPlanningSession_ThemeAndChefOff(t *testing.T) {
	ctx := context.Background()
	r := newSessionRig(t, true, 1)
	require.NoError(t, r.svc.ThemeSet(ctx, "japanese", 2))
	require.Len(t, r.queued, 2)
	assert.True(t, strings.HasPrefix(r.queued[0], "japanese "), r.queued[0])
	assert.True(t, strings.HasPrefix(r.queued[1], "japanese "), r.queued[1])
	assert.NotEqual(t, r.queued[0], r.queued[1], "two finds of one theme look for different mains")

	off := newSessionRig(t, false, 2)
	require.NoError(t, off.svc.Touch(ctx))
	require.NoError(t, off.svc.Opened(ctx, nextWindow, true, true))
	require.NoError(t, off.svc.ThemeSet(ctx, "japanese", 2))
	open, err := off.svc.InSession(ctx)
	require.NoError(t, err)
	assert.False(t, open)
	assert.Empty(t, off.queued)
}

// readyDraft stores a ready planning find with the draft.
func (r *sessionRig) readyDraft(t *testing.T, d *importer.Draft) {
	t.Helper()
	ctx := context.Background()
	f, err := r.store.Create(ctx, "french main dish", finds.TriggerPlanning)
	require.NoError(t, err)
	_, err = r.store.Finish(ctx, f.ID, finds.Outcome{State: finds.StateReady, Draft: d, SourceURL: "https://example.org/" + d.Name})
	require.NoError(t, err)
}

// CP3b item 1, FIND-3: a ready find that cannot fill a lunch or dinner main
// (a side, or longer than the longest time allowance) stays ready but does
// not hold the window's new searches back.
func TestPrefetch_ReadyFindThatCannotFillAMainDoesNotCount(t *testing.T) {
	r := newSessionRig(t, true, 2)
	r.readyDraft(t, &importer.Draft{Name: "French Onion Soup", Tags: []string{"Dinner", "Side Dish", "Soup"}, PrepMinutes: ptr(10), CookMinutes: ptr(90)})
	r.readyDraft(t, &importer.Draft{Name: "Slow ragu", Tags: []string{"Main Course"}, TotalMinutes: ptr(180)})

	require.NoError(t, r.svc.Opened(context.Background(), nextWindow, false, true))
	require.Len(t, r.queued, 2, "neither ready find fills a main cell: 2 searches")
	assert.NotEqual(t, r.queued[0], r.queued[1])
	for _, d := range r.queued {
		assert.True(t, strings.HasPrefix(d, "thai ") && strings.HasSuffix(d, "main") || d == "thai main dish", d)
	}

	fits := newSessionRig(t, true, 2)
	fits.readyDraft(t, &importer.Draft{Name: "Dal", Tags: []string{"Main Course", "Side Dish"}, TotalMinutes: ptr(45)})
	require.NoError(t, fits.svc.Opened(context.Background(), nextWindow, false, true))
	assert.Len(t, fits.queued, 1, "a fitting ready main counts")
}

// CP3 run 3 item 5: a fitting ready find a generation passed over (it could
// not use it in its window) no longer counts toward New dishes: the next
// window searches a new one.
func TestPrefetch_PassedReadyFindDoesNotCount(t *testing.T) {
	r := newSessionRig(t, true, 1)
	r.readyDraft(t, &importer.Draft{Name: "Roasted Bratwurst", Tags: []string{"Main Course"}, TotalMinutes: ptr(60)})
	ready := finds.StateReady
	list, err := r.store.List(context.Background(), &ready, 5)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NoError(t, r.store.Pass(context.Background(), list[0].ID, nextWindow.AddDate(0, 0, -7)))

	require.NoError(t, r.svc.Opened(context.Background(), nextWindow, false, true))
	assert.Len(t, r.queued, 1, "the passed find stays ready but a new one is searched")
}

// CP3 run 3 item 6: planning finds follow the household's cuisines best
// first (its loves, not the home cuisine at the end), vary by find and by
// week, and ask for meat-free mains when the weekly meat-day limit is tight.
func TestPrefetch_DishesFollowTheHousehold(t *testing.T) {
	r := newSessionRig(t, true, 2)
	r.cuisines = []string{"indian", "italian", "mexican", "chinese", "german"}
	limit := 3
	r.house.MeatDaysMax = &limit
	require.NoError(t, r.svc.Opened(context.Background(), nextWindow, false, true))
	require.Len(t, r.queued, 2)
	assert.NotEqual(t, r.queued[0], r.queued[1])
	for _, d := range r.queued {
		assert.Contains(t, d, " meat-free ", d)
		assert.NotContains(t, d, "german", "the home cuisine comes last")
		assert.NotContains(t, d, "chinese")
	}

	free := newSessionRig(t, true, 1)
	free.cuisines = []string{"indian"}
	limit7 := 7
	free.house.MeatDaysMax = &limit7
	require.NoError(t, free.svc.Opened(context.Background(), nextWindow, false, true))
	require.Len(t, free.queued, 1)
	assert.NotContains(t, free.queued[0], "meat-free", "no tight limit: any main")
	assert.True(t, strings.HasPrefix(free.queued[0], "indian "))
}

// CP3b item 2: the finds of consecutive weeks look for different mains.
func TestThemeSet_DishesVaryByWeek(t *testing.T) {
	r := newSessionRig(t, true, 1)
	require.NoError(t, r.svc.ThemeSet(context.Background(), "japanese", 1))
	r.now = r.now.AddDate(0, 0, 7)
	require.NoError(t, r.svc.ThemeSet(context.Background(), "japanese", 1))
	require.Len(t, r.queued, 2)
	assert.NotEqual(t, r.queued[0], r.queued[1])
}

// LIB-3, FIND-1b: a queued library gap starts one find at the next planning
// session, never outside one and never a second in the same session; it
// leaves the queue when its find succeeds and is tried again in a later
// session when the find failed.
func TestPrefetch_QueuedGapStartsOneFindPerSession(t *testing.T) {
	ctx := context.Background()
	r := newSessionRig(t, true, 0) // no new dishes: only gaps search
	at := r.now.Add(-24 * time.Hour)
	require.NoError(t, r.gaps.QueueGap(ctx, libreview.Gap{Kind: "quick_main", Dish: "a quick weeknight main"}, at))
	require.NoError(t, r.gaps.QueueGap(ctx, libreview.Gap{Kind: "few_mains", Dish: "a family main"}, at.Add(time.Minute)))
	queued := func() []finds.QueuedGap {
		g, err := r.gaps.Gaps(ctx)
		require.NoError(t, err)
		return g
	}

	require.NoError(t, r.svc.Prefetch(ctx, nextWindow))
	assert.Empty(t, r.queued, "no session, no search")

	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	assert.Equal(t, []string{"a quick weeknight main"}, r.queued, "the oldest gap, one find")
	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	r.now = r.now.Add(10 * time.Minute)
	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	assert.Len(t, r.queued, 1, "one gap per planning session while it searches")

	first := queued()[0]
	require.NotZero(t, first.FindID)
	_, err := r.store.Finish(ctx, first.FindID, finds.Outcome{State: finds.StateReady})
	require.NoError(t, err)
	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	assert.Len(t, r.queued, 1, "still the same session: the next gap waits")
	require.Len(t, queued(), 1, "the filled gap left the queue")
	assert.Equal(t, "few_mains", queued()[0].Kind)
	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	assert.Len(t, r.queued, 1, "a later call of the same session still starts nothing")

	r.now = r.now.Add(finds.SessionLength + time.Minute) // the session closed
	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	assert.Equal(t, []string{"a quick weeknight main", "a family main"}, r.queued, "next session: the next gap")

	second := queued()[0]
	_, err = r.store.Finish(ctx, second.FindID, finds.Outcome{State: finds.StateFailed})
	require.NoError(t, err)
	r.now = r.now.Add(finds.SessionLength + time.Minute)
	require.NoError(t, r.svc.Opened(ctx, nextWindow, false, true))
	assert.Equal(t, "a family main", r.queued[2], "a failed find is tried again in a later session")
	assert.Len(t, queued(), 1)

	off := newSessionRig(t, false, 0)
	require.NoError(t, off.gaps.QueueGap(ctx, libreview.Gap{Kind: "quick_main", Dish: "x"}, at))
	require.NoError(t, off.svc.Opened(ctx, nextWindow, true, true))
	assert.Empty(t, off.queued, "the Chef off queues nothing")
}

// refusingLibrary is a library whose recipe save refuses every draft.
type refusingLibrary struct{ finds.Library }

func (refusingLibrary) Leaf(context.Context, food.Candidate, food.Origin) (int64, error) {
	return 1, nil
}

func (refusingLibrary) Finalize(context.Context, importer.FinalizeInput) (*importer.FinalizedFood, error) {
	return nil, fmt.Errorf("%w: amount must be > 0", domain.ErrImportInvalidResolution)
}

// CP4 fix: a ready find the recipe save refuses every time ends failed with
// a reason; it is not offered again each week.
func TestStage_RefusedSaveFailsTheFind(t *testing.T) {
	r := newSessionRig(t, true, 1)
	r.svc = finds.NewService(finds.Deps{
		Store:     r.store,
		Household: func(context.Context) (finds.Household, error) { return r.house, nil },
		Logger:    slog.New(slog.DiscardHandler),
		Planning: finds.Planning{
			Library: refusingLibrary{},
			ChefOn:  func(context.Context) (bool, error) { return true, nil },
			Now:     func() time.Time { return r.now },
		},
	})
	id := int64(1)
	r.readyDraft(t, &importer.Draft{
		Name: "Taco Casserole", Tags: []string{"Main Course"}, TotalMinutes: ptr(40), Instructions: []string{"Bake."},
		Ingredients: []importer.DraftIngredient{{Name: "spray", Match: &food.LineMatch{Status: food.MatchMatched, Pick: 0, Candidates: []food.Candidate{{Name: "spray"}}}}},
	})

	ids, notes, err := r.svc.Stage(context.Background(), 1, nextWindow)
	require.NoError(t, err)
	assert.Empty(t, ids)
	require.Len(t, notes, 1)
	f, err := r.store.Get(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, finds.StateFailed, f.State)
	require.NotEmpty(t, f.Steps)
	assert.Equal(t, "recipe_save_refused", f.Steps[len(f.Steps)-1].Key)
}
