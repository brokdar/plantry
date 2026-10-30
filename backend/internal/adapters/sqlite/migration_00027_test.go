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

// setupMigration27DB opens a fresh SQLite file migrated to 00026 and seeds:
//   - slots 1 (lunch, sort 1) and 2 (dinner, sort 0)
//   - leaf Rice (1), composed Chili (2) and Curry (3)
//   - household_portions = 2
//   - plate 10 (2026-11-02, slot 1): Chili ×1, Rice 150 g
//   - plate 11 (2026-11-01, slot 1): Curry ×2
//   - plate 12 (2026-11-01, slot 2): Chili ×3
//   - a preset with a composed (×2) and a leaf component
func setupMigration27DB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration27.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 26))

	mustExec(t, conn, `INSERT INTO time_slots (id, name_key, icon, sort_order) VALUES (1, 'slot.lunch', 'sun', 1), (2, 'slot.dinner', 'moon', 0)`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Rice', 'leaf', 'manual')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, role, reference_portions) VALUES (2, 'Chili', 'composed', 'main', 4), (3, 'Curry', 'composed', 'main', 2)`)
	mustExec(t, conn, `UPDATE user_profile SET household_portions = 2 WHERE id = 1`)

	mustExec(t, conn, `INSERT INTO plates (id, slot_id, date, note, created_at) VALUES
		(10, 1, '2026-11-02', 'late', '2026-10-01 10:00:00'),
		(11, 1, '2026-11-01', NULL, '2026-10-02 10:00:00'),
		(12, 2, '2026-11-01', NULL, '2026-10-03 10:00:00')`)
	mustExec(t, conn, `INSERT INTO plate_components (id, plate_id, food_id, portions, sort_order) VALUES
		(100, 10, 2, 1, 0),
		(102, 11, 3, 2, 0),
		(103, 12, 2, 3, 0)`)
	mustExec(t, conn, `INSERT INTO plate_components (id, plate_id, food_id, amount, unit, grams, grams_source, sort_order)
		VALUES (101, 10, 1, 150, 'g', 150, 'direct', 1)`)

	mustExec(t, conn, `INSERT INTO presets (id, name) VALUES (1, 'Lunch box')`)
	mustExec(t, conn, `INSERT INTO preset_plates (id, preset_id, slot_id) VALUES (1, 1, 1)`)
	mustExec(t, conn, `INSERT INTO preset_components (id, preset_plate_id, food_id, portions, sort_order) VALUES (1, 1, 2, 2, 0)`)
	mustExec(t, conn, `INSERT INTO preset_components (id, preset_plate_id, food_id, amount, unit, grams, grams_source, sort_order)
		VALUES (2, 1, 1, 80, 'g', 80, 'direct', 1)`)
	return conn
}

type session27 struct {
	ID, FoodID, SlotID int64
	Date               string
	Portions, Frozen   float64
}

func sessionOf27(t *testing.T, conn *sql.DB, componentID int64) session27 {
	t.Helper()
	var s session27
	err := conn.QueryRow(`SELECT s.id, s.food_id, s.cook_slot_id, s.cook_date, s.portions, s.frozen_portions
		FROM plate_components pc JOIN cook_sessions s ON s.id = pc.cook_session_id
		WHERE pc.id = ?`, componentID).Scan(&s.ID, &s.FoodID, &s.SlotID, &s.Date, &s.Portions, &s.Frozen)
	require.NoError(t, err)
	return s
}

