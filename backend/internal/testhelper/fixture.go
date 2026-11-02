package testhelper

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/db"
)

// CopyDB copies the SQLite file at src into a temp dir and opens the copy
// WITHOUT migrating it, so a test can read the old schema before MigrateDB.
func CopyDB(t testing.TB, src string) *sql.DB {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "copy.db")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	conn, err := sql.Open("sqlite", "file:"+path+dsnPragmas)
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// MigrateDB migrates conn to the latest schema.
func MigrateDB(t testing.TB, conn *sql.DB) {
	t.Helper()
	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(conn, "migrations"))
}
