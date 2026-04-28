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

// setupMigration15DB opens a fresh SQLite file, applies migrations 1-14, seeds
// a template with two components, and returns the connection.
func setupMigration15DB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration15.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 14))

	// Seed a food so we can create template_components.
	_, err = conn.Exec(`INSERT INTO foods (name, kind, source) VALUES ('Test Food', 'leaf', 'manual')`)
	require.NoError(t, err)

	// Seed a template with two components.
	_, err = conn.Exec(`INSERT INTO templates (name) VALUES ('Test Template')`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO template_components (template_id, food_id, portions, sort_order) VALUES (1, 1, 1, 0)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO template_components (template_id, food_id, portions, sort_order) VALUES (1, 1, 2, 1)`)
	require.NoError(t, err)

	return conn
}

// TestMigration_TemplateDayOffset_Up verifies that migration 15 adds the
// day_offset column and backfills existing rows to 0.
func TestMigration_TemplateDayOffset_Up(t *testing.T) {
	conn := setupMigration15DB(t)

	require.NoError(t, goose.UpTo(conn, "migrations", 15))

	// Both pre-existing rows must have day_offset = 0.
	rows, err := conn.Query(`SELECT id, day_offset FROM template_components ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()

	type row struct {
		id        int64
		dayOffset int64
	}
	var got []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.id, &r.dayOffset))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())

	require.Len(t, got, 2, "expected 2 pre-existing rows")
	for _, r := range got {
		require.Equal(t, int64(0), r.dayOffset, "pre-existing row id=%d must have day_offset=0", r.id)
	}
}

// TestMigration_TemplateDayOffset_Insert verifies that a new row can be
// inserted with a non-zero day_offset after migration 15.
func TestMigration_TemplateDayOffset_Insert(t *testing.T) {
	conn := setupMigration15DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 15))

	_, err := conn.Exec(
		`INSERT INTO template_components (template_id, food_id, portions, sort_order, day_offset) VALUES (1, 1, 1.5, 2, 3)`,
	)
	require.NoError(t, err)

	var dayOffset int64
	require.NoError(t, conn.QueryRow(
		`SELECT day_offset FROM template_components WHERE sort_order = 2`,
	).Scan(&dayOffset))
	require.Equal(t, int64(3), dayOffset)
}

// TestMigration_TemplateDayOffset_Down verifies that rolling back migration 15
// removes the day_offset column.
func TestMigration_TemplateDayOffset_Down(t *testing.T) {
	conn := setupMigration15DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 15))
	require.NoError(t, goose.Down(conn, "migrations"))

	// day_offset column must be gone.
	var colCount int
	require.NoError(t, conn.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('template_components') WHERE name='day_offset'`,
	).Scan(&colCount))
	require.Equal(t, 0, colCount, "day_offset column must be absent after DOWN")

	// Table must still be operable.
	_, err := conn.Exec(
		`INSERT INTO template_components (template_id, food_id, portions, sort_order) VALUES (1, 1, 1, 10)`,
	)
	require.NoError(t, err)
}
