package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
)

// setupMigration14DB opens a fresh SQLite file, applies migrations 1-13, seeds
// a minimal week + slot + plate row, and returns the connection.
func setupMigration14DB(t *testing.T, year, weekNum, day int) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration14.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 13))

	_, err = conn.Exec(`INSERT INTO weeks (year, week_number) VALUES (?, ?)`, year, weekNum)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO time_slots (name_key, icon) VALUES ('slot.dinner', 'utensils')`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO plates (week_id, day, slot_id) VALUES (1, ?, 1)`, day)
	require.NoError(t, err)

	return conn
}

// TestMigration_DateKeyedPlates_Up verifies that the Up migration backfills the
// date column correctly for a regular week and for the cross-year ISO boundary.
func TestMigration_DateKeyedPlates_Up(t *testing.T) {
	t.Run("regular week backfill", func(t *testing.T) {
		// week_number=17 of 2026, day=0 (Monday)
		// ISO week 17 of 2026 starts on Mon 2026-04-20.
		conn := setupMigration14DB(t, 2026, 17, 0)

		require.NoError(t, goose.UpTo(conn, "migrations", 14))

		var date string
		require.NoError(t, conn.QueryRow(`SELECT date FROM plates WHERE id = 1`).Scan(&date))
		require.Equal(t, "2026-04-20", date)
	})

	t.Run("ISO week boundary: 2025 week 1 day 0 = 2024-12-30", func(t *testing.T) {
		// ISO week 1 of 2025 starts on Mon 2024-12-30.
		conn := setupMigration14DB(t, 2025, 1, 0)

		require.NoError(t, goose.UpTo(conn, "migrations", 14))

		var date string
		require.NoError(t, conn.QueryRow(`SELECT date FROM plates WHERE id = 1`).Scan(&date))
		require.Equal(t, "2024-12-30", date, "ISO week 1 of 2025 starts on 2024-12-30")
	})

	t.Run("53rd week of 2020 day 6 = 2021-01-03", func(t *testing.T) {
		// 2020 has 53 ISO weeks. week=53, day=6 (Sunday).
		conn := setupMigration14DB(t, 2020, 53, 6)

		require.NoError(t, goose.UpTo(conn, "migrations", 14))

		var date string
		require.NoError(t, conn.QueryRow(`SELECT date FROM plates WHERE id = 1`).Scan(&date))
		require.Equal(t, "2021-01-03", date, "53rd week of 2020, day 6 must be 2021-01-03")
	})
}

// TestMigration_DateKeyedPlates_NotNull verifies that after migration 14 the
// date column rejects NULL values.
func TestMigration_DateKeyedPlates_NotNull(t *testing.T) {
	conn := setupMigration14DB(t, 2026, 17, 0)
	require.NoError(t, goose.UpTo(conn, "migrations", 14))

	// Attempting to insert a plate with a NULL date must fail.
	_, err := conn.Exec(`INSERT INTO plates (week_id, day, slot_id, date) VALUES (1, 1, 1, NULL)`)
	require.Error(t, err, "inserting NULL date must be rejected after migration 14")
}

// TestMigration_DateKeyedPlates_Down verifies that rolling back migration 14
// removes the date column and the date-related indexes.
func TestMigration_DateKeyedPlates_Down(t *testing.T) {
	conn := setupMigration14DB(t, 2026, 17, 0)

	require.NoError(t, goose.UpTo(conn, "migrations", 14))
	require.NoError(t, goose.Down(conn, "migrations"))

	// date column must be gone.
	var colCount int
	require.NoError(t, conn.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('plates') WHERE name='date'`,
	).Scan(&colCount))
	require.Equal(t, 0, colCount, "date column must be absent after DOWN")

	// date indexes must be gone.
	for _, idx := range []string{"ix_plates_date", "ix_plates_date_slot"} {
		var n int
		require.NoError(t, conn.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx,
		).Scan(&n))
		require.Equalf(t, 0, n, "index %s must be absent after DOWN", idx)
	}

	// The original index must still exist.
	var origIdx int
	require.NoError(t, conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ix_plates_week_day_slot'`,
	).Scan(&origIdx))
	require.Equal(t, 1, origIdx, "original ix_plates_week_day_slot must survive DOWN")

	// Plates table must still be operable.
	_, err := conn.Exec(`INSERT INTO plates (week_id, day, slot_id) VALUES (1, 2, 1)`)
	require.NoError(t, err, "plates table must accept inserts after DOWN")
}
