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
	return conn
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
	var at string
	require.NoError(t, conn.QueryRow(`SELECT at FROM plate_events`).Scan(&at))
	assert.NotEmpty(t, at, "at defaults to now")
}

func TestMigration00030_DownUpRoundTrips(t *testing.T) {
	conn := openDBAt00029(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 30))
	require.NoError(t, goose.DownTo(conn, "migrations", 29))
	assert.False(t, columnExists(t, conn, "plate_events", "id"))
	assert.False(t, indexExists(t, conn, "ix_plate_events_plate"))

	require.NoError(t, goose.UpTo(conn, "migrations", 30))
	assert.True(t, columnExists(t, conn, "plate_events", "id"))
}
