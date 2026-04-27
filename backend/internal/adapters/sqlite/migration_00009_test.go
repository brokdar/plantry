package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
)

// TestMigration00009_UpDownUp verifies that the plate_feedback migration is
// reversible: goose up → down → up leaves a schema the app can query without
// error.
func TestMigration00009_UpDownUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback-roundtrip.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))

	require.NoError(t, goose.Up(conn, "migrations"))

	// Seed: need a slot + plate to attach feedback to (FK cascade target).
	// weeks table was dropped in migration 16; plates no longer has week_id/day.
	_, err = conn.Exec(`INSERT INTO time_slots (name_key, icon) VALUES ('slot.dinner', 'utensils')`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO plates (slot_id, date) VALUES (1, '2025-12-29')`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO plate_feedback (plate_id, status) VALUES (1, 'loved')`)
	require.NoError(t, err)

	require.NoError(t, goose.DownTo(conn, "migrations", 8))

	var exists int
	err = conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='plate_feedback'`).Scan(&exists)
	require.NoError(t, err)
	require.Equal(t, 0, exists, "plate_feedback should be dropped on down")

	require.NoError(t, goose.Up(conn, "migrations"))
	_, err = conn.Exec(`INSERT INTO plate_feedback (plate_id, status) VALUES (1, 'cooked')`)
	require.NoError(t, err)
}
