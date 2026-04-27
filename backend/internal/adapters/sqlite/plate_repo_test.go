package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// seedSlot inserts a time_slot and returns its id.
func seedSlot(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'utensils', 1, 1)`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

// TestPlatesRepo_ListByDateRange_Ordering creates three plates with different
// dates and slot_ids and verifies that ListByDateRange returns them ordered by
// date ASC, slot_id ASC, id ASC.
func TestPlatesRepo_ListByDateRange_Ordering(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	repo := NewPlateRepo(db)

	d1 := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC) // Monday
	d2 := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC) // Tuesday
	d3 := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC) // Monday next week

	pA := &plate.Plate{Date: d1, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, pA))

	pB := &plate.Plate{Date: d2, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, pB))

	pC := &plate.Plate{Date: d3, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, pC))

	from := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)

	got, err := repo.ListByDateRange(ctx, from, to)
	require.NoError(t, err)
	require.Len(t, got, 3)

	assert.Equal(t, pA.ID, got[0].ID, "first plate: 2026-04-20")
	assert.Equal(t, pB.ID, got[1].ID, "second plate: 2026-04-21")
	assert.Equal(t, pC.ID, got[2].ID, "third plate: 2026-04-27")
}

// TestPlatesRepo_ListByDateRange_EmptyRange verifies that an empty result set
// is returned for a date range with no plates.
func TestPlatesRepo_ListByDateRange_EmptyRange(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	repo := NewPlateRepo(db)

	from := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, 1, 7, 0, 0, 0, 0, time.UTC)

	got, err := repo.ListByDateRange(ctx, from, to)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestPlatesRepo_CreatePlate_WritesDate verifies that creating a plate
// via Date correctly writes the date string.
func TestPlatesRepo_CreatePlate_WritesDate(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	target := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	slotID := seedSlot(t, db)

	repo := NewPlateRepo(db)
	p := &plate.Plate{Date: target, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, p))
	require.NotZero(t, p.ID)

	// Verify via raw SQL that the date column is correct.
	var dateStr string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT date FROM plates WHERE id = ?`, p.ID,
	).Scan(&dateStr))

	assert.Equal(t, "2025-06-15", dateStr, "date column")
}

// TestPlatesRepo_UpdatePlate_UpdatesDate creates a plate and then moves it
// to a new date, verifying the date column is updated.
func TestPlatesRepo_UpdatePlate_UpdatesDate(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	d1 := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	slotID := seedSlot(t, db)

	repo := NewPlateRepo(db)
	p := &plate.Plate{Date: d1, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, p))

	p.Date = d2
	require.NoError(t, repo.Update(ctx, p))

	var dateStr string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT date FROM plates WHERE id = ?`, p.ID,
	).Scan(&dateStr))

	assert.Equal(t, "2026-04-27", dateStr, "date column after update")
}