func TestMigration00027_ComposedComponentsGetOwnSessions(t *testing.T) {
	conn := setupMigration27DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 27))

	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM cook_sessions`).Scan(&n))
	assert.Equal(t, 3, n, "one session per composed component")

	chiliLate := sessionOf27(t, conn, 100)
	assert.Equal(t, session27{ID: chiliLate.ID, FoodID: 2, SlotID: 1, Date: "2026-11-02", Portions: 2, Frozen: 0}, chiliLate)
	curry := sessionOf27(t, conn, 102)
	assert.Equal(t, session27{ID: curry.ID, FoodID: 3, SlotID: 1, Date: "2026-11-01", Portions: 4, Frozen: 0}, curry)
	chiliDinner := sessionOf27(t, conn, 103)
	assert.Equal(t, session27{ID: chiliDinner.ID, FoodID: 2, SlotID: 2, Date: "2026-11-01", Portions: 6, Frozen: 0}, chiliDinner)

	// Sessions are created in (date, slot sort_order, component id) order:
	// Nov 1 dinner (sort 0), Nov 1 lunch (sort 1), Nov 2 lunch.
	assert.Less(t, chiliDinner.ID, curry.ID)
	assert.Less(t, curry.ID, chiliLate.ID)

	var portions float64
	require.NoError(t, conn.QueryRow(`SELECT portions FROM plate_components WHERE id = 103`).Scan(&portions))
	assert.InDelta(t, 3.0, portions, 1e-9, "component portions stay per eater")
}

func TestMigration00027_LeafRowsUnchanged(t *testing.T) {
	conn := setupMigration27DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 27))

	var (
		plateID, foodID, sortOrder int64
		amount, grams              float64
		unit, source               string
		portions, session          sql.NullFloat64
	)
	err := conn.QueryRow(`SELECT plate_id, food_id, amount, unit, grams, grams_source, sort_order, portions, cook_session_id
		FROM plate_components WHERE id = 101`).
		Scan(&plateID, &foodID, &amount, &unit, &grams, &source, &sortOrder, &portions, &session)
	require.NoError(t, err)
	assert.Equal(t, int64(10), plateID)
	assert.Equal(t, int64(1), foodID)
	assert.InDelta(t, 150.0, amount, 1e-9)
	assert.Equal(t, "g", unit)
	assert.InDelta(t, 150.0, grams, 1e-9)
	assert.Equal(t, "direct", source)
	assert.Equal(t, int64(1), sortOrder)
	assert.False(t, portions.Valid)
	assert.False(t, session.Valid)
}

func TestMigration00027_PlatesGetEatersAndUpdatedAt(t *testing.T) {
	conn := setupMigration27DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 27))

	rows, err := conn.Query(`SELECT eaters, updated_at, created_at FROM plates ORDER BY id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var eaters float64
		var updated, created string
		require.NoError(t, rows.Scan(&eaters, &updated, &created))
		assert.InDelta(t, 2.0, eaters, 1e-9, "eaters = household_portions")
		assert.Equal(t, created, updated)
		count++
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, 3, count)
}

func TestMigration00027_MissingProfileKeepsOneEater(t *testing.T) {
	conn := setupMigration27DB(t)
	mustExec(t, conn, `DELETE FROM user_profile`)
	require.NoError(t, goose.UpTo(conn, "migrations", 27))

	var eaters float64
	require.NoError(t, conn.QueryRow(`SELECT eaters FROM plates WHERE id = 12`).Scan(&eaters))
	assert.InDelta(t, 1.0, eaters, 1e-9)
	assert.InDelta(t, 3.0, sessionOf27(t, conn, 103).Portions, 1e-9, "portions × 1 eater")
}

func TestMigration00027_Checks(t *testing.T) {
	conn := setupMigration27DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 27))
	sessionID := sessionOf27(t, conn, 100).ID

	reject := []struct {
		name string
		sql  string
		args []any
	}{
		{"composed without a session", `INSERT INTO plate_components (plate_id, food_id, portions) VALUES (10, 2, 1)`, nil},
		{"composed with zero portions", `INSERT INTO plate_components (plate_id, food_id, portions, cook_session_id) VALUES (10, 2, 0, ?)`, []any{sessionID}},
		{"composed with amount", `INSERT INTO plate_components (plate_id, food_id, portions, cook_session_id, amount) VALUES (10, 2, 1, ?, 5)`, []any{sessionID}},
		{"leaf with a session", `INSERT INTO plate_components (plate_id, food_id, amount, unit, grams, cook_session_id) VALUES (10, 1, 5, 'g', 5, ?)`, []any{sessionID}},
		{"leaf without unit", `INSERT INTO plate_components (plate_id, food_id, amount, unit, grams) VALUES (10, 1, 5, '', 5)`, nil},
		{"neither shape", `INSERT INTO plate_components (plate_id, food_id) VALUES (10, 1)`, nil},
		{"zero eaters", `UPDATE plates SET eaters = 0 WHERE id = 10`, nil},
		{"session zero portions", `INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions) VALUES (2, '2026-11-03', 1, 0)`, nil},
		{"frozen above portions", `INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions, frozen_portions) VALUES (2, '2026-11-03', 1, 2, 3)`, nil},
		{"negative frozen", `INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions, frozen_portions) VALUES (2, '2026-11-03', 1, 2, -1)`, nil},
		{"delete a linked session", `DELETE FROM cook_sessions WHERE id = ?`, []any{sessionID}},
		{"keep_days out of range", `UPDATE foods SET keep_days = 8 WHERE id = 2`, nil},
		{"keep_days on a leaf", `UPDATE foods SET keep_days = 3 WHERE id = 1`, nil},
		{"freezable not boolean", `UPDATE foods SET freezable = 2 WHERE id = 2`, nil},
		{"freezable on a leaf", `UPDATE foods SET freezable = 1 WHERE id = 1`, nil},
		{"preset composed zero portions", `INSERT INTO preset_components (preset_plate_id, food_id, portions) VALUES (1, 2, 0)`, nil},
	}
	for _, tc := range reject {
		t.Run(tc.name, func(t *testing.T) {
			_, err := conn.Exec(tc.sql, tc.args...)
			assert.Error(t, err)
		})
	}

	mustExec(t, conn, `INSERT INTO plate_components (plate_id, food_id, portions, cook_session_id) VALUES (10, 2, 1.5, ?)`, sessionID)
	mustExec(t, conn, `UPDATE foods SET keep_days = 7, freezable = 0 WHERE id = 2`)
	mustExec(t, conn, `INSERT INTO preset_components (preset_plate_id, food_id, portions) VALUES (1, 2, 0.5)`)
	mustExec(t, conn, `UPDATE plates SET eaters = 3.5 WHERE id = 10`)
}

