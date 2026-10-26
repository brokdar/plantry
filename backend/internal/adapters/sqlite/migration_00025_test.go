package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
)

// setupMigration25DB opens a fresh SQLite file migrated to 00024, with one
// slot (id 1) and one leaf food (id 1).
func setupMigration25DB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration25.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 24))

	mustExec(t, conn, `INSERT INTO time_slots (id, name_key, icon) VALUES (1, 'slot.dinner', 'utensils')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Rice', 'leaf', 'manual')`)
	return conn
}

func mustExec(t *testing.T, conn *sql.DB, q string, args ...any) {
	t.Helper()
	_, err := conn.Exec(q, args...)
	require.NoError(t, err, q)
}

func seedPlate25(t *testing.T, conn *sql.DB, id int64, date string, skipped bool, note *string) {
	t.Helper()
	s := 0
	if skipped {
		s = 1
	}
	mustExec(t, conn, `INSERT INTO plates (id, slot_id, date, skipped, note) VALUES (?, 1, ?, ?, ?)`, id, date, s, note)
}

func seedComponent25(t *testing.T, conn *sql.DB, id, plateID int64, sortOrder int) {
	t.Helper()
	mustExec(t, conn, `INSERT INTO plate_components (id, plate_id, food_id, amount, unit, grams, grams_source, sort_order)
		VALUES (?, ?, 1, 100, 'g', 100, 'direct', ?)`, id, plateID, sortOrder)
}

func plateIDs25(t *testing.T, conn *sql.DB) []int64 {
	t.Helper()
	rows, err := conn.Query(`SELECT id FROM plates ORDER BY id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func strPtr25(s string) *string { return &s }

func TestMigration00025_SkippedPlateBecomesNoneOverride(t *testing.T) {
	conn := setupMigration25DB(t)
	seedPlate25(t, conn, 1, "2026-05-04", true, strPtr25("canteen"))
	seedPlate25(t, conn, 2, "2026-05-05", true, nil)

	require.NoError(t, goose.UpTo(conn, "migrations", 25))

	assert.Empty(t, plateIDs25(t, conn), "skipped plates are deleted")

	rows, err := conn.Query(`SELECT date, slot_id, mode, preset_id, note FROM availability_overrides ORDER BY date`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	type ov struct {
		date   string
		slotID int64
		mode   string
		preset sql.NullInt64
		note   sql.NullString
	}
	var got []ov
	for rows.Next() {
		var o ov
		require.NoError(t, rows.Scan(&o.date, &o.slotID, &o.mode, &o.preset, &o.note))
		got = append(got, o)
	}
	require.NoError(t, rows.Err())
	require.Len(t, got, 2)
	assert.Equal(t, ov{date: "2026-05-04", slotID: 1, mode: "none", note: sql.NullString{String: "canteen", Valid: true}}, got[0])
	assert.Equal(t, ov{date: "2026-05-05", slotID: 1, mode: "none"}, got[1])
}

func TestMigration00025_RealPlateWinsOverSkipped(t *testing.T) {
	tests := []struct {
		name           string
		withComponents bool
	}{
		{name: "real plate with components", withComponents: true},
		{name: "real plate without components", withComponents: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := setupMigration25DB(t)
			// The skipped plate has the lower id, so the real plate must win
			// on merit, not because it is the merge survivor.
			seedPlate25(t, conn, 1, "2026-05-04", true, strPtr25("eating out"))
			seedPlate25(t, conn, 2, "2026-05-04", false, nil)
			if tt.withComponents {
				seedComponent25(t, conn, 10, 2, 0)
			}

			require.NoError(t, goose.UpTo(conn, "migrations", 25))

			assert.Equal(t, []int64{2}, plateIDs25(t, conn))
			assert.Equal(t, 0, countAvailabilityRows(t, conn, "availability_overrides"))
			var comps int
			require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plate_components WHERE plate_id = 2`).Scan(&comps))
			if tt.withComponents {
				assert.Equal(t, 1, comps)
			} else {
				assert.Equal(t, 0, comps)
			}
		})
	}
}

func TestMigration00025_ExistingOverrideIsKept(t *testing.T) {
	conn := setupMigration25DB(t)
	mustExec(t, conn, `INSERT INTO availability_overrides (date, slot_id, mode, note) VALUES ('2026-05-04', 1, 'none', 'kept')`)
	seedPlate25(t, conn, 1, "2026-05-04", true, strPtr25("from plate"))

	require.NoError(t, goose.UpTo(conn, "migrations", 25))

	var note string
	require.NoError(t, conn.QueryRow(`SELECT note FROM availability_overrides WHERE date = '2026-05-04' AND slot_id = 1`).Scan(&note))
	assert.Equal(t, "kept", note)
	assert.Empty(t, plateIDs25(t, conn))
}

