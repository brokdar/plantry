package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
)

// TestMigration00008_UpDownUp verifies that the AI migration is reversible:
// goose up → down → up leaves a schema the app can query without error.
func TestMigration00008_UpDownUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roundtrip.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))

	require.NoError(t, goose.Up(conn, "migrations"))

	// Seed a conversation + message at head.
	_, err = conn.Exec(`INSERT INTO ai_conversations (title) VALUES ('roundtrip')`)
	require.NoError(t, err)

	// Step down past 00008 — removes AI tables.
	require.NoError(t, goose.Down(conn, "migrations"))

	// ai_conversations must no longer exist.
	var exists int
	err = conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ai_conversations'`).Scan(&exists)
	require.NoError(t, err)
	require.Equal(t, 0, exists, "ai_conversations should be dropped on down")

	// Re-apply: schema should be usable again.
	require.NoError(t, goose.Up(conn, "migrations"))
	_, err = conn.Exec(`INSERT INTO ai_conversations (title) VALUES ('again')`)
	require.NoError(t, err)

	var count int
	err = conn.QueryRow(`SELECT count(*) FROM ai_conversations`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "re-upped schema holds 1 fresh row")
}
