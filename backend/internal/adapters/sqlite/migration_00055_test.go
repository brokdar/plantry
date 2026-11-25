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

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Migration 00055 rebuilds ai_runs (kind CHECK dropped, cost_usd added) and
// keeps every run, step, change and link to a run: plates, cook sessions and
// suggestions still point at their runs, and steps and changes were not
// cascade-deleted.
func TestMigration00055_RebuildKeepsRunsAndLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration55.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 54))

	mustExec(t, conn, `INSERT INTO ai_prompts (hash, text) VALUES ('h1', 'system')`)
	// Run 3: priced and finished; run 7: unpriced model; run 9: still running.
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at, finished_at, status, provider, model, prompt_hash,
		input_tokens, output_tokens, cache_read_tokens, notes, container_id)
		VALUES (3, 'generate', '2026-11-01T10:00:00.000Z', '2026-11-01T10:01:00.000Z', 'ok', 'anthropic', 'claude-sonnet-5-5', 'h1',
		1000, 500, 2000, 'note', 'c1')`)
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at, finished_at, status, provider, model, input_tokens)
		VALUES (7, 'chat', '2026-11-02T10:00:00.000Z', '2026-11-02T10:01:00.000Z', 'ok', 'openai', 'gpt-6.2', 10)`)
	mustExec(t, conn, `INSERT INTO ai_runs (id, kind, started_at) VALUES (9, 'discovery', '2026-11-03T10:00:00.000Z')`)
	mustExec(t, conn, `INSERT INTO ai_run_steps (run_id, seq, type, name) VALUES (3, 1, 'llm_call', 'm'), (3, 2, 'tool_call', 't'), (7, 1, 'llm_call', 'm')`)
	mustExec(t, conn, `INSERT INTO ai_changes (run_id, step_seq, tool, text_key, params_json, run_ids_json, dates_json, plate_ids_json, session_ids_json)
		VALUES (7, 1, 'add_plate', 'k', '{}', '[7]', '[]', '[5]', '[]')`)
	mustExec(t, conn, `INSERT INTO time_slots (id, name_key, icon) VALUES (1, 'slot.dinner', 'moon')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions) VALUES (1, 'Chili', 'composed', 4)`)
	mustExec(t, conn, `INSERT INTO cook_sessions (id, food_id, cook_date, cook_slot_id, portions, ai_run_id) VALUES (4, 1, '2026-11-02', 1, 4, 3)`)
	mustExec(t, conn, `INSERT INTO plates (id, slot_id, date, eaters, ai_run_id) VALUES (5, 1, '2026-11-02', 2, 3)`)
	mustExec(t, conn, `INSERT INTO recipe_suggestions (id, mode, source_url, draft_json, annotations_json, grounded, ai_run_id)
		VALUES (6, 'daily', 'https://x', '{}', '{}', 1, 9)`)

	require.NoError(t, goose.UpTo(conn, "migrations", 55))

	count := func(q string) int {
		t.Helper()
		var n int
		require.NoError(t, conn.QueryRow(q).Scan(&n))
		return n
	}
	assert.Equal(t, 3, count(`SELECT COUNT(*) FROM ai_runs`))
	assert.Equal(t, 3, count(`SELECT COUNT(*) FROM ai_run_steps`))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM ai_changes WHERE run_id = 7`))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM plates WHERE id = 5 AND ai_run_id = 3`))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM cook_sessions WHERE id = 4 AND ai_run_id = 3`))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM recipe_suggestions WHERE id = 6 AND ai_run_id = 9`))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM ai_runs WHERE id = 3 AND prompt_hash = 'h1' AND notes = 'note'
		AND container_id = 'c1' AND cache_read_tokens = 2000 AND finished_at = '2026-11-01T10:01:00.000Z'`))
	assert.Equal(t, 0, count(`SELECT COUNT(*) FROM pragma_foreign_key_check`))
	assert.Equal(t, 1, count(`PRAGMA foreign_keys`), "foreign keys are back on")
	assert.Equal(t, 2, count(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name IN ('ix_ai_runs_started', 'ix_ai_runs_conversation')`))

	// Finished runs are priced from the table (Sonnet 5.5: $2 in, $10 out,
	// $0.10 cache reads); unpriced and running runs stay NULL.
	costs := map[int64]sql.NullFloat64{}
	rows, err := conn.Query(`SELECT id, cost_usd FROM ai_runs`)
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		var c sql.NullFloat64
		require.NoError(t, rows.Scan(&id, &c))
		costs[id] = c
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.True(t, costs[3].Valid)
	assert.InDelta(t, (1000*2+500*10+2000*0.10)/1e6, costs[3].Float64, 1e-12)
	assert.False(t, costs[7].Valid, "gpt-6.2 has no price")
	assert.False(t, costs[9].Valid, "a running run has no cost yet")

	// The kind CHECK is gone: Go validates kinds now.
	mustExec(t, conn, `INSERT INTO ai_runs (kind, started_at) VALUES ('chef_find', '2026-11-04T10:00:00.000Z')`)

	// Deleting a run still cascades and nulls its links.
	mustExec(t, conn, `DELETE FROM ai_runs WHERE id = 3`)
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM ai_run_steps`))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM plates WHERE id = 5 AND ai_run_id IS NULL`))

	require.NoError(t, goose.DownTo(conn, "migrations", 54))
	assert.False(t, columnExists(t, conn, "ai_runs", "cost_usd"))
}
