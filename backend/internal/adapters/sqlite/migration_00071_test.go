package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/db/migrations"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Migration 00071 removes the hero items 00068 made from imported recipe
// tags: a learned hero item with only migrated evidence whose key names no
// leaf food goes, with its signals. A hero naming a food, an item with any
// other signal, a statement and other levels stay. Running it again
// changes nothing.
func TestMigration00071_TasteLegacyJunk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration71.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 70))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Salmon', 'leaf', 'manual')`)
	for _, it := range []struct{ level, key, source, other string }{
		{"hero", "celebration", "learned", ""},
		{"hero", "christmas_dinner", "learned", ""},
		{"hero", "salmon", "learned", ""},
		{"hero", "quick_curry", "learned", "loved"},
		{"hero", "dinner", "statement", ""},
		{"cuisine", "thai", "learned", ""},
	} {
		mustExec(t, conn, `INSERT INTO taste_signals (kind, level, key, weight, share, at) VALUES ('migrated', ?, ?, 0.5, 1, '2026-11-01 10:00:00')`, it.level, it.key)
		if it.other != "" {
			mustExec(t, conn, `INSERT INTO taste_signals (kind, level, key, weight, share, at) VALUES (?, ?, ?, 0.1, 0.1, '2026-11-02 10:00:00')`, it.other, it.level, it.key)
		}
		mustExec(t, conn, `INSERT INTO taste_items (level, key, scope, window_start, source, pos_evidence, pos_n, neg_evidence, neg_n, created_at, updated_at)
			VALUES (?, ?, 'always', '', ?, 0.5, 1, 0, 0, '2026-11-01 10:00:00', '2026-11-01 10:00:00')`, it.level, it.key, it.source)
	}

	require.NoError(t, goose.UpTo(conn, "migrations", 71))
	want := []string{"cuisine/thai", "hero/dinner", "hero/quick_curry", "hero/salmon"}
	assertLeft := func() {
		t.Helper()
		assert.Equal(t, want, column(t, conn, `SELECT level || '/' || key FROM taste_items ORDER BY 1`))
		assert.Equal(t, want, column(t, conn, `SELECT DISTINCT level || '/' || key FROM taste_signals ORDER BY 1`), "the junk's signals go too")
	}
	assertLeft()

	tx, err := conn.Begin()
	require.NoError(t, err)
	require.NoError(t, migrations.CleanTasteLegacyJunk(context.Background(), tx))
	require.NoError(t, tx.Commit())
	assertLeft()
}

func column(t *testing.T, conn *sql.DB, q string) []string {
	t.Helper()
	rows, err := conn.Query(q)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	return out
}
