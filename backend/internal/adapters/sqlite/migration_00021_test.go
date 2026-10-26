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

func TestMigration00021_CreatesTables(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	for _, name := range []string{"ai_prompts", "ai_runs", "ai_run_steps"} {
		var n int
		err := conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
		require.NoError(t, err)
		assert.Equal(t, 1, n, "table %s should exist", name)
	}
}

func TestMigration00021_RejectsInvalidEnums(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	tests := []struct {
		name    string
		kind    string
		status  string
		wantErr bool
	}{
		{"valid", "generate", "running", false},
		{"lookup kind", "lookup", "ok", false},
		{"untracked kind", "untracked", "partial", false},
		{"invalid kind", "bogus", "ok", true},
		{"invalid status", "chat", "done", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := conn.Exec(`INSERT INTO ai_runs (kind, started_at, status) VALUES (?, '2026-01-01 00:00:00', ?)`, tt.kind, tt.status)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestMigration00021_DefaultsToRunning(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	res, err := conn.Exec(`INSERT INTO ai_runs (kind, started_at) VALUES ('chat', '2026-01-01 00:00:00')`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)

	var status string
	require.NoError(t, conn.QueryRow(`SELECT status FROM ai_runs WHERE id = ?`, id).Scan(&status))
	assert.Equal(t, "running", status)
}

func TestMigration00021_DeleteRunCascadesSteps(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	res, err := conn.Exec(`INSERT INTO ai_runs (kind, started_at) VALUES ('chat', '2026-01-01 00:00:00')`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO ai_run_steps (run_id, seq, type, name) VALUES (?, 1, 'llm_call', 'm'), (?, 2, 'tool_call', 't')`, id, id)
	require.NoError(t, err)

	_, err = conn.Exec(`DELETE FROM ai_runs WHERE id = ?`, id)
	require.NoError(t, err)

	var n int
	require.NoError(t, conn.QueryRow(`SELECT count(*) FROM ai_run_steps`).Scan(&n))
	assert.Equal(t, 0, n)
}

// TestMigration00021_UpDownUp verifies the migration is reversible.
func TestMigration00021_UpDownUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai-runs-roundtrip.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))

	require.NoError(t, goose.UpTo(conn, "migrations", 21))
	require.NoError(t, goose.DownTo(conn, "migrations", 20))

	var n int
	err = conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('ai_prompts','ai_runs','ai_run_steps')`).Scan(&n)
	require.NoError(t, err)
	require.Equal(t, 0, n, "ai run tables should be dropped on down")

	require.NoError(t, goose.UpTo(conn, "migrations", 21))
	_, err = conn.Exec(`INSERT INTO ai_runs (kind, started_at) VALUES ('chat', '2026-01-01 00:00:00')`)
	require.NoError(t, err)
}
