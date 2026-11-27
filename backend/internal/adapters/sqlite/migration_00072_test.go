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

// Migration 00072 adds the week theme tables and moves plan.explore_per_week
// to chef.new_dishes_per_week with its value (THEME-6); down reverses both.
func TestMigration00072_UpDown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration72.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 70))
	ctx := context.Background()
	_, err = conn.ExecContext(ctx, `INSERT INTO app_settings (key, value) VALUES ('plan.explore_per_week', '2')`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(conn, "migrations", 72))
	var v string
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'chef.new_dishes_per_week'`).Scan(&v))
	require.Equal(t, "2", v)
	var n int
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT count(*) FROM app_settings WHERE key = 'plan.explore_per_week'`).Scan(&n))
	require.Zero(t, n)
	_, err = conn.ExecContext(ctx, `INSERT INTO week_themes (window_start, kind, value, source) VALUES ('2026-11-28', 'cuisine', 'japanese', 'user')`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO week_themes (window_start, kind, value, source) VALUES ('2026-11-28', 'colour', 'red', 'user')`)
	require.Error(t, err, "unknown kind")

	require.NoError(t, goose.DownTo(conn, "migrations", 70))
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'plan.explore_per_week'`).Scan(&v))
	require.Equal(t, "2", v)
	_, err = conn.ExecContext(ctx, `SELECT 1 FROM week_themes`)
	require.Error(t, err)
}
