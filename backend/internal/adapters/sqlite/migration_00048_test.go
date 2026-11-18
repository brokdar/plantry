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

// Migration 00048 gives every component a position and backfills it per
// plate: the first main-like composed food is the main (one per plate).
func TestMigration00048_BackfillsPositions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration48.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 47))

	mustExec(t, conn, `INSERT INTO time_slots (id, name_key, icon) VALUES (1, 'slot.dinner', 'moon')`)
	foods := []string{
		`(1, 'Chili', 'composed', 'main', 2)`,
		`(2, 'Reis-Beilage', 'composed', 'side_starch', 2)`,
		`(3, 'Curry', 'composed', 'main', 2)`,
		`(4, 'Eintopf', 'composed', NULL, 2)`,
		`(5, 'Porridge', 'composed', 'standalone', 2)`,
		`(6, 'Kuchen', 'composed', 'dessert', 2)`,
	}
	for _, f := range foods {
		mustExec(t, conn, `INSERT INTO foods (id, name, kind, role, reference_portions) VALUES `+f)
	}
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source, shop_section) VALUES (7, 'Gurke', 'leaf', 'manual', 'produce')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source, shop_section) VALUES (8, 'Joghurt', 'leaf', 'manual', 'dairy_eggs')`)

	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (9, 'Zzqx Unbekannt', 'leaf', 'manual')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source, role) VALUES (10, 'Zzqy Override', 'leaf', 'manual', 'side_veg')`)

	plateN := 0
	comp := func(plateID, foodID int64, sortOrder int) {
		var kind string
		require.NoError(t, conn.QueryRow(`SELECT kind FROM foods WHERE id = ?`, foodID).Scan(&kind))
		if kind == "leaf" {
			mustExec(t, conn, `INSERT INTO plate_components (plate_id, food_id, amount, unit, grams, sort_order) VALUES (?, ?, 100, 'g', 100, ?)`, plateID, foodID, sortOrder)
			return
		}
		res, err := conn.Exec(`INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions) VALUES (?, '2026-11-01', 1, 2)`, foodID)
		require.NoError(t, err)
		sid, err := res.LastInsertId()
		require.NoError(t, err)
		mustExec(t, conn, `INSERT INTO plate_components (plate_id, food_id, portions, cook_session_id, sort_order) VALUES (?, ?, 1, ?, ?)`, plateID, foodID, sid, sortOrder)
	}
	plate := func(foodIDs ...int64) int64 {
		plateN++
		id := int64(plateN)
		mustExec(t, conn, `INSERT INTO plates (id, slot_id, date, eaters) VALUES (?, 1, ?, 2)`, id, "2026-11-0"+string(rune('0'+plateN)))
		for i, fid := range foodIDs {
			comp(id, fid, i)
		}
		return id
	}
	mainAndSide := plate(1, 2)
	twoMains := plate(1, 3)
	roleless := plate(4)
	breakfast := plate(5)
	leaves := plate(1, 7, 8)
	dessert := plate(1, 6)
	unclassified := plate(1, 9, 10)

	require.NoError(t, goose.UpTo(conn, "migrations", 48))

	positionsOf := func(plateID int64) []string {
		rows, err := conn.Query(`SELECT position FROM plate_components WHERE plate_id = ? ORDER BY sort_order`, plateID)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		var out []string
		for rows.Next() {
			var p string
			require.NoError(t, rows.Scan(&p))
			out = append(out, p)
		}
		require.NoError(t, rows.Err())
		return out
	}
	assert.Equal(t, []string{"main", "side"}, positionsOf(mainAndSide))
	assert.Equal(t, []string{"main", "side"}, positionsOf(twoMains), "a second main-role recipe is a side")
	assert.Equal(t, []string{"main"}, positionsOf(roleless))
	assert.Equal(t, []string{"main"}, positionsOf(breakfast))
	assert.Equal(t, []string{"main", "side", "extra"}, positionsOf(leaves), "produce is a side, dairy an extra")
	assert.Equal(t, []string{"main", "extra"}, positionsOf(dessert))
	assert.Equal(t, []string{"main", "extra", "side"}, positionsOf(unclassified), "an unclassifiable leaf is an extra, a stored side_veg override a side")

	var plates int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM (SELECT plate_id FROM plate_components WHERE position = 'main' GROUP BY plate_id HAVING COUNT(*) > 1)`).Scan(&plates))
	assert.Zero(t, plates, "at most one main per plate")

	require.NoError(t, goose.DownTo(conn, "migrations", 47))
	_, err = conn.Exec(`SELECT position FROM plate_components`)
	assert.Error(t, err)
}

// Migration 00049 adds the nullable no_cook override.
func TestMigration00049_NoCookDefaultsToDerived(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration49.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 48))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Gurke', 'leaf', 'manual')`)
	require.NoError(t, goose.UpTo(conn, "migrations", 49))

	var v sql.NullInt64
	require.NoError(t, conn.QueryRow(`SELECT no_cook FROM foods WHERE id = 1`).Scan(&v))
	assert.False(t, v.Valid)
	_, err = conn.Exec(`UPDATE foods SET no_cook = 2 WHERE id = 1`)
	assert.Error(t, err, "only 0 and 1")
}

// Migration 00052 adds the nullable serving_g, positive only.
func TestMigration00052_ServingG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration52.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 51))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Apfel', 'leaf', 'manual')`)
	require.NoError(t, goose.UpTo(conn, "migrations", 52))

	var v sql.NullFloat64
	require.NoError(t, conn.QueryRow(`SELECT serving_g FROM foods WHERE id = 1`).Scan(&v))
	assert.False(t, v.Valid)
	mustExec(t, conn, `UPDATE foods SET serving_g = 150 WHERE id = 1`)
	_, err = conn.Exec(`UPDATE foods SET serving_g = 0 WHERE id = 1`)
	assert.Error(t, err, "only positive")
	require.NoError(t, goose.DownTo(conn, "migrations", 51))
	_, err = conn.Exec(`SELECT serving_g FROM foods`)
	assert.Error(t, err)
}
