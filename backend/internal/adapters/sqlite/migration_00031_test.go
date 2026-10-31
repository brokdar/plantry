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

// openDBAt00031 opens a fresh SQLite file migrated to 00031 holding one leaf
// (id 1) and one recipe (id 2).
func openDBAt00031(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration31.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 29))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Zwiebel', 'leaf', 'manual')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions) VALUES (2, 'Chili', 'composed', 4)`)
	require.NoError(t, goose.UpTo(conn, "migrations", 31))
	return conn
}

func TestMigration00031_ExistingFoodsHaveNoOverride(t *testing.T) {
	conn := openDBAt00031(t)
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM foods WHERE shop_section IS NOT NULL`).Scan(&n))
	assert.Zero(t, n)
}

func TestMigration00031_Check(t *testing.T) {
	conn := openDBAt00031(t)
	for _, s := range []string{"produce", "bakery", "dairy_eggs", "meat_fish", "dry_goods", "cans_jars", "spices_oils", "frozen", "other"} {
		_, err := conn.Exec(`UPDATE foods SET shop_section = ? WHERE id = 1`, s)
		assert.NoError(t, err, s)
	}
	_, err := conn.Exec(`UPDATE foods SET shop_section = 'garden' WHERE id = 1`)
	assert.Error(t, err, "unknown section")
	_, err = conn.Exec(`UPDATE foods SET shop_section = 'produce' WHERE id = 2`)
	assert.Error(t, err, "composed food")
	_, err = conn.Exec(`UPDATE foods SET shop_section = NULL WHERE id = 1`)
	assert.NoError(t, err)
}

func TestMigration00031_Down(t *testing.T) {
	conn := openDBAt00031(t)
	require.NoError(t, goose.DownTo(conn, "migrations", 29))
	assert.False(t, columnExists(t, conn, "foods", "shop_section"))
}
