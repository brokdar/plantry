package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// State, run and reason round-trip on plates and sessions; SetState and
// the by-id lists read and write them in one statement each.
func TestPlanStateRepos(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	slotID := seedSlot(t, db)
	chili := seedComposed(t, db, "Chili")
	_, err := db.Exec(`INSERT INTO ai_runs (id, kind, started_at) VALUES (7, 'generate', '2026-11-01 10:00:00')`)
	require.NoError(t, err)
	plates, sessions := NewPlateRepo(db), NewCookSessionRepo(db)
	sun := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	run, why := int64(7), "uses the spinach"

	s := &plate.CookSession{FoodID: chili, CookDate: sun, CookSlotID: slotID, Portions: 4, State: plate.StateDraft, AIRunID: &run, Reason: &why}
	require.NoError(t, sessions.CreateSession(ctx, s))
	one := 1.0
	p1 := &plate.Plate{
		Date: sun, SlotID: slotID, Eaters: 2, State: plate.StateDraft, AIRunID: &run, Reason: &why,
		Components: []plate.PlateComponent{{FoodID: chili, Portions: &one, CookSessionID: &s.ID}},
	}
	p2 := &plate.Plate{Date: sun.AddDate(0, 0, 1), SlotID: slotID, Eaters: 2, State: plate.StateConfirmed}
	require.NoError(t, plates.Create(ctx, p1))
	require.NoError(t, plates.Create(ctx, p2))
	assert.Equal(t, plate.StateDraft, p1.State)
	assert.Equal(t, &run, p1.AIRunID)
	assert.Equal(t, &why, p1.Reason)

	got, err := plates.ListByIDs(ctx, []int64{p2.ID, p1.ID, 999})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, p1.ID, got[0].ID, "date order")
	assert.Equal(t, plate.StateDraft, got[0].State)
	assert.Equal(t, &why, got[0].Reason)
	require.Len(t, got[0].Components, 1)
	assert.Equal(t, "Chili", got[0].Components[0].FoodName)
	assert.Equal(t, plate.StateConfirmed, got[1].State)
	assert.Nil(t, got[1].AIRunID)
	assert.Empty(t, got[1].Components)
	none, err := plates.ListByIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	require.NoError(t, plates.SetState(ctx, []int64{p1.ID, p2.ID}, plate.StateConfirmed))
	require.NoError(t, plates.SetState(ctx, nil, plate.StateDraft))
	byDate, err := plates.ListByDateRange(ctx, sun, sun.AddDate(0, 0, 1))
	require.NoError(t, err)
	for _, p := range byDate {
		assert.Equal(t, plate.StateConfirmed, p.State)
	}
	assert.Equal(t, &run, byDate[0].AIRunID, "SetState keeps the run")

	gotS, err := sessions.ListSessionsByIDs(ctx, []int64{s.ID, 999})
	require.NoError(t, err)
	require.Len(t, gotS, 1)
	assert.Equal(t, plate.StateDraft, gotS[0].State)
	assert.Equal(t, &run, gotS[0].AIRunID)
	assert.Equal(t, &why, gotS[0].Reason)
	require.NoError(t, sessions.SetSessionsState(ctx, []int64{s.ID}, plate.StateConfirmed))
	back, err := sessions.GetSession(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, plate.StateConfirmed, back.State)

	// The CHECK rejects an unknown state.
	assert.Error(t, plates.SetState(ctx, []int64{p1.ID}, "locked"))
	assert.Error(t, sessions.SetSessionsState(ctx, []int64{s.ID}, "locked"))
}
