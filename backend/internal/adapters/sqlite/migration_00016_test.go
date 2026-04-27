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

// setupMigration16DB opens a fresh SQLite file, applies migrations 1-15, seeds
// a week + slot + plate row, and returns the connection. The plate row has both
// week_id and date populated (as they exist after migration 15).
func setupMigration16DB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration16.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 15))

	_, err = conn.Exec(`INSERT INTO weeks (year, week_number) VALUES (2026, 17)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO time_slots (name_key, icon) VALUES ('slot.dinner', 'utensils')`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO plates (week_id, day, slot_id, date) VALUES (1, 0, 1, '2026-04-20')`)
	require.NoError(t, err)

	return conn
}

// TestMigration00016_WeeksTableDropped verifies that after migration 16 the
// weeks table no longer exists.
func TestMigration00016_WeeksTableDropped(t *testing.T) {
	conn := setupMigration16DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 16))

	var n int
	require.NoError(t, conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='weeks'`,
	).Scan(&n))
	require.Equal(t, 0, n, "weeks table must be absent after migration 16")
}

// TestMigration00016_PlatesColumnsDropped verifies that week_id and day columns
// are removed from the plates table after migration 16.
func TestMigration00016_PlatesColumnsDropped(t *testing.T) {
	conn := setupMigration16DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 16))

	for _, col := range []string{"week_id", "day"} {
		var count int
		require.NoError(t, conn.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('plates') WHERE name=?`, col,
		).Scan(&count))
		require.Equalf(t, 0, count, "column %s must be absent from plates after migration 16", col)
	}
}

// TestMigration00016_DateIndexesExist verifies that the date-based indexes are
// still present on the plates table after migration 16.
func TestMigration00016_DateIndexesExist(t *testing.T) {
	conn := setupMigration16DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 16))

	for _, idx := range []string{"ix_plates_date", "ix_plates_date_slot"} {
		var n int
		require.NoError(t, conn.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx,
		).Scan(&n))
		require.Equalf(t, 1, n, "index %s must exist after migration 16", idx)
	}
}

// TestMigration00016_RoundTrip applies migration 16, inserts a plate row,
// rolls back, and applies migration 16 again. The plate row count must be
// unchanged after the round-trip.
func TestMigration00016_RoundTrip(t *testing.T) {
	conn := setupMigration16DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 16))

	// Count rows after initial up (seed row copied from pre-16 state).
	var countAfterUp int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plates`).Scan(&countAfterUp))
	require.Equal(t, 1, countAfterUp, "seed plate row must survive the up migration")

	// Roll back migration 16.
	require.NoError(t, goose.Down(conn, "migrations"))

	// Verify weeks table is back and plates has week_id / day columns.
	var weeksBack int
	require.NoError(t, conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='weeks'`,
	).Scan(&weeksBack))
	require.Equal(t, 1, weeksBack, "weeks table must be recreated after DOWN")

	var weekIDCol int
	require.NoError(t, conn.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('plates') WHERE name='week_id'`,
	).Scan(&weekIDCol))
	require.Equal(t, 1, weekIDCol, "week_id column must be restored after DOWN")

	// Row count must be preserved through the down migration.
	var countAfterDown int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plates`).Scan(&countAfterDown))
	require.Equal(t, countAfterUp, countAfterDown, "plate row count must be unchanged after DOWN")

	// Apply migration 16 again.
	require.NoError(t, goose.UpTo(conn, "migrations", 16))

	var countAfterReUp int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plates`).Scan(&countAfterReUp))
	require.Equal(t, countAfterUp, countAfterReUp, "plate row count must be unchanged after round-trip")
}

// TestMigration00016_PlatesDateColumnSurvives verifies that existing plate data
// is intact after migration 16 (the date value is preserved).
func TestMigration00016_PlatesDateColumnSurvives(t *testing.T) {
	conn := setupMigration16DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 16))

	var date string
	err := conn.QueryRow(`SELECT date FROM plates WHERE id = 1`).Scan(&date)
	require.NoError(t, err)
	require.Equal(t, "2026-04-20", date, "plate date must be preserved after migration 16")
}

// Verify that migration 16 is reachable as the final migration via goose.Up.
func TestMigration00016_FullMigrateUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration16_full.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(conn, "migrations"))

	// Confirm weeks is gone and the plates date indexes exist.
	var weeksCount int
	require.NoError(t, conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='weeks'`,
	).Scan(&weeksCount))
	require.Equal(t, 0, weeksCount, "weeks table must be absent after full migrate up")
}
