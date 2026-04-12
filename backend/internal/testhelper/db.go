package testhelper

import (
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
)

// NewTestDB returns a fresh in-memory SQLite database with all migrations
// applied. The connection is closed automatically when the test ends.
func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)

	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(conn, "migrations"))

	return conn
}
