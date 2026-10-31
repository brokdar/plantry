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

// openDBAt00033WithPlan opens a fresh SQLite file migrated to 00033 holding
// session 3 (Chili) eaten by plate 5, and AI run 9.
func openDBAt00033WithPlan(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration34.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 33))
	mustExec(t, conn, `INSERT INTO time_slots (id, name_key, icon) VALUES (1, 'slot.dinner', 'moon')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions) VALUES (1, 'Chili', 'composed', 4)`)
	mustExec(t, conn, `INSERT INTO cook_sessions (id, food_id, cook_date, cook_slot_id, portions) VALUES (3, 1, '2026-11-02', 1, 4)`)
	mustExec(t, conn, `INSERT INTO plates (id, slot_id, date, eaters) VALUES (5, 1, '2026-11-02', 2)`)
	mustExec(t, conn, `INSERT INTO plate_components (id, plate_id, food_id, portions, cook_session_id, sort_order) VALUES (7, 5, 1, 1, 3, 0)`)
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at) VALUES (9, 'generate', '2026-11-01 10:00:00')`)
	return conn
}

// B1-AC1: existing plates and sessions become confirmed, with no run and no
// reason.
func TestMigration00034_ExistingRowsConfirmed(t *testing.T) {
	conn := openDBAt00033WithPlan(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 34))

	for _, table := range []string{"plates", "cook_sessions"} {
		var state string
		var run sql.NullInt64
		var reason sql.NullString
		require.NoError(t, conn.QueryRow(`SELECT state, ai_run_id, reason FROM `+table).Scan(&state, &run, &reason), table)
		assert.Equal(t, "confirmed", state, table)
		assert.False(t, run.Valid, table)
		assert.False(t, reason.Valid, table)
	}
	assert.True(t, indexExists(t, conn, "ix_plates_state_date"))
	assert.True(t, indexExists(t, conn, "ix_cook_sessions_ai_run"))
}

func TestMigration00034_StateCheckAndRunFK(t *testing.T) {
	conn := openDBAt00033WithPlan(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 34))

	for _, table := range []string{"plates", "cook_sessions"} {
		_, err := conn.Exec(`UPDATE ` + table + ` SET state = 'locked'`)
		assert.Error(t, err, table+": unknown state")
		mustExec(t, conn, `UPDATE `+table+` SET state = 'draft', ai_run_id = 9, reason = 'quick'`)
		_, err = conn.Exec(`UPDATE ` + table + ` SET ai_run_id = 404`)
		assert.Error(t, err, table+": unknown run")
	}

	// Deleting a run keeps its rows and clears their run id.
	mustExec(t, conn, `DELETE FROM ai_runs WHERE id = 9`)
	for _, table := range []string{"plates", "cook_sessions"} {
		var state string
		var run sql.NullInt64
		require.NoError(t, conn.QueryRow(`SELECT state, ai_run_id FROM `+table).Scan(&state, &run), table)
		assert.Equal(t, "draft", state, table)
		assert.False(t, run.Valid, table)
	}
}

// B1-AC1: down drops the columns and the indexes.
func TestMigration00034_Down(t *testing.T) {
	conn := openDBAt00033WithPlan(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 34))
	require.NoError(t, goose.DownTo(conn, "migrations", 33))

	for _, table := range []string{"plates", "cook_sessions"} {
		for _, col := range []string{"state", "ai_run_id", "reason"} {
			assert.False(t, columnExists(t, conn, table, col), table+"."+col)
		}
	}
	assert.False(t, indexExists(t, conn, "ix_plates_state_date"))
	assert.False(t, indexExists(t, conn, "ix_cook_sessions_ai_run"))
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plate_components WHERE cook_session_id = 3`).Scan(&n))
	assert.Equal(t, 1, n)
}
