package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Migration 00070 adds placed_on and the planning session row; down drops
// both.
func TestMigration00070_UpDown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration70.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 70))
	ctx := context.Background()
	_, err = conn.ExecContext(ctx, `INSERT INTO chef_planning_session (id, until) VALUES (1, '2026-11-02T18:30:00Z')`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `SELECT placed_on FROM recipe_suggestions`)
	require.NoError(t, err)

	require.NoError(t, goose.DownTo(conn, "migrations", 69))
	_, err = conn.ExecContext(ctx, `SELECT placed_on FROM recipe_suggestions`)
	require.Error(t, err)
	_, err = conn.ExecContext(ctx, `SELECT 1 FROM chef_planning_session`)
	require.Error(t, err)
}
