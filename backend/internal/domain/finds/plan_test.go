package finds_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// sessionRig is a find service with a settable clock that counts the finds
// it queues (each would be a capped run).
type sessionRig struct {
	svc    *finds.Service
	store  *sqlite.FindRepo
	now    time.Time
	queued []string
}

func newSessionRig(t *testing.T, chefOn bool, newDishes int) *sessionRig {
	t.Helper()
	r := &sessionRig{store: sqlite.NewFindRepo(testhelper.NewTestDB(t)), now: time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)}
	r.svc = finds.NewService(finds.Deps{
		Store: r.store,
		Enqueue: func(_ context.Context, id int64, requested bool) error {
			f, err := r.store.Get(context.Background(), id)
			require.NoError(t, err)
			require.False(t, requested, "planning finds are autonomous")
			r.queued = append(r.queued, f.Dish)
			return nil
		},
		Planning: finds.Planning{
			ChefOn:    func(context.Context) (bool, error) { return chefOn, nil },
			NewDishes: func(context.Context) (int, error) { return newDishes, nil },
			Dish:      func(context.Context, time.Time) (string, error) { return "thai", nil },
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
	assert.Equal(t, []string{"thai"}, r.queued, "2 new dishes, 1 ready: 1 search")
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

// FIND-1d: a theme starts finds outside a session; with the Chef off
// nothing opens a session or starts a find (ON-1).
func TestPlanningSession_ThemeAndChefOff(t *testing.T) {
	ctx := context.Background()
	r := newSessionRig(t, true, 1)
	require.NoError(t, r.svc.ThemeSet(ctx, "japanese", 2))
	assert.Equal(t, []string{"japanese", "japanese"}, r.queued)

	off := newSessionRig(t, false, 2)
	require.NoError(t, off.svc.Touch(ctx))
	require.NoError(t, off.svc.Opened(ctx, nextWindow, true, true))
	require.NoError(t, off.svc.ThemeSet(ctx, "japanese", 2))
	open, err := off.svc.InSession(ctx)
	require.NoError(t, err)
	assert.False(t, open)
	assert.Empty(t, off.queued)
}
