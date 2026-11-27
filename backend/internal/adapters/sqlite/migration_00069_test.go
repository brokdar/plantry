package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Migration 00069: pending Discover suggestions become ready finds without
// their image; decided ones keep no state; the down migration restores the
// old table.
func TestMigration00069_PendingSuggestionsBecomeReadyFinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration69.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 68))
	ctx := context.Background()
	_, err = conn.ExecContext(ctx, `INSERT INTO recipe_suggestions (mode, source_url, draft_json, annotations_json, image_path, grounded, status)
		VALUES ('daily', 'https://a.example/1', '{"name":"Dal","image_url":"https://a.example/dal.jpg"}', '{}', 'suggestions/1.jpg', 1, 'pending'),
		       ('explore', 'https://a.example/2', '{"name":"Pho"}', '{}', NULL, 1, 'rejected')`)
	require.NoError(t, err)

	// 00070 only adds placed_on, which the repo reads.
	require.NoError(t, goose.UpTo(conn, "migrations", 70))
	repo := sqlite.NewFindRepo(conn)
	ready := finds.StateReady
	got, err := repo.List(ctx, &ready, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Dal", got[0].Draft.Name)
	require.Empty(t, got[0].Draft.ImageURL)
	all, err := repo.List(ctx, nil, 10)
	require.NoError(t, err)
	require.Len(t, all, 1)

	require.NoError(t, goose.DownTo(conn, "migrations", 68))
	var n int
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM recipe_suggestions`).Scan(&n))
	require.Equal(t, 2, n)
}