func TestMigration00027_PresetPortionsBecomeReal(t *testing.T) {
	conn := setupMigration27DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 27))

	var typ string
	require.NoError(t, conn.QueryRow(`SELECT type FROM pragma_table_info('preset_components') WHERE name = 'portions'`).Scan(&typ))
	assert.Equal(t, "REAL", typ)
	var portions float64
	require.NoError(t, conn.QueryRow(`SELECT portions FROM preset_components WHERE id = 1`).Scan(&portions))
	assert.InDelta(t, 2.0, portions, 1e-9)
	var amount float64
	require.NoError(t, conn.QueryRow(`SELECT amount FROM preset_components WHERE id = 2`).Scan(&amount))
	assert.InDelta(t, 80.0, amount, 1e-9)
}

// schema27 returns the normalised CREATE statements of the tables and indexes
// 00027 touches.
func schema27(t *testing.T, conn *sql.DB) map[string]string {
	t.Helper()
	rows, err := conn.Query(`SELECT name, COALESCE(sql, '') FROM sqlite_master
		WHERE tbl_name IN ('plates', 'plate_components', 'preset_components', 'foods', 'cook_sessions')
		  AND name NOT LIKE 'sqlite_%'`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var name, stmt string
		require.NoError(t, rows.Scan(&name, &stmt))
		out[name] = stmt
	}
	require.NoError(t, rows.Err())
	return out
}

func columns27(t *testing.T, conn *sql.DB, table string) map[string]string {
	t.Helper()
	rows, err := conn.Query(`SELECT name, type FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var name, typ string
		require.NoError(t, rows.Scan(&name, &typ))
		out[name] = typ
	}
	require.NoError(t, rows.Err())
	return out
}

func TestMigration00027_DownUpRoundTrip(t *testing.T) {
	conn := setupMigration27DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 27))
	upSchema := schema27(t, conn)

	// A fractional portion is rounded half-up on the way down, minimum 1.
	mustExec(t, conn, `UPDATE plate_components SET portions = 0.4 WHERE id = 100`)
	mustExec(t, conn, `UPDATE plate_components SET portions = 2.5 WHERE id = 103`)
	require.NoError(t, goose.DownTo(conn, "migrations", 26))

	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'cook_sessions'`).Scan(&n))
	assert.Zero(t, n, "cook_sessions dropped")
	assert.Equal(t, "INTEGER", columns27(t, conn, "plate_components")["portions"])
	assert.Equal(t, "INTEGER", columns27(t, conn, "preset_components")["portions"])
	assert.NotContains(t, columns27(t, conn, "plate_components"), "cook_session_id")
	assert.NotContains(t, columns27(t, conn, "plates"), "eaters")
	assert.NotContains(t, columns27(t, conn, "plates"), "updated_at")
	assert.NotContains(t, columns27(t, conn, "foods"), "keep_days")
	assert.NotContains(t, columns27(t, conn, "foods"), "freezable")

	var p100, p103, p102 int
	require.NoError(t, conn.QueryRow(`SELECT portions FROM plate_components WHERE id = 100`).Scan(&p100))
	require.NoError(t, conn.QueryRow(`SELECT portions FROM plate_components WHERE id = 102`).Scan(&p102))
	require.NoError(t, conn.QueryRow(`SELECT portions FROM plate_components WHERE id = 103`).Scan(&p103))
	assert.Equal(t, 1, p100)
	assert.Equal(t, 2, p102)
	assert.Equal(t, 3, p103)

	require.NoError(t, goose.UpTo(conn, "migrations", 27))
	assert.Equal(t, upSchema, schema27(t, conn), "Down then Up gives the same schema")
}
