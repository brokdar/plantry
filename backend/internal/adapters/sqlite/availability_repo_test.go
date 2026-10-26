package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func availDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func insertSlot(t *testing.T, conn *sql.DB, nameKey string, sortOrder int, active bool) int64 {
	t.Helper()
	a := 0
	if active {
		a = 1
	}
	res, err := conn.Exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES (?, 'Coffee', ?, ?)`, nameKey, sortOrder, a)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func insertPreset(t *testing.T, conn *sql.DB, name string) int64 {
	t.Helper()
	res, err := conn.Exec(`INSERT INTO presets (name) VALUES (?)`, name)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func newAvailabilityService(conn *sql.DB) *availability.Service {
	return availability.NewService(sqlite.NewAvailabilityRepo(conn), sqlite.NewTxRunner(conn), sqlite.NewPresetRepo(conn))
}

func TestAvailabilityRepo_DefaultsRoundTrip(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	ctx := context.Background()
	repo := sqlite.NewAvailabilityRepo(conn)
	breakfast := insertSlot(t, conn, "slot.breakfast", 1, true)
	dinner := insertSlot(t, conn, "slot.dinner", 2, true)
	inactive := insertSlot(t, conn, "slot.snack", 3, false)
	oatmeal := insertPreset(t, conn, "Oatmeal")

	_, err := conn.Exec(`INSERT INTO availability_defaults (weekday, slot_id, mode) VALUES (4, ?, 'none')`, inactive)
	require.NoError(t, err)

	require.NoError(t, repo.ReplaceActiveDefaults(ctx, []availability.Default{
		{Weekday: 0, SlotID: breakfast, Mode: availability.ModeRoutine, PresetID: &oatmeal},
		{Weekday: 6, SlotID: dinner, Mode: availability.ModeNone},
	}))
	got, err := repo.ListDefaults(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, breakfast, got[0].SlotID)
	require.NotNil(t, got[0].PresetName)
	assert.Equal(t, "Oatmeal", *got[0].PresetName)
	assert.Equal(t, inactive, got[1].SlotID, "inactive slot row kept")

	one, err := repo.GetDefault(ctx, 6, dinner)
	require.NoError(t, err)
	require.NotNil(t, one)
	assert.Equal(t, availability.ModeNone, one.Mode)
	assert.Nil(t, one.PresetID)
	missing, err := repo.GetDefault(ctx, 1, dinner)
	require.NoError(t, err)
	assert.Nil(t, missing)

	// Replacing again drops the previous active rows.
	require.NoError(t, repo.ReplaceActiveDefaults(ctx, nil))
	got, err = repo.ListDefaults(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, inactive, got[0].SlotID)
}

func TestAvailabilityRepo_OverridesRoundTrip(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	ctx := context.Background()
	repo := sqlite.NewAvailabilityRepo(conn)
	lunch := insertSlot(t, conn, "slot.lunch", 1, true)
	oatmeal := insertPreset(t, conn, "Oatmeal")
	note := "restaurant"

	require.NoError(t, repo.UpsertOverride(ctx, availability.Override{Date: availDate("2026-10-26"), SlotID: lunch, Mode: availability.ModeNone, Note: &note}))
	o, err := repo.GetOverride(ctx, availDate("2026-10-26"), lunch)
	require.NoError(t, err)
	require.NotNil(t, o)
	assert.Equal(t, availDate("2026-10-26"), o.Date)
	assert.Equal(t, availability.ModeNone, o.Mode)
	require.NotNil(t, o.Note)
	assert.Equal(t, "restaurant", *o.Note)

	// Upsert replaces mode, preset and note (note back to null).
	require.NoError(t, repo.UpsertOverride(ctx, availability.Override{Date: availDate("2026-10-26"), SlotID: lunch, Mode: availability.ModeRoutine, PresetID: &oatmeal}))
	o, err = repo.GetOverride(ctx, availDate("2026-10-26"), lunch)
	require.NoError(t, err)
	assert.Equal(t, availability.ModeRoutine, o.Mode)
	assert.Nil(t, o.Note)
	require.NotNil(t, o.PresetName)
	assert.Equal(t, "Oatmeal", *o.PresetName)

	require.NoError(t, repo.UpsertOverride(ctx, availability.Override{Date: availDate("2026-11-05"), SlotID: lunch, Mode: availability.ModePlan}))
	list, err := repo.ListOverrides(ctx, availDate("2026-10-26"), availDate("2026-11-01"))
	require.NoError(t, err)
	require.Len(t, list, 1, "range is inclusive and excludes later dates")

	require.NoError(t, repo.DeleteOverride(ctx, availDate("2026-10-26"), lunch))
	require.NoError(t, repo.DeleteOverride(ctx, availDate("2026-10-26"), lunch), "delete is idempotent")
	o, err = repo.GetOverride(ctx, availDate("2026-10-26"), lunch)
	require.NoError(t, err)
	assert.Nil(t, o)
}

func TestAvailabilityRepo_SlotState(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	ctx := context.Background()
	repo := sqlite.NewAvailabilityRepo(conn)
	active := insertSlot(t, conn, "slot.lunch", 2, true)
	inactive := insertSlot(t, conn, "slot.snack", 1, false)
	first := insertSlot(t, conn, "slot.breakfast", 0, true)

	ids, err := repo.ActiveSlotIDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, []int64{first, active}, ids, "sort order, inactive excluded")

	exists, isActive, err := repo.SlotState(ctx, inactive)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.False(t, isActive)
	exists, _, err = repo.SlotState(ctx, 999)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestAvailabilityPresetName(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	repo := sqlite.NewPresetRepo(conn)
	id := insertPreset(t, conn, "Oatmeal")
	name, ok, err := repo.PresetName(context.Background(), id)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Oatmeal", name)
	_, ok, err = repo.PresetName(context.Background(), id+100)
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestAvailabilityResolve_WeekGrid resolves 7 days × 3 slots against the
// real DB. The service reads each table once (no per-cell query), so the
// whole grid comes back from a fixed number of queries.
func TestAvailabilityResolve_WeekGrid(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	ctx := context.Background()
	svc := newAvailabilityService(conn)
	b := insertSlot(t, conn, "slot.breakfast", 0, true)
	l := insertSlot(t, conn, "slot.lunch", 1, true)
	d := insertSlot(t, conn, "slot.dinner", 2, true)
	oatmeal := insertPreset(t, conn, "Oatmeal")

	grid := make([]availability.Default, 0, 5)
	for wd := 0; wd < 5; wd++ {
		grid = append(grid, availability.Default{Weekday: wd, SlotID: b, Mode: availability.ModeRoutine, PresetID: &oatmeal})
	}
	grid = append(grid, availability.Default{Weekday: 0, SlotID: l, Mode: availability.ModeNone})
	require.NoError(t, svc.SetDefaults(ctx, grid))
	_, _, err := svc.SetOverride(ctx, availability.OverrideInput{Date: availDate("2026-10-26"), SlotID: l, Mode: availability.ModePlan})
	require.NoError(t, err)
	_, _, err = svc.SetOverride(ctx, availability.OverrideInput{Date: availDate("2026-11-01"), SlotID: d, Mode: availability.ModeNone})
	require.NoError(t, err)

	cells, err := svc.Resolve(ctx, availDate("2026-10-26"), availDate("2026-11-01"))
	require.NoError(t, err)
	require.Len(t, cells, 21)

	type key struct {
		date string
		slot int64
	}
	byKey := map[key]availability.Cell{}
	for _, c := range cells {
		byKey[key{c.Date.Format("2006-01-02"), c.SlotID}] = c
	}
	mon := byKey[key{"2026-10-26", b}]
	assert.Equal(t, availability.ModeRoutine, mon.Mode)
	assert.Equal(t, availability.SourceDefault, mon.Source)
	assert.Equal(t, "Oatmeal", *mon.PresetName)
	assert.Equal(t, availability.SourceOverride, byKey[key{"2026-10-26", l}].Source)
	assert.Equal(t, availability.ModePlan, byKey[key{"2026-10-26", l}].Mode)
	assert.Equal(t, availability.ModePlan, byKey[key{"2026-10-31", b}].Mode, "Saturday breakfast falls back")
	assert.Equal(t, availability.SourceFallback, byKey[key{"2026-10-31", b}].Source)
	assert.Equal(t, availability.ModeNone, byKey[key{"2026-11-01", d}].Mode)

	mode, err := svc.ModeAt(ctx, availDate("2026-10-28"), b)
	require.NoError(t, err)
	assert.Equal(t, availability.ModeRoutine, mode)
}

func TestAvailabilitySetOverride_RemovesPlateInTx(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	ctx := context.Background()
	svc := newAvailabilityService(conn)
	lunch := insertSlot(t, conn, "slot.lunch", 1, true)
	res, err := conn.Exec(`INSERT INTO foods (name, kind, reference_portions) VALUES ('Curry', 'composed', 2)`)
	require.NoError(t, err)
	foodID, err := res.LastInsertId()
	require.NoError(t, err)
	res, err = conn.Exec(`INSERT INTO plates (slot_id, date) VALUES (?, '2026-10-26')`, lunch)
	require.NoError(t, err)
	plateID, err := res.LastInsertId()
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO plate_components (plate_id, food_id, portions, sort_order) VALUES (?, ?, 2, 0)`, plateID, foodID)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO plate_feedback (plate_id, status) VALUES (?, 'loved')`, plateID)
	require.NoError(t, err)

	in := availability.OverrideInput{Date: availDate("2026-10-26"), SlotID: lunch, Mode: availability.ModeNone}
	_, _, err = svc.SetOverride(ctx, in)
	require.ErrorIs(t, err, domain.ErrCellHasPlate)
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "availability_overrides"))
	assert.Equal(t, 1, countAvailabilityRows(t, conn, "plates"))

	in.RemovePlate = true
	cell, removed, err := svc.SetOverride(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, availability.ModeNone, cell.Mode)
	require.NotNil(t, removed)
	assert.Equal(t, plateID, removed.ID)
	require.Len(t, removed.Components, 1)
	assert.Equal(t, foodID, removed.Components[0].FoodID)
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "plates"))
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "plate_components"))
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "plate_feedback"))
	assert.Equal(t, 1, countAvailabilityRows(t, conn, "availability_overrides"))
}

func TestAvailabilityRepo_ForeignKeyFailureIsNotFound(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	ctx := context.Background()
	repo := sqlite.NewAvailabilityRepo(conn)
	lunch := insertSlot(t, conn, "slot.lunch", 1, true)
	missingPreset := int64(9999)

	err := repo.UpsertOverride(ctx, availability.Override{Date: availDate("2026-10-26"), SlotID: 9999, Mode: availability.ModeNone})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	err = repo.UpsertOverride(ctx, availability.Override{Date: availDate("2026-10-26"), SlotID: lunch, Mode: availability.ModeRoutine, PresetID: &missingPreset})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	err = repo.ReplaceActiveDefaults(ctx, []availability.Default{{Weekday: 0, SlotID: lunch, Mode: availability.ModeRoutine, PresetID: &missingPreset}})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	err = repo.ReplaceActiveDefaults(ctx, []availability.Default{{Weekday: 0, SlotID: 9999, Mode: availability.ModeNone}})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// failingUpsertRepo fails the override write so the tx must roll back the
// plate delete that ran before it.
type failingUpsertRepo struct {
	availability.Repository
}

func (failingUpsertRepo) UpsertOverride(context.Context, availability.Override) error {
	return errors.New("write failed")
}

type failingUpsertTx struct{ inner *sqlite.TxRunner }

func (f failingUpsertTx) RunInAvailabilityTx(ctx context.Context, fn func(availability.Repository, plate.Repository) error) error {
	return f.inner.RunInAvailabilityTx(ctx, func(r availability.Repository, p plate.Repository) error {
		return fn(failingUpsertRepo{r}, p)
	})
}

func TestAvailabilitySetOverride_RollsBackPlateDeleteOnWriteFailure(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	ctx := context.Background()
	svc := availability.NewService(sqlite.NewAvailabilityRepo(conn), failingUpsertTx{sqlite.NewTxRunner(conn)}, sqlite.NewPresetRepo(conn))
	lunch := insertSlot(t, conn, "slot.lunch", 1, true)
	_, err := conn.Exec(`INSERT INTO plates (slot_id, date) VALUES (?, '2026-10-26')`, lunch)
	require.NoError(t, err)

	_, removed, err := svc.SetOverride(ctx, availability.OverrideInput{Date: availDate("2026-10-26"), SlotID: lunch, Mode: availability.ModeNone, RemovePlate: true})
	require.Error(t, err)
	assert.Nil(t, removed)
	assert.Equal(t, 1, countAvailabilityRows(t, conn, "plates"), "plate delete rolled back")
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "availability_overrides"))
}
