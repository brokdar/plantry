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

// openDBAt00029 opens a fresh SQLite file migrated to 00029 holding plate 5
// (Chili from session 3 plus 150 g Rice, rated loved) and plate 6 (empty),
// so the highest ids are not 1.
func openDBAt00029(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration30.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 29))
	mustExec(t, conn, `INSERT INTO time_slots (id, name_key, icon) VALUES (1, 'slot.dinner', 'moon')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions) VALUES (1, 'Chili', 'composed', 4)`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (2, 'Rice', 'leaf', 'manual')`)
	mustExec(t, conn, `INSERT INTO cook_sessions (id, food_id, cook_date, cook_slot_id, portions, frozen_portions, note)
		VALUES (3, 1, '2026-11-02', 1, 4, 1, 'big pot')`)
	mustExec(t, conn, `INSERT INTO plates (id, slot_id, date, eaters, note, updated_at) VALUES (5, 1, '2026-11-02', 2, 'n', '2026-11-01 10:00:00')`)
	mustExec(t, conn, `INSERT INTO plates (id, slot_id, date, eaters) VALUES (6, 1, '2026-11-03', 2)`)
	mustExec(t, conn, `INSERT INTO plate_components (id, plate_id, food_id, portions, cook_session_id, sort_order) VALUES (7, 5, 1, 1.5, 3, 0)`)
	mustExec(t, conn, `INSERT INTO plate_components (id, plate_id, food_id, amount, unit, grams, grams_source, sort_order)
		VALUES (8, 5, 2, 150, 'g', 150, 'unit', 1)`)
	mustExec(t, conn, `INSERT INTO plate_feedback (plate_id, status, note, rated_at) VALUES (5, 'loved', 'yum', '2026-11-02 20:00:00')`)
	return conn
}

func queryString(t *testing.T, conn *sql.DB, q string) string {
	t.Helper()
	var s string
	require.NoError(t, conn.QueryRow(q).Scan(&s))
	return s
}

func TestMigration00030_CreatesPlateEvents(t *testing.T) {
	conn := openDBAt00029(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 30))

	for _, c := range []string{"id", "plate_id", "cook_session_id", "date", "slot_id", "action", "actor", "ai_run_id", "before_json", "after_json", "at"} {
		assert.True(t, columnExists(t, conn, "plate_events", c), c)
	}
	assert.True(t, indexExists(t, conn, "ix_plate_events_plate"))
	assert.True(t, indexExists(t, conn, "ix_plate_events_run"))

	// No foreign keys: the ids point at nothing and the row still inserts
	// with foreign_keys on (D20, B3-AC5).
	mustExec(t, conn, `INSERT INTO plate_events (plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id)
		VALUES (999, 998, '2026-11-02', 997, 'delete', 'agent', 996)`)
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_list('plate_events')`).Scan(&n))
	assert.Zero(t, n)
	assert.NotEmpty(t, queryString(t, conn, `SELECT at FROM plate_events`), "at defaults to now")
}

// The rebuild keeps every row, id and link, and every index.
func TestMigration00030_RebuildKeepsData(t *testing.T) {
	conn := openDBAt00029(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 30))

	assert.Equal(t, "5|2026-11-02|2.0|n|2026-11-01 10:00:00;6|2026-11-03|2.0||",
		queryString(t, conn, `SELECT group_concat(id || '|' || date || '|' || eaters || '|' || COALESCE(note, '') || '|' || updated_at, ';') FROM (SELECT * FROM plates ORDER BY id)`))
	assert.Equal(t, "3|1|2026-11-02|1|4.0|1.0|big pot",
		queryString(t, conn, `SELECT id || '|' || food_id || '|' || cook_date || '|' || cook_slot_id || '|' || portions || '|' || frozen_portions || '|' || note FROM cook_sessions`))
	assert.Equal(t, "7|5|3;8|5|",
		queryString(t, conn, `SELECT group_concat(id || '|' || plate_id || '|' || COALESCE(cook_session_id, ''), ';') FROM (SELECT * FROM plate_components ORDER BY id)`))
	assert.Equal(t, "5|loved|yum|2026-11-02 20:00:00",
		queryString(t, conn, `SELECT plate_id || '|' || status || '|' || note || '|' || rated_at FROM plate_feedback`))

	for _, ix := range []string{"ix_plates_date", "ix_plates_date_slot", "ux_plates_date_slot", "ix_cook_sessions_cook_date", "ix_cook_sessions_food"} {
		assert.True(t, indexExists(t, conn, ix), ix)
	}
	_, err := conn.Exec(`INSERT INTO plates (slot_id, date) VALUES (1, '2026-11-02')`)
	assert.Error(t, err, "the cell is still unique")

	rows, err := conn.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	assert.False(t, rows.Next(), "no foreign key violations")
	require.NoError(t, rows.Close())

	var fk int
	require.NoError(t, conn.QueryRow(`PRAGMA foreign_keys`).Scan(&fk))
	assert.Equal(t, 1, fk, "foreign keys are back on")
	mustExec(t, conn, `DELETE FROM plates WHERE id = 5`)
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plate_components WHERE plate_id = 5`).Scan(&n))
	assert.Zero(t, n, "children still cascade from the rebuilt plates")
	_, err = conn.Exec(`DELETE FROM cook_sessions WHERE id = 3`)
	assert.NoError(t, err, "no component links session 3 any more")
}

// AUTOINCREMENT: a deleted highest id is never handed out again.
func TestMigration00030_NoIDReuse(t *testing.T) {
	conn := openDBAt00029(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 30))

	mustExec(t, conn, `DELETE FROM plates WHERE id = 6`)
	res, err := conn.Exec(`INSERT INTO plates (slot_id, date) VALUES (1, '2026-11-09')`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	assert.Equal(t, int64(7), id, "plate 6 is not reused")

	mustExec(t, conn, `INSERT INTO cook_sessions (id, food_id, cook_date, cook_slot_id, portions) VALUES (10, 1, '2026-11-09', 1, 2)`)
	mustExec(t, conn, `DELETE FROM cook_sessions WHERE id = 10`)
	res, err = conn.Exec(`INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions) VALUES (1, '2026-11-09', 1, 2)`)
	require.NoError(t, err)
	id, err = res.LastInsertId()
	require.NoError(t, err)
	assert.Equal(t, int64(11), id, "session 10 is not reused")
}

func TestMigration00030_DownUpRoundTrips(t *testing.T) {
	conn := openDBAt00029(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 30))
	require.NoError(t, goose.DownTo(conn, "migrations", 29))
	assert.False(t, columnExists(t, conn, "plate_events", "id"))
	assert.False(t, indexExists(t, conn, "ix_plate_events_plate"))
	assert.Equal(t, "2", queryString(t, conn, `SELECT COUNT(*) FROM plates`))

	require.NoError(t, goose.UpTo(conn, "migrations", 30))
	assert.True(t, columnExists(t, conn, "plate_events", "id"))
	assert.Equal(t, "2", queryString(t, conn, `SELECT COUNT(*) FROM plate_components`))
}
