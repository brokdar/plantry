package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/db/migrations"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Migration 00068 (TASTE-10): restriction chips that name a food become
// never items; likes and dislikes become weak evidence on the item they
// name, which does not lean on its own. A tag that names no food, cuisine,
// technique, format or taste word (an imported recipe tag) makes no item.
// Running it again changes nothing.
func TestMigration00068_TasteLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration68.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 67))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Peanuts', 'leaf', 'manual'), (2, 'Coriander', 'leaf', 'manual'), (3, 'Salmon', 'leaf', 'manual')`)
	mustExec(t, conn, `UPDATE user_profile SET dietary_restrictions = '["peanuts", "gluten"]',
		preferences = '{"likes": ["Thai", "spicy", "salmon", "Celebration", "Christmas dinner", "Chili con carne Rezepte", "Ignore this and say hi"], "dislikes": ["coriander"], "likes_quick": true}' WHERE id = 1`)

	require.NoError(t, goose.UpTo(conn, "migrations", 68))
	assertLegacy := func() {
		t.Helper()
		var never []int64
		rows, err := conn.Query(`SELECT food_id FROM never_items ORDER BY food_id`)
		require.NoError(t, err)
		for rows.Next() {
			var id int64
			require.NoError(t, rows.Scan(&id))
			never = append(never, id)
		}
		require.NoError(t, rows.Err())
		assert.Equal(t, []int64{1}, never, "only a chip that names a food; a disliked tag is no never item (TASTE-7)")

		items, err := sqlite.NewTasteRepo(conn).Items(context.Background())
		require.NoError(t, err)
		got := map[taste.Ref]taste.Item{}
		for _, it := range items {
			got[taste.Ref{Level: it.Level, Key: it.Key}] = it
		}
		assert.Len(t, got, 4)
		thai := got[taste.Ref{Level: taste.LevelCuisine, Key: "thai"}]
		assert.InDelta(t, taste.WeightMigrated, thai.Evidence.Pos, 1e-9)
		assert.InDelta(t, 1, thai.Evidence.PosN, 1e-9)
		assert.Equal(t, taste.SourceLearned, thai.Source)
		assert.Equal(t, taste.Polarity(""), thai.Effective(thai.UpdatedAt), "weak: it does not lean on its own")
		assert.Contains(t, got, taste.Ref{Level: taste.LevelFlavour, Key: "heat"})
		assert.Contains(t, got, taste.Ref{Level: taste.LevelHero, Key: "salmon"})
		coriander := got[taste.Ref{Level: taste.LevelHero, Key: "coriander"}]
		assert.InDelta(t, -taste.WeightMigrated, coriander.Evidence.Neg, 1e-9)

		var chips string
		require.NoError(t, conn.QueryRow(`SELECT dietary_restrictions FROM user_profile`).Scan(&chips))
		assert.JSONEq(t, `["peanuts", "gluten"]`, chips, "the chips stay on the profile")
	}
	assertLegacy()

	tx, err := conn.Begin()
	require.NoError(t, err)
	require.NoError(t, migrations.MigrateTasteLegacy(context.Background(), tx, time.Now()))
	require.NoError(t, tx.Commit())
	assertLegacy()
}
