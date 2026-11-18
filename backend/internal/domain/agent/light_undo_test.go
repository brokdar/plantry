package agent_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// A saved light request is part of the change: Undo puts the dates' stored
// light back (here: none before, one day intent after), and refuses with
// change_stale when the rows were edited since.
func TestChange_UndoRestoresLight(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.freshMain()
	date := p.DateString()
	store := sqlite.NewPlanLightRepo(e.db)
	today := planToday
	stored := func() []planning.LightIntent {
		got, err := store.List(context.Background(), planStart, planStart.AddDate(0, 0, 6))
		require.NoError(t, err)
		return got
	}

	// Existing stored light on the date: a dinner meal request.
	require.NoError(t, store.Replace(context.Background(), today, []string{date}, []planning.LightIntent{{Date: date, Scope: "meal", Slot: "dinner"}}))
	input := fmt.Sprintf(`{"light":[{"date":%q,"scope":"day"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]}`, date, p.ID)
	ch := e.change(ctx, "apply_changes", input)
	require.NotNil(t, ch.Light)
	assert.True(t, ch.Undoable())
	assert.ElementsMatch(t, []planning.LightIntent{{Date: date, Scope: "meal", Slot: "dinner"}, {Date: date, Scope: "day"}}, stored())

	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{{Date: date, Scope: "meal", Slot: "dinner"}}, stored(), "the earlier request is back")
}

func TestChange_UndoLightStale(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.freshMain()
	date := p.DateString()
	store := sqlite.NewPlanLightRepo(e.db)
	input := fmt.Sprintf(`{"light":[{"date":%q,"scope":"day"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]}`, date, p.ID)
	ch := e.change(ctx, "apply_changes", input)
	require.NotNil(t, ch.Light)

	require.NoError(t, store.Replace(context.Background(), planToday, []string{date}, nil))
	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.ErrorIs(t, err, domain.ErrAIChangeStale)
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.InDelta(t, 2.0, got.Eaters, 1e-9, "nothing was undone")
}

// Undo prunes from the real today, not from the change's earliest date: an
// untouched light row on an earlier day that is not past must survive.
func TestChange_UndoKeepsUntouchedEarlierLight(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.freshMain()
	date := p.DateString()
	today := planToday.Format(time.DateOnly)
	require.Less(t, today, date)
	store := sqlite.NewPlanLightRepo(e.db)
	untouched := planning.LightIntent{Date: today, Scope: "meal", Slot: "dinner"}
	require.NoError(t, store.Replace(context.Background(), planToday, []string{today}, []planning.LightIntent{untouched}))

	input := fmt.Sprintf(`{"light":[{"date":%q,"scope":"day"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]}`, date, p.ID)
	ch := e.change(ctx, "apply_changes", input)
	require.NotNil(t, ch.Light)
	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)

	got, err := store.List(context.Background(), planToday, planStart.AddDate(0, 0, 6))
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{untouched}, got)
}
