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
)

// TestMigration00018_PlateComponentQuantity verifies that the kind-aware
// quantity rebuild backfills cleanly: composed rows keep an integer portions
// count (rounded half-up), leaf rows pivot to (amount=portions×100, unit='g',
// grams, grams_source='direct'). The XOR CHECK constraint must reject any row
// that sets both shapes or neither, and the up→down→up cycle must leave the
// schema equivalent to a single up.
func TestMigration00018_PlateComponentQuantity(t *testing.T) {
	conn := openMigration18DB(t)

	// Apply migrations 1..17 (pre-rework).
	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 17))

	// Seed: two foods (1 leaf, 1 composed), one plate, two components.
	_, err := conn.Exec(`
		INSERT INTO foods (id, name, kind, source, kcal_100g)
		VALUES (1, 'Rice', 'leaf', 'manual', 130);
		INSERT INTO foods (id, name, kind, role, reference_portions)
		VALUES (2, 'Curry', 'composed', 'main', 4);
		INSERT INTO time_slots (id, name_key, icon) VALUES (1, 'slot.dinner', '🍽');
		INSERT INTO plates (id, slot_id, date) VALUES (1, 1, '2026-04-26');
		INSERT INTO plate_components (id, plate_id, food_id, portions, sort_order)
		VALUES
			(1, 1, 1, 1.5, 0),  -- leaf: 1.5 → amount=150g, grams=150
			(2, 1, 2, 2.5, 1);  -- composed: 2.5 → portions=3 (round half-up)
	`)
	require.NoError(t, err)

	// Apply 00018.
	require.NoError(t, goose.UpTo(conn, "migrations", 18))

	t.Run("up_creates_columns", func(t *testing.T) {
		cols := tableInfo(t, conn, "plate_components")
		require.Contains(t, cols, "amount")
		require.Contains(t, cols, "unit")
		require.Contains(t, cols, "grams")
		require.Contains(t, cols, "grams_source")
		require.Equal(t, "INTEGER", cols["portions"], "portions must be INTEGER post-up")
	})

	t.Run("up_backfills_leaf_to_grams", func(t *testing.T) {
		var (
			portions sql.NullInt64
			amount   sql.NullFloat64
			unit     sql.NullString
			grams    sql.NullFloat64
			source   sql.NullString
		)
		require.NoError(t, conn.QueryRow(
			`SELECT portions, amount, unit, grams, grams_source FROM plate_components WHERE id = 1`,
		).Scan(&portions, &amount, &unit, &grams, &source))
		assert.False(t, portions.Valid, "leaf rows must have portions = NULL")
		assert.True(t, amount.Valid)
		assert.InDelta(t, 150.0, amount.Float64, 0.001)
		assert.Equal(t, "g", unit.String)
		assert.InDelta(t, 150.0, grams.Float64, 0.001)
		assert.Equal(t, "direct", source.String)
	})

	t.Run("up_backfills_composed_to_int_portions", func(t *testing.T) {
		var (
			portions sql.NullInt64
			amount   sql.NullFloat64
			unit     sql.NullString
		)
		require.NoError(t, conn.QueryRow(
			`SELECT portions, amount, unit FROM plate_components WHERE id = 2`,
		).Scan(&portions, &amount, &unit))
		assert.True(t, portions.Valid)
		// 2.5 rounds half-up to 3.
		assert.Equal(t, int64(3), portions.Int64)
		assert.False(t, amount.Valid, "composed rows must have amount = NULL")
		assert.False(t, unit.Valid, "composed rows must have unit = NULL")
	})

	t.Run("xor_check_rejects_both_shapes", func(t *testing.T) {
		_, err := conn.Exec(`
			INSERT INTO plate_components (plate_id, food_id, portions, amount, unit, grams, sort_order)
			VALUES (1, 1, 1, 100, 'g', 100, 99)
		`)
		require.Error(t, err, "row with both portions and leaf shape must be rejected")
	})

	t.Run("xor_check_rejects_neither_shape", func(t *testing.T) {
		_, err := conn.Exec(`
			INSERT INTO plate_components (plate_id, food_id, sort_order)
			VALUES (1, 1, 100)
		`)
		require.Error(t, err, "row with no quantity shape must be rejected")
	})

	t.Run("xor_check_accepts_composed_only", func(t *testing.T) {
		_, err := conn.Exec(`
			INSERT INTO plate_components (plate_id, food_id, portions, sort_order)
			VALUES (1, 2, 1, 200)
		`)
		require.NoError(t, err)
	})

	t.Run("xor_check_accepts_leaf_only", func(t *testing.T) {
		_, err := conn.Exec(`
			INSERT INTO plate_components (plate_id, food_id, amount, unit, grams, grams_source, sort_order)
			VALUES (1, 1, 50, 'g', 50, 'direct', 201)
		`)
		require.NoError(t, err)
	})

	// Down → re-up cycle: schema after second up must match first.
	t.Run("up_down_up_idempotent", func(t *testing.T) {
		require.NoError(t, goose.Down(conn, "migrations"))
		// After down, plate_components is the legacy single-portions shape:
		// portions REAL, no amount/unit/grams/grams_source.
		cols := tableInfo(t, conn, "plate_components")
		assert.NotContains(t, cols, "amount", "amount must be gone after down")
		assert.Equal(t, "REAL", cols["portions"], "portions must be REAL post-down")

		// The leaf row's portions must back-translate from grams: 150 / 100 = 1.5.
		var portions float64
		require.NoError(t, conn.QueryRow(
			`SELECT portions FROM plate_components WHERE id = 1`,
		).Scan(&portions))
		assert.InDelta(t, 1.5, portions, 0.001)

		// Up again, schema must be equivalent.
		require.NoError(t, goose.UpTo(conn, "migrations", 18))
		cols2 := tableInfo(t, conn, "plate_components")
		assert.Equal(t, "INTEGER", cols2["portions"])
		assert.Contains(t, cols2, "amount")
		assert.Contains(t, cols2, "unit")
		assert.Contains(t, cols2, "grams")
		assert.Contains(t, cols2, "grams_source")
	})
}

func openMigration18DB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration-00018.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// tableInfo returns a column-name → declared-type map via PRAGMA table_info.
func tableInfo(t *testing.T, conn *sql.DB, name string) map[string]string {
	t.Helper()
	rows, err := conn.Query(`SELECT name, type FROM pragma_table_info(?)`, name)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var n, ty string
		require.NoError(t, rows.Scan(&n, &ty))
		out[n] = ty
	}
	require.NoError(t, rows.Err())
	return out
}
