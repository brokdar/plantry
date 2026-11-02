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

// Runs and steps written before 00040 count zero web searches.
func TestMigration00040_ExistingRowsCountZeroSearches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration40.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 38))
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at) VALUES (1, 'chat', '2026-10-31 10:00:00')`)
	mustExec(t, conn, `INSERT INTO ai_run_steps (run_id, seq, type, name) VALUES (1, 1, 'llm_call', 'm')`)
	require.NoError(t, goose.UpTo(conn, "migrations", 40))

	var runN, stepN int
	require.NoError(t, conn.QueryRow(`SELECT web_search_requests FROM ai_runs WHERE id = 1`).Scan(&runN))
	require.NoError(t, conn.QueryRow(`SELECT web_search_requests FROM ai_run_steps WHERE run_id = 1`).Scan(&stepN))
	assert.Zero(t, runN)
	assert.Zero(t, stepN)

	require.NoError(t, goose.DownTo(conn, "migrations", 38))
	_, err = conn.Exec(`SELECT web_search_requests FROM ai_runs`)
	assert.Error(t, err)
}
