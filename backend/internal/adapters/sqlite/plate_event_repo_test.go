package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestPlateEventRepo_AppendListRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewPlateEventRepo(db)
	at := time.Date(2026, 11, 2, 18, 30, 5, 0, time.UTC)
	date := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	plateID, sessionID, runID := int64(7), int64(3), int64(42)

	full := plate.Event{
		PlateID: &plateID, CookSessionID: &sessionID, Date: date, SlotID: 2,
		Action: plate.ActionUpdate, Actor: actor.Agent, AIRunID: &runID,
		Before: json.RawMessage(`{"eaters":2}`), After: json.RawMessage(`{"eaters":3}`), At: at,
	}
	// A session event: no plate, no run, nothing after.
	bare := plate.Event{
		CookSessionID: &sessionID, Date: date, SlotID: 2,
		Action: plate.ActionDelete, Actor: actor.User,
		Before: json.RawMessage(`{"id":3}`), At: at,
	}
	require.NoError(t, repo.Append(ctx, full))
	require.NoError(t, repo.Append(ctx, bare))

	got, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	full.ID, bare.ID = got[0].ID, got[1].ID
	assert.NotZero(t, full.ID)
	assert.Equal(t, full, got[0])
	assert.Equal(t, bare, got[1])
	assert.Nil(t, got[1].PlateID)
	assert.Nil(t, got[1].AIRunID)
	assert.Nil(t, got[1].After)
}

func TestPlateEventRepo_JoinsUnitOfWork(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewPlateEventRepo(db)
	boom := errors.New("boom")

	err := sqlite.NewTxRunner(db).InTx(ctx, func(ctx context.Context) error {
		require.NoError(t, repo.Append(ctx, plate.Event{Date: time.Now(), SlotID: 1, Action: plate.ActionCreate, Actor: actor.User, At: time.Now()}))
		return boom
	})
	require.ErrorIs(t, err, boom)
	got, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, got, "the rollback takes the event with it")
}