func TestMigration00025_DuplicatesMergeIntoLowestID(t *testing.T) {
	conn := setupMigration25DB(t)
	seedPlate25(t, conn, 5, "2026-05-04", false, strPtr25("survivor"))
	seedPlate25(t, conn, 6, "2026-05-04", false, strPtr25("dup a"))
	seedPlate25(t, conn, 7, "2026-05-04", false, nil)
	// Survivor: components 50 (sort 0), 51 (sort 1).
	seedComponent25(t, conn, 50, 5, 0)
	seedComponent25(t, conn, 51, 5, 1)
	// Plate 6: 61 (sort 1) before 60 (sort 0) by insertion; sort_order decides.
	seedComponent25(t, conn, 61, 6, 1)
	seedComponent25(t, conn, 60, 6, 0)
	// Plate 7: one component.
	seedComponent25(t, conn, 70, 7, 0)
	mustExec(t, conn, `INSERT INTO plate_feedback (plate_id, status) VALUES (5, 'loved')`)
	mustExec(t, conn, `INSERT INTO plate_feedback (plate_id, status) VALUES (6, 'disliked')`)
	// A different cell is untouched.
	seedPlate25(t, conn, 8, "2026-05-05", false, nil)
	seedComponent25(t, conn, 80, 8, 0)

	require.NoError(t, goose.UpTo(conn, "migrations", 25))

	assert.Equal(t, []int64{5, 8}, plateIDs25(t, conn))

	rows, err := conn.Query(`SELECT id, sort_order FROM plate_components WHERE plate_id = 5 ORDER BY sort_order, id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var ids []int64
	var orders []int
	for rows.Next() {
		var id int64
		var so int
		require.NoError(t, rows.Scan(&id, &so))
		ids = append(ids, id)
		orders = append(orders, so)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []int64{50, 51, 60, 61, 70}, ids)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, orders)

	var note string
	require.NoError(t, conn.QueryRow(`SELECT note FROM plates WHERE id = 5`).Scan(&note))
	assert.Equal(t, "survivor", note)

	var status string
	require.NoError(t, conn.QueryRow(`SELECT status FROM plate_feedback WHERE plate_id = 5`).Scan(&status))
	assert.Equal(t, "loved", status)
	var fb int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plate_feedback`).Scan(&fb))
	assert.Equal(t, 1, fb, "feedback of merged plates is dropped")

	var other int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plate_components WHERE plate_id = 8`).Scan(&other))
	assert.Equal(t, 1, other)
}

func TestMigration00025_ColumnDroppedAndCellUnique(t *testing.T) {
	conn := setupMigration25DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 25))

	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('plates') WHERE name = 'skipped'`).Scan(&n))
	assert.Equal(t, 0, n, "skipped column must be gone")

	mustExec(t, conn, `INSERT INTO plates (slot_id, date) VALUES (1, '2026-05-04')`)
	_, err := conn.Exec(`INSERT INTO plates (slot_id, date) VALUES (1, '2026-05-04')`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNIQUE constraint failed")
}

func TestMigration00025_RoundTrip(t *testing.T) {
	conn := setupMigration25DB(t)
	seedPlate25(t, conn, 1, "2026-05-04", false, nil)
	seedComponent25(t, conn, 10, 1, 0)
	require.NoError(t, goose.UpTo(conn, "migrations", 25))

	require.NoError(t, goose.Down(conn, "migrations"))

	var skipped int
	require.NoError(t, conn.QueryRow(`SELECT skipped FROM plates WHERE id = 1`).Scan(&skipped))
	assert.Equal(t, 0, skipped, "Down re-adds skipped with default 0")
	var idx int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'ux_plates_date_slot'`).Scan(&idx))
	assert.Equal(t, 0, idx, "Down drops the unique index")
	mustExec(t, conn, `INSERT INTO plates (slot_id, date) VALUES (1, '2026-05-04')`)
	mustExec(t, conn, `DELETE FROM plates WHERE id <> 1`)

	require.NoError(t, goose.UpTo(conn, "migrations", 25))
	assert.Equal(t, []int64{1}, plateIDs25(t, conn))
	var comps int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plate_components WHERE plate_id = 1`).Scan(&comps))
	assert.Equal(t, 1, comps)
}
