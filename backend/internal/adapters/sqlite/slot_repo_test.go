package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func newSlotRepo(t *testing.T) *sqlite.SlotRepo {
	t.Helper()
	return sqlite.NewSlotRepo(testhelper.NewTestDB(t))
}

func TestSlotRepo_RoundTrip(t *testing.T) {
	repo := newSlotRepo(t)
	ctx := context.Background()

	s := &slot.TimeSlot{NameKey: "slot.breakfast", Icon: "Coffee", SortOrder: 1, Active: true}
	require.NoError(t, repo.Create(ctx, s))
	assert.NotZero(t, s.ID)

	got, err := repo.Get(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, "slot.breakfast", got.NameKey)
	assert.True(t, got.Active)

	got.Icon = "Sun"
	got.SortOrder = 5
	require.NoError(t, repo.Update(ctx, got))
	reloaded, err := repo.Get(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, "Sun", reloaded.Icon)
	assert.Equal(t, 5, reloaded.SortOrder)

	require.NoError(t, repo.Delete(ctx, s.ID))
	_, err = repo.Get(ctx, s.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestSlotRepo_List(t *testing.T) {
	repo := newSlotRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &slot.TimeSlot{NameKey: "slot.breakfast", Icon: "Coffee", SortOrder: 1, Active: true}))
	require.NoError(t, repo.Create(ctx, &slot.TimeSlot{NameKey: "slot.lunch", Icon: "Sun", SortOrder: 2, Active: false}))
	require.NoError(t, repo.Create(ctx, &slot.TimeSlot{NameKey: "slot.dinner", Icon: "Moon", SortOrder: 3, Active: true}))

	all, err := repo.List(ctx, false)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	active, err := repo.List(ctx, true)
	require.NoError(t, err)
	assert.Len(t, active, 2)
}

func TestSlotRepo_DeleteNotFound(t *testing.T) {
	repo := newSlotRepo(t)
	err := repo.Delete(context.Background(), 9999)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestSlotRepo_CountPlatesUsing_Empty(t *testing.T) {
	repo := newSlotRepo(t)
	count, err := repo.CountPlatesUsing(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}
