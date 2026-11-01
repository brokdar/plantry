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

// openDBAt00037 opens a fresh SQLite file migrated to 00037 holding one chat
// run (id 1) written before the migration.
func openDBAt00037(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration37.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 36))
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at) VALUES (1, 'chat', '2026-11-01 10:00:00')`)
	require.NoError(t, goose.UpTo(conn, "migrations", 37))
	return conn
}

func TestMigration00037_AIChanges(t *testing.T) {
	conn := openDBAt00037(t)
	mustExec(t, conn, `INSERT INTO ai_changes (run_id, step_seq, tool, text_key, params_json, run_ids_json, dates_json, plate_ids_json, session_ids_json)
		VALUES (1, 2, 'set_plate_eaters', 'set_plate_eaters', '{}', '[1]', '[]', '[]', '[]')`)
	var (
		createdAt string
		undone    sql.NullString
		first     sql.NullInt64
		before    sql.NullString
	)
	require.NoError(t, conn.QueryRow(`SELECT created_at, undone_at, first_event_id, availability_before_json FROM ai_changes`).
		Scan(&createdAt, &undone, &first, &before))
	assert.NotEmpty(t, createdAt)
	assert.False(t, undone.Valid)
	assert.False(t, first.Valid)
	assert.False(t, before.Valid)

	_, err := conn.Exec(`INSERT INTO ai_changes (run_id, step_seq, tool, text_key, params_json, run_ids_json, dates_json, plate_ids_json, session_ids_json)
		VALUES (99, 1, 't', 't', '{}', '[]', '[]', '[]', '[]')`)
	assert.Error(t, err, "run_id references ai_runs")

	mustExec(t, conn, `DELETE FROM ai_runs WHERE id = 1`)
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM ai_changes`).Scan(&n))
	assert.Zero(t, n, "a run's changes go with it")
}

func TestMigration00037_Down(t *testing.T) {
	conn := openDBAt00037(t)
	require.NoError(t, goose.Down(conn, "migrations"))
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'ai_changes'`).Scan(&n))
	assert.Zero(t, n)
}
