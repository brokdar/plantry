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

// openDBAt00032WithPresets opens a fresh SQLite file migrated to 00032
// holding preset 3 "Pasta night" (tag quick, one plate with Rice, a routine
// default and override on it) and preset 5 "Soup".
func openDBAt00032WithPresets(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration33.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 32))
	mustExec(t, conn, `INSERT INTO time_slots (id, name_key, icon) VALUES (1, 'slot.dinner', 'moon')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (2, 'Rice', 'leaf', 'manual')`)
	mustExec(t, conn, `INSERT INTO presets (id, name, last_used_at) VALUES (3, 'Pasta night', '2026-10-01 12:00:00')`)
	mustExec(t, conn, `INSERT INTO presets (id, name) VALUES (5, 'Soup')`)
	mustExec(t, conn, `INSERT INTO preset_tags (preset_id, tag) VALUES (3, 'quick')`)
	mustExec(t, conn, `INSERT INTO preset_plates (id, preset_id, slot_id, sort_order) VALUES (6, 3, 1, 0)`)
	mustExec(t, conn, `INSERT INTO preset_components (id, preset_plate_id, food_id, amount, unit, grams, sort_order)
		VALUES (7, 6, 2, 150, 'g', 150, 0)`)
	mustExec(t, conn, `INSERT INTO availability_defaults (weekday, slot_id, mode, preset_id) VALUES (0, 1, 'routine', 3)`)
	mustExec(t, conn, `INSERT INTO availability_overrides (date, slot_id, mode, preset_id) VALUES ('2026-11-02', 1, 'routine', 3)`)
	return conn
}

// The rebuild keeps every preset and every child row, and children still
// cascade from the rebuilt table.
func TestMigration00033_RebuildKeepsPresetsAndChildren(t *testing.T) {
	conn := openDBAt00032WithPresets(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 33))

	assert.Equal(t, "3|Pasta night|2026-10-01 12:00:00;5|Soup|",
		queryString(t, conn, `SELECT group_concat(id || '|' || name || '|' || COALESCE(last_used_at, ''), ';') FROM (SELECT * FROM presets ORDER BY id)`))
	assert.Equal(t, "3|quick", queryString(t, conn, `SELECT preset_id || '|' || tag FROM preset_tags`))
	assert.Equal(t, "6|3", queryString(t, conn, `SELECT id || '|' || preset_id FROM preset_plates`))
	assert.Equal(t, "7|6", queryString(t, conn, `SELECT id || '|' || preset_plate_id FROM preset_components`))
	assert.Equal(t, "1", queryString(t, conn, `SELECT COUNT(*) FROM availability_defaults WHERE preset_id = 3`))
	assert.Equal(t, "1", queryString(t, conn, `SELECT COUNT(*) FROM availability_overrides WHERE preset_id = 3`))

	var fk int
	require.NoError(t, conn.QueryRow(`PRAGMA foreign_keys`).Scan(&fk))
	assert.Equal(t, 1, fk, "foreign keys are back on")

	mustExec(t, conn, `DELETE FROM presets WHERE id = 3`)
	for _, table := range []string{"preset_tags", "preset_plates", "preset_components", "availability_defaults", "availability_overrides"} {
		assert.Equal(t, "0", queryString(t, conn, `SELECT COUNT(*) FROM `+table), table+" cascades from the rebuilt presets")
	}
}

// The name search index follows inserts, renames and deletes after the
// rebuild.
func TestMigration00033_SearchIndexFollowsPresets(t *testing.T) {
	conn := openDBAt00032WithPresets(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 33))

	assert.Equal(t, "3", queryString(t, conn, `SELECT rowid FROM presets_fts WHERE presets_fts MATCH 'pasta'`))
	mustExec(t, conn, `INSERT INTO presets (name) VALUES ('Curry')`)
	assert.Equal(t, "6", queryString(t, conn, `SELECT rowid FROM presets_fts WHERE presets_fts MATCH 'curry'`))
	mustExec(t, conn, `UPDATE presets SET name = 'Lentil soup' WHERE id = 5`)
	assert.Equal(t, "5", queryString(t, conn, `SELECT rowid FROM presets_fts WHERE presets_fts MATCH 'lentil'`))
	mustExec(t, conn, `DELETE FROM presets WHERE id = 3`)
	assert.Equal(t, "0", queryString(t, conn, `SELECT COUNT(*) FROM presets_fts WHERE presets_fts MATCH 'pasta'`))
}

// AUTOINCREMENT: a deleted highest id is never handed out again.
func TestMigration00033_NoIDReuse(t *testing.T) {
	conn := openDBAt00032WithPresets(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 33))

	mustExec(t, conn, `DELETE FROM presets WHERE id = 5`)
	res, err := conn.Exec(`INSERT INTO presets (name) VALUES ('Curry')`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	assert.Equal(t, int64(6), id, "preset 5 is not reused")
}

func TestMigration00033_DownUpRoundTrips(t *testing.T) {
	conn := openDBAt00032WithPresets(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 33))
	require.NoError(t, goose.DownTo(conn, "migrations", 32))
	assert.Equal(t, "2", queryString(t, conn, `SELECT COUNT(*) FROM presets`))
	require.NoError(t, goose.UpTo(conn, "migrations", 33))
	assert.Equal(t, "2", queryString(t, conn, `SELECT COUNT(*) FROM presets`))
	assert.Equal(t, "1", queryString(t, conn, `SELECT COUNT(*) FROM preset_components`))
}
