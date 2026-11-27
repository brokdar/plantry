package finds_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// sessionRig is a find service with a settable clock that counts the finds
// it queues (each would be a capped run).
type sessionRig struct {
	svc    *finds.Service
	store  *sqlite.FindRepo
	now    time.Time
	dish   string
	queued []string
}

func newSessionRig(t *testing.T, chefOn bool, newDishes int) *sessionRig {
	t.Helper()
	r := &sessionRig{store: sqlite.NewFindRepo(testhelper.NewTestDB(t)), now: time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC), dish: "thai"}
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
			return finds.Household{Kitchen: profile.Kitchen{MinutesWeekday: 30, MinutesWeekend: 60}}, nil
		},
		Planning: finds.Planning{
			ChefOn:    func(context.Context) (bool, error) { return chefOn, nil },
			NewDishes: func(context.Context) (int, error) { return newDishes, nil },
			Dish:      func(context.Context, time.Time) (string, error) { return r.dish, nil },
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
	r.dish = ""
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

// CP3b item 2: the finds of consecutive weeks look for different mains.
func TestThemeSet_DishesVaryByWeek(t *testing.T) {
	r := newSessionRig(t, true, 1)
	require.NoError(t, r.svc.ThemeSet(context.Background(), "japanese", 1))
	r.now = r.now.AddDate(0, 0, 7)
	require.NoError(t, r.svc.ThemeSet(context.Background(), "japanese", 1))
	require.Len(t, r.queued, 2)
	assert.NotEqual(t, r.queued[0], r.queued[1])
}
