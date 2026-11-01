package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// B7-AC1 (read side): only the user's recipe swaps of the 4 weeks before the
// window come back.
func TestPlateEventRepo_UserReplacements(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewPlateEventRepo(testhelper.NewTestDB(t))
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	plateID := int64(1)
	snap := func(foods ...int64) json.RawMessage {
		cs := make([]plate.SnapshotComponent, len(foods))
		for i, f := range foods {
			cs[i] = plate.SnapshotComponent{FoodID: f, SortOrder: i}
		}
		b, err := json.Marshal(plate.SnapshotPlate{Components: cs})
		require.NoError(t, err)
		return b
	}
	add := func(date string, who actor.Kind, action plate.Action, before, after json.RawMessage) {
		d, err := time.Parse(time.DateOnly, date)
		require.NoError(t, err)
		require.NoError(t, repo.Append(ctx, plate.Event{PlateID: &plateID, Date: d, SlotID: 2, Action: action, Actor: who, Before: before, After: after, At: at}))
	}
	const fish, pasta = 10, 11
	add("2026-10-12", actor.User, plate.ActionUpdate, snap(fish), snap(pasta)) // older than 4 weeks
	add("2026-10-14", actor.User, plate.ActionUpdate, snap(fish), snap(pasta))
	add("2026-10-21", actor.User, plate.ActionUpdate, snap(fish), snap(pasta))
	add("2026-10-28", actor.User, plate.ActionUpdate, snap(fish), snap(pasta))
	add("2026-10-29", actor.Agent, plate.ActionUpdate, snap(fish), snap(pasta)) // not the user
	add("2026-10-30", actor.User, plate.ActionUpdate, snap(fish), snap(fish))   // no swap
	add("2026-10-30", actor.User, plate.ActionDelete, snap(fish), nil)          // a delete is no swap
	add("2026-11-07", actor.User, plate.ActionUpdate, snap(fish), snap(pasta))  // inside the window

	got, err := repo.UserReplacements(ctx, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, got, 4)
	assert.Equal(t, []int64{fish}, got[1].Removed)
	assert.Equal(t, []int64{pasta}, got[1].Added)

	got, err = repo.UserReplacements(ctx, time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Len(t, got, 3)
}
