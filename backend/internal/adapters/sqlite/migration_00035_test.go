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

// openDBAt00035 opens a fresh SQLite file migrated to 00035 holding one leaf
// (id 1) and one recipe (id 2) created before it.
func openDBAt00035(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration35.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 33))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Feta', 'leaf', 'manual')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions) VALUES (2, 'Chili', 'composed', 4)`)
	require.NoError(t, goose.UpTo(conn, "migrations", 35))
	return conn
}

func TestMigration00035_ExistingFoodsHaveNoOverride(t *testing.T) {
	conn := openDBAt00035(t)
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM foods WHERE diet_class IS NOT NULL`).Scan(&n))
	assert.Zero(t, n)
}

func TestMigration00035_Check(t *testing.T) {
	conn := openDBAt00035(t)
	for _, id := range []int{1, 2} {
		for _, c := range []string{"vegan", "vegetarian", "pescetarian", "omnivore"} {
			_, err := conn.Exec(`UPDATE foods SET diet_class = ? WHERE id = ?`, c, id)
			assert.NoError(t, err, c)
		}
	}
	for _, c := range []string{"unknown", "paleo", ""} {
		_, err := conn.Exec(`UPDATE foods SET diet_class = ? WHERE id = 1`, c)
		assert.Error(t, err, c)
	}
	_, err := conn.Exec(`UPDATE foods SET diet_class = NULL WHERE id = 1`)
	assert.NoError(t, err)
}

func TestMigration00035_Down(t *testing.T) {
	conn := openDBAt00035(t)
	require.NoError(t, goose.DownTo(conn, "migrations", 33))
	assert.False(t, columnExists(t, conn, "foods", "diet_class"))
}
