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
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func foodColumns(t *testing.T, conn *sql.DB) map[string]bool {
	t.Helper()
	rows, err := conn.Query(`SELECT name FROM pragma_table_info('foods')`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	cols := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		cols[name] = true
	}
	require.NoError(t, rows.Err())
	return cols
}

func TestMigration00022_AddsColumns(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	cols := foodColumns(t, conn)
	assert.True(t, cols["source_url"], "foods.source_url should exist")
	assert.True(t, cols["description"], "foods.description should exist")

	_, err := conn.Exec(`INSERT INTO foods (name, kind, source) VALUES ('Apple', 'leaf', 'manual')`)
	require.NoError(t, err, "both columns must be nullable")
}

// TestMigration00022_UpDownUp verifies the migration is reversible.
func TestMigration00022_UpDownUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "food-source-roundtrip.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))

	require.NoError(t, goose.UpTo(conn, "migrations", 22))
	require.NoError(t, goose.DownTo(conn, "migrations", 21))
	cols := foodColumns(t, conn)
	assert.False(t, cols["source_url"], "source_url should be dropped on down")
	assert.False(t, cols["description"], "description should be dropped on down")

	require.NoError(t, goose.UpTo(conn, "migrations", 22))
	cols = foodColumns(t, conn)
	assert.True(t, cols["source_url"])
	assert.True(t, cols["description"])
}
