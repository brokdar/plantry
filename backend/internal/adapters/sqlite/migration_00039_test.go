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

// openDBAt00039 opens a fresh SQLite file migrated to 00039 holding one AI
// run (id 1) and one food (id 1).
func openDBAt00039(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration39.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 38))
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at) VALUES (1, 'discovery', '2026-11-01 10:00:00')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions) VALUES (1, 'Fesenjan', 'composed', 4)`)
	require.NoError(t, goose.UpTo(conn, "migrations", 39))
	return conn
}

const insertSuggestion = `INSERT INTO recipe_suggestions (mode, source_url, draft_json, annotations_json, grounded`

func TestMigration00039_Defaults(t *testing.T) {
	conn := openDBAt00039(t)
	mustExec(t, conn, insertSuggestion+`) VALUES ('explore', 'https://x.test/a', '{}', '{}', 1)`)
	var (
		status, createdAt string
		image, reason     sql.NullString
		decided, shownOn  sql.NullString
		query             sql.NullString
	)
	require.NoError(t, conn.QueryRow(`SELECT status, created_at, image_path, reason, decided_at, shown_on, query FROM recipe_suggestions`).
		Scan(&status, &createdAt, &image, &reason, &decided, &shownOn, &query))
	assert.Equal(t, "pending", status)
	assert.NotEmpty(t, createdAt)
	assert.False(t, image.Valid || reason.Valid || decided.Valid || shownOn.Valid || query.Valid)
}

func TestMigration00039_Checks(t *testing.T) {
	conn := openDBAt00039(t)
	tests := []struct{ name, stmt string }{
		{"unknown mode", insertSuggestion + `) VALUES ('weekly', 'u', '{}', '{}', 0)`},
		{"unknown status", insertSuggestion + `, status) VALUES ('daily', 'u', '{}', '{}', 0, 'maybe')`},
		{"unknown reason", insertSuggestion + `, reason) VALUES ('daily', 'u', '{}', '{}', 0, 'boring')`},
		{"grounded not boolean", insertSuggestion + `) VALUES ('daily', 'u', '{}', '{}', 2)`},
		{"no source url", `INSERT INTO recipe_suggestions (mode, draft_json, annotations_json, grounded) VALUES ('daily', '{}', '{}', 0)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := conn.Exec(tt.stmt)
			assert.Error(t, err)
		})
	}
	for _, mode := range []string{"daily", "explore", "known"} {
		mustExec(t, conn, insertSuggestion+`) VALUES ('`+mode+`', 'u', '{}', '{}', 0)`)
	}
	for _, status := range []string{"pending", "accepted", "rejected"} {
		mustExec(t, conn, insertSuggestion+`, status) VALUES ('daily', 'u', '{}', '{}', 0, '`+status+`')`)
	}
	for _, reason := range []string{"too_long", "ingredients", "taste"} {
		mustExec(t, conn, insertSuggestion+`, reason) VALUES ('daily', 'u', '{}', '{}', 0, '`+reason+`')`)
	}
}

func TestMigration00039_ShownOnUniqueWhereSet(t *testing.T) {
	conn := openDBAt00039(t)
	mustExec(t, conn, insertSuggestion+`) VALUES ('daily', 'a', '{}', '{}', 0)`)
	mustExec(t, conn, insertSuggestion+`) VALUES ('daily', 'b', '{}', '{}', 0)`) // NULL shown_on repeats
	mustExec(t, conn, insertSuggestion+`, shown_on) VALUES ('daily', 'c', '{}', '{}', 0, '2026-11-02')`)
	_, err := conn.Exec(insertSuggestion + `, shown_on) VALUES ('daily', 'd', '{}', '{}', 0, '2026-11-02')`)
	assert.Error(t, err, "one suggestion per shown day")
}

func TestMigration00039_RunAndFoodSetNull(t *testing.T) {
	conn := openDBAt00039(t)
	mustExec(t, conn, insertSuggestion+`, ai_run_id, food_id) VALUES ('daily', 'u', '{}', '{}', 0, 1, 1)`)

	_, err := conn.Exec(insertSuggestion + `, ai_run_id) VALUES ('daily', 'v', '{}', '{}', 0, 99)`)
	assert.Error(t, err, "ai_run_id references ai_runs")
	_, err = conn.Exec(insertSuggestion + `, food_id) VALUES ('daily', 'w', '{}', '{}', 0, 99)`)
	assert.Error(t, err, "food_id references foods")

	mustExec(t, conn, `DELETE FROM ai_runs WHERE id = 1`)
	mustExec(t, conn, `DELETE FROM foods WHERE id = 1`)
	var run, food sql.NullInt64
	require.NoError(t, conn.QueryRow(`SELECT ai_run_id, food_id FROM recipe_suggestions WHERE source_url = 'u'`).Scan(&run, &food))
	assert.False(t, run.Valid, "the suggestion outlives its run")
	assert.False(t, food.Valid, "and its food")
}

func TestMigration00039_Down(t *testing.T) {
	conn := openDBAt00039(t)
	require.NoError(t, goose.Down(conn, "migrations"))
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name LIKE '%recipe_suggestions%'`).Scan(&n))
	assert.Zero(t, n)
}
