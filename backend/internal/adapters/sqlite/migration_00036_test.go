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

// openDBAt00036 opens a fresh SQLite file migrated to 00036 holding one run
// (id 1) written before the migration.
func openDBAt00036(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration36.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 35))
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at) VALUES (1, 'chat', '2026-10-31 10:00:00')`)
	require.NoError(t, goose.UpTo(conn, "migrations", 36))
	return conn
}

func TestMigration00036_ExistingRunsHaveNoNotes(t *testing.T) {
	conn := openDBAt00036(t)
	var notes sql.NullString
	require.NoError(t, conn.QueryRow(`SELECT notes FROM ai_runs WHERE id = 1`).Scan(&notes))
	assert.False(t, notes.Valid)

	mustExec(t, conn, `UPDATE ai_runs SET notes = 'Batch-cooked chili on Monday.' WHERE id = 1`)
	require.NoError(t, conn.QueryRow(`SELECT notes FROM ai_runs WHERE id = 1`).Scan(&notes))
	assert.Equal(t, "Batch-cooked chili on Monday.", notes.String)
}

func TestMigration00036_Down(t *testing.T) {
	conn := openDBAt00036(t)
	require.NoError(t, goose.Down(conn, "migrations"))
	assert.False(t, columnExists(t, conn, "ai_runs", "notes"))
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&n))
	assert.Equal(t, 1, n)
}
