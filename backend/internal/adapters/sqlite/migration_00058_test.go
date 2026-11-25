package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Migration 00058 adds food_attributes beside the existing foods; down drops
// only the new table.
func TestMigration00058_FoodAttributesUpAndDown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration58.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 57))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions) VALUES (1, 'Chili', 'composed', 4)`)

	require.NoError(t, goose.UpTo(conn, "migrations", 58))
	mustExec(t, conn, `INSERT INTO food_attributes (food_id, version, values_json, sources_json)
		VALUES (1, 1, '{"kind":"stew"}', '{"kind":"user"}')`)
	var values string
	require.NoError(t, conn.QueryRow(`SELECT values_json FROM food_attributes WHERE food_id = 1`).Scan(&values))
	assert.JSONEq(t, `{"kind":"stew"}`, values)

	require.NoError(t, goose.DownTo(conn, "migrations", 57))
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM foods`).Scan(&n))
	assert.Equal(t, 1, n, "the food stays")
	err = conn.QueryRow(`SELECT COUNT(*) FROM food_attributes`).Scan(&n)
	assert.ErrorContains(t, err, "no such table")
}
