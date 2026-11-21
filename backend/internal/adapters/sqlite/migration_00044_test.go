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

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// 00044 rebuilds ai_run_steps for the code_execution type: existing steps
// survive, new columns are NULL, and the down migration restores the CHECK.
func TestMigration00044_StepsRebuiltForCodeExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration44.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 43))
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at) VALUES (1, 'chat', '2026-10-31 10:00:00')`)
	mustExec(t, conn, `INSERT INTO ai_run_steps (run_id, seq, type, name, args_json, provider_request_id, web_search_requests) VALUES (1, 1, 'llm_call', 'm', 'a', 'req_1', 2)`)
	_, err = conn.Exec(`INSERT INTO ai_run_steps (run_id, seq, type, name) VALUES (1, 2, 'code_execution', 'x')`)
	require.Error(t, err, "the old CHECK refuses the new type")

	require.NoError(t, goose.UpTo(conn, "migrations", 44))
	var name, args, reqID string
	var searches int
	var caller sql.NullString
	require.NoError(t, conn.QueryRow(`SELECT name, args_json, provider_request_id, web_search_requests, caller FROM ai_run_steps WHERE run_id = 1`).
		Scan(&name, &args, &reqID, &searches, &caller))
	assert.Equal(t, []any{"m", "a", "req_1", 2, false}, []any{name, args, reqID, searches, caller.Valid})
	mustExec(t, conn, `INSERT INTO ai_run_steps (run_id, seq, type, name, caller, caller_tool_id) VALUES (1, 2, 'code_execution', 'code_execution', 'direct', 'srv')`)
	mustExec(t, conn, `UPDATE ai_runs SET container_id = 'c1' WHERE id = 1`)

	require.NoError(t, goose.DownTo(conn, "migrations", 43))
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM ai_run_steps`).Scan(&n))
	assert.Equal(t, 1, n, "code_execution rows go with the down migration")
	_, err = conn.Exec(`SELECT container_id FROM ai_runs`)
	assert.Error(t, err)
}
