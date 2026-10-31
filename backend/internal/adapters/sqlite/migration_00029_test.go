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

// openDBAt00028 opens a fresh SQLite file migrated to 00028 holding a cooked
// recipe and four plates rated cooked, skipped, loved and disliked.
func openDBAt00028(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration29.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 28))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions, cook_count, last_cooked_at)
		VALUES (1, 'Chili', 'composed', 4, 3, '2026-10-20 18:00:00')`)
	mustExec(t, conn, `INSERT INTO time_slots (id, name_key, icon) VALUES (1, 'slot.dinner', 'moon')`)
	for i, status := range []string{"cooked", "skipped", "loved", "disliked"} {
		id := i + 1
		mustExec(t, conn, `INSERT INTO plates (id, slot_id, date) VALUES (?, 1, ?)`, id, "2026-10-2"+string(rune('0'+id)))
		mustExec(t, conn, `INSERT INTO plate_feedback (plate_id, status, note, rated_at) VALUES (?, ?, ?, '2026-10-25 12:00:00')`,
			id, status, "n"+status)
	}
	return conn
}

func columnExists(t *testing.T, conn *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n))
	return n == 1
}

func indexExists(t *testing.T, conn *sql.DB, name string) bool {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&n))
	return n == 1
}

func feedbackRows(t *testing.T, conn *sql.DB) map[int64]string {
	t.Helper()
	rows, err := conn.Query(`SELECT plate_id, status, note, rated_at FROM plate_feedback ORDER BY plate_id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var status, note, ratedAt string
		require.NoError(t, rows.Scan(&id, &status, &note, &ratedAt))
		assert.Equal(t, "2026-10-25 12:00:00", ratedAt, "rated_at is kept")
		out[id] = status + "/" + note
	}
	require.NoError(t, rows.Err())
	return out
}

// B7-AC7
func TestMigration00029_DropsCookCountersAndRewritesFeedback(t *testing.T) {
	conn := openDBAt00028(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 29))

	assert.False(t, columnExists(t, conn, "foods", "cook_count"))
	assert.False(t, columnExists(t, conn, "foods", "last_cooked_at"))
	assert.False(t, indexExists(t, conn, "ix_foods_last_cooked"))
	assert.True(t, indexExists(t, conn, "ix_plate_feedback_status"))

	assert.Equal(t, map[int64]string{
		2: "not_eaten/nskipped",
		3: "loved/nloved",
		4: "disliked/ndisliked",
	}, feedbackRows(t, conn), "skipped became not_eaten and cooked is gone")

	_, err := conn.Exec(`INSERT INTO plate_feedback (plate_id, status) VALUES (1, 'cooked')`)
	assert.Error(t, err, "cooked is no longer allowed")
	_, err = conn.Exec(`INSERT INTO plate_feedback (plate_id, status) VALUES (1, 'skipped')`)
	assert.Error(t, err, "skipped is no longer allowed")
	mustExec(t, conn, `INSERT INTO plate_feedback (plate_id, status) VALUES (1, 'not_eaten')`)

	mustExec(t, conn, `DELETE FROM plates WHERE id = 3`)
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM plate_feedback WHERE plate_id = 3`).Scan(&n))
	assert.Zero(t, n, "the rebuilt table still cascades from plates")
}

func TestMigration00029_DownUpRoundTrips(t *testing.T) {
	conn := openDBAt00028(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 29))
	require.NoError(t, goose.DownTo(conn, "migrations", 28))

	assert.True(t, columnExists(t, conn, "foods", "cook_count"))
	assert.True(t, columnExists(t, conn, "foods", "last_cooked_at"))
	assert.True(t, indexExists(t, conn, "ix_foods_last_cooked"))
	assert.Equal(t, map[int64]string{
		2: "skipped/nskipped",
		3: "loved/nloved",
		4: "disliked/ndisliked",
	}, feedbackRows(t, conn), "not_eaten goes back to skipped; deleted cooked rows stay gone")
	mustExec(t, conn, `INSERT INTO plate_feedback (plate_id, status) VALUES (1, 'cooked')`)
	mustExec(t, conn, `DELETE FROM plate_feedback WHERE plate_id = 1`)

	require.NoError(t, goose.UpTo(conn, "migrations", 29))
	assert.False(t, columnExists(t, conn, "foods", "cook_count"))
	assert.Equal(t, "not_eaten/nskipped", feedbackRows(t, conn)[2])
}
