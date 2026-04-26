package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
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

// seedWeek inserts a week row for (year, weekNum) and returns its id.
func seedWeek(t *testing.T, db *sql.DB, year, weekNum int) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO weeks (year, week_number) VALUES (?, ?)`, year, weekNum)
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

	// Create two weeks: 2025-W1 (starts 2024-12-30) and 2025-W2 (starts 2026-01-06).
	// Use weeks that are easy to reason about.
	// 2026-W17: Mon=2026-04-20. day 0=Mon, day 1=Tue.
	// 2026-W18: Mon=2026-04-27. day 0=Mon.
	w17 := seedWeek(t, db, 2026, 17) // Mon 2026-04-20
	w18 := seedWeek(t, db, 2026, 18) // Mon 2026-04-27

	repo := NewPlateRepo(db)

	// Plate A: 2026-04-20 (week 17, day 0 = Monday), slotID
	pA := &plate.Plate{WeekID: w17, Day: 0, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, pA))

	// Plate B: 2026-04-21 (week 17, day 1 = Tuesday), slotID
	pB := &plate.Plate{WeekID: w17, Day: 1, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, pB))

	// Plate C: 2026-04-27 (week 18, day 0 = Monday), slotID
	pC := &plate.Plate{WeekID: w18, Day: 0, SlotID: slotID}
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

// TestPlatesRepo_CreatePlate_WritesDateAndLegacy verifies that creating a plate
// via WeekID+Day correctly writes the derived date string and legacy columns.
// 2025-06-15 is a Sunday in ISO week 24 of 2025 (Mon 2025-06-09 … Sun 2025-06-15).
func TestPlatesRepo_CreatePlate_WritesDateAndLegacy(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	// ISO 2025-W24: Mon 2025-06-09. Sunday is day 6.
	target := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	year, week := target.ISOWeek()         // 2025, 24
	day := (int(target.Weekday()) + 6) % 7 // (0+6)%7 = 6

	wID := seedWeek(t, db, year, week)
	slotID := seedSlot(t, db)

	repo := NewPlateRepo(db)
	p := &plate.Plate{WeekID: wID, Day: day, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, p))
	require.NotZero(t, p.ID)

	// Verify via raw SQL that both date and legacy columns are correct.
	var dateStr string
	var dbDay int64
	var dbWeekID int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT date, day, week_id FROM plates WHERE id = ?`, p.ID,
	).Scan(&dateStr, &dbDay, &dbWeekID))

	assert.Equal(t, "2025-06-15", dateStr, "date column")
	assert.Equal(t, int64(6), dbDay, "day column: Sunday=6")
	assert.Equal(t, wID, dbWeekID, "week_id column")

	// Verify the week row has the correct year and ISO week number.
	q := sqlcgen.New(db)
	weekRow, err := q.GetWeek(ctx, dbWeekID)
	require.NoError(t, err)
	assert.Equal(t, int64(2025), weekRow.Year)
	assert.Equal(t, int64(24), weekRow.WeekNumber)
}

// TestPlatesRepo_UpdatePlate_UpdatesLegacy creates a plate and then moves it
// to a new date. Verifies that week_id and day are updated accordingly.
func TestPlatesRepo_UpdatePlate_UpdatesLegacy(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	// Start: 2026-04-20 (Monday, ISO 2026-W17, day=0).
	w17 := seedWeek(t, db, 2026, 17)
	slotID := seedSlot(t, db)

	repo := NewPlateRepo(db)
	p := &plate.Plate{WeekID: w17, Day: 0, SlotID: slotID}
	require.NoError(t, repo.Create(ctx, p))

	// Move to: 2026-04-27 (Monday, ISO 2026-W18, day=0).
	w18 := seedWeek(t, db, 2026, 18)
	p.WeekID = w18
	p.Day = 0
	require.NoError(t, repo.Update(ctx, p))

	var dateStr string
	var dbDay int64
	var dbWeekID int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT date, day, week_id FROM plates WHERE id = ?`, p.ID,
	).Scan(&dateStr, &dbDay, &dbWeekID))

	assert.Equal(t, "2026-04-27", dateStr, "date column after update")
	assert.Equal(t, int64(0), dbDay, "day column after update")
	assert.Equal(t, w18, dbWeekID, "week_id column after update")
}

// TestPlatesRepo_LegacyFromDate_CrossYear verifies the legacyFromDate helper
// for 2025-01-01, which is ISO week 1 of 2025 (Wednesday, day=2).
// ISO week 1 of 2025 starts on Mon 2024-12-30.
func TestPlatesRepo_LegacyFromDate_CrossYear(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	repo := NewPlateRepo(db)

	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	weekID, day, err := repo.legacyFromDate(ctx, date)
	require.NoError(t, err)

	// 2025-01-01 is a Wednesday: day = (Wednesday=3 + 6) % 7 = 2.
	assert.Equal(t, 2, day, "Wednesday = day 2")

	// The week must be ISO 2025-W1.
	q := sqlcgen.New(db)
	weekRow, err := q.GetWeek(ctx, weekID)
	require.NoError(t, err)
	assert.Equal(t, int64(2025), weekRow.Year)
	assert.Equal(t, int64(1), weekRow.WeekNumber)
}

// TestPlatesRepo_LegacyFromDate_CrossYear_UTC verifies that date math is stable
// even when the local timezone is set to America/New_York.
func TestPlatesRepo_LegacyFromDate_CrossYear_UTC(t *testing.T) {
	t.Setenv("TZ", "America/New_York")

	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	repo := NewPlateRepo(db)

	// Same date as the previous test; result must be identical regardless of TZ.
	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	weekID, day, err := repo.legacyFromDate(ctx, date)
	require.NoError(t, err)

	assert.Equal(t, 2, day)

	q := sqlcgen.New(db)
	weekRow, err := q.GetWeek(ctx, weekID)
	require.NoError(t, err)
	assert.Equal(t, int64(2025), weekRow.Year)
	assert.Equal(t, int64(1), weekRow.WeekNumber)
}
