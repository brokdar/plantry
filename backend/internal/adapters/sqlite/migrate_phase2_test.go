package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Phase2Fixture is a Phase 2-era database (see testdata/README.md).
const Phase2Fixture = "testdata/phase2.db"

func scalar(t *testing.T, conn *sql.DB, q string) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(q).Scan(&n), q)
	return n
}

// Z4-AC2 (AC-7.2): the Phase 2 database migrates to head with its data and
// invariants intact.
func TestMigratePhase2FixtureToHead(t *testing.T) {
	conn := testhelper.CopyDB(t, Phase2Fixture)
	counts := map[string]int{
		"foods": 20, "food_components": 36, "time_slots": 3, "plates": 20, "plate_components": 20,
		"cook_sessions": 20, "presets": 2, "preset_plates": 2, "availability_defaults": 2, "ai_conversations": 1,
	}
	before := map[string]int{}
	for table := range counts {
		before[table] = scalar(t, conn, "SELECT COUNT(*) FROM "+table)
		assert.Equal(t, counts[table], before[table], "fixture "+table)
	}
	require.Equal(t, 33, scalar(t, conn, "SELECT MAX(version_id) FROM goose_db_version"))

	testhelper.MigrateDB(t, conn)

	assert.Greater(t, scalar(t, conn, "SELECT MAX(version_id) FROM goose_db_version"), 33)
	for _, table := range []string{"foods", "time_slots", "plates", "plate_components", "cook_sessions", "presets", "availability_defaults", "ai_conversations"} {
		assert.Equal(t, before[table], scalar(t, conn, "SELECT COUNT(*) FROM "+table), table)
	}
	assert.Equal(t, 12, scalar(t, conn, "SELECT COUNT(*) FROM foods WHERE kind = 'composed'"))
	assert.Equal(t, 10, scalar(t, conn, "SELECT COUNT(*) FROM cook_sessions WHERE cooked_at IS NOT NULL"))

	// Invariants.
	assert.Zero(t, scalar(t, conn, "SELECT COUNT(*) FROM pragma_foreign_key_check"), "foreign keys")
	assert.Zero(t, scalar(t, conn, `SELECT COUNT(*) FROM plate_components pc LEFT JOIN plates p ON p.id = pc.plate_id WHERE p.id IS NULL`), "orphan components")
	assert.Zero(t, scalar(t, conn, `SELECT COUNT(*) FROM (SELECT date, slot_id FROM plates GROUP BY date, slot_id HAVING COUNT(*) > 1)`), "one plate per cell")
	assert.Zero(t, scalar(t, conn, `SELECT COUNT(*) FROM plate_components WHERE cook_session_id IS NOT NULL
		AND cook_session_id NOT IN (SELECT id FROM cook_sessions)`), "sessions intact")
	assert.Zero(t, scalar(t, conn, `SELECT COUNT(*) FROM cook_sessions cs WHERE NOT EXISTS
		(SELECT 1 FROM plate_components pc WHERE pc.cook_session_id = cs.id)`), "session without a plate")
	var ok string
	require.NoError(t, conn.QueryRow("PRAGMA integrity_check").Scan(&ok))
	assert.Equal(t, "ok", ok)
}
