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

// setupMigration26DB opens a fresh SQLite file migrated to 00025 holding one
// run (id 1) with one llm_call step.
func setupMigration26DB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration26.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 25))

	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at) VALUES (1, 'chat', '2026-10-01 10:00:00')`)
	mustExec(t, conn, `INSERT INTO ai_run_steps (run_id, seq, type, name) VALUES (1, 1, 'llm_call', 'm')`)
	return conn
}

func TestMigration00026_OldRowsReadBackWithNulls(t *testing.T) {
	conn := setupMigration26DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 26))

	var convID sql.NullInt64
	var reqID sql.NullString
	require.NoError(t, conn.QueryRow(`SELECT conversation_id, request_id FROM ai_runs WHERE id = 1`).Scan(&convID, &reqID))
	assert.False(t, convID.Valid)
	assert.False(t, reqID.Valid)

	var reqJSON, respJSON, stop sql.NullString
	require.NoError(t, conn.QueryRow(`SELECT request_json, response_json, stop_reason FROM ai_run_steps WHERE run_id = 1`).
		Scan(&reqJSON, &respJSON, &stop))
	assert.False(t, reqJSON.Valid)
	assert.False(t, respJSON.Valid)
	assert.False(t, stop.Valid)
}

func TestMigration00026_ToolSchemasAndIndex(t *testing.T) {
	conn := setupMigration26DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 26))

	mustExec(t, conn, `INSERT INTO ai_tool_schemas (hash, json) VALUES ('h', '[]')`)
	_, err := conn.Exec(`INSERT INTO ai_tool_schemas (hash, json) VALUES ('h', '[]')`)
	assert.Error(t, err, "hash is the primary key")
	_, err = conn.Exec(`INSERT INTO ai_tool_schemas (hash, json) VALUES ('n', NULL)`)
	assert.Error(t, err, "json is NOT NULL")

	var name string
	require.NoError(t, conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'ix_ai_runs_conversation'`).Scan(&name))
}

func TestMigration00026_Down(t *testing.T) {
	conn := setupMigration26DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 26))
	require.NoError(t, goose.DownTo(conn, "migrations", 25))

	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('ai_tool_schemas', 'ix_ai_runs_conversation')`).Scan(&n))
	assert.Zero(t, n)
	_, err := conn.Exec(`SELECT conversation_id FROM ai_runs`)
	assert.Error(t, err)
	var steps int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM ai_run_steps`).Scan(&steps))
	assert.Equal(t, 1, steps, "down keeps old steps")
}
