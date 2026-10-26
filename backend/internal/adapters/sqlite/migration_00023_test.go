package sqlite_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// openDBAt00022 opens a fresh database migrated up to version 22.
func openDBAt00022(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile-prefs.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 22))
	return conn
}

func settingRow(t *testing.T, conn *sql.DB, key string) (string, bool) {
	t.Helper()
	var v string
	err := conn.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	require.NoError(t, err)
	return v, true
}

func profileColumns(t *testing.T, conn *sql.DB) map[string]bool {
	t.Helper()
	rows, err := conn.Query(`SELECT name FROM pragma_table_info('user_profile')`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	cols := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		cols[name] = true
	}
	require.NoError(t, rows.Err())
	return cols
}

func TestMigration00023_AddsColumnsWithDefaults(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	var (
		diet      string
		meatDays  sql.NullInt64
		household float64
	)
	err := conn.QueryRow(`SELECT diet_type, meat_days_per_week_max, household_portions FROM user_profile WHERE id = 1`).
		Scan(&diet, &meatDays, &household)
	require.NoError(t, err)
	assert.Equal(t, "omnivore", diet)
	assert.False(t, meatDays.Valid, "meat_days_per_week_max defaults to NULL")
	assert.InDelta(t, 1.0, household, 1e-9)
}

func TestMigration00023_FreshDBHasNoSetupCompleted(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	_, ok := settingRow(t, conn, "setup.completed")
	assert.False(t, ok, "fresh database must not have a setup.completed row")
}

func TestMigration00023_ExistingSlotMarksSetupCompleted(t *testing.T) {
	conn := openDBAt00022(t)
	_, err := conn.Exec(`INSERT INTO time_slots (name_key, icon) VALUES ('slot.lunch', 'sun')`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(conn, "migrations", 23))

	v, ok := settingRow(t, conn, "setup.completed")
	require.True(t, ok)
	assert.Equal(t, "true", v)
}

func TestMigration00023_DeletesWindowDays(t *testing.T) {
	conn := openDBAt00022(t)
	_, err := conn.Exec(`INSERT INTO app_settings (key, value) VALUES ('plan.window_days', '7')`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(conn, "migrations", 23))

	_, ok := settingRow(t, conn, "plan.window_days")
	assert.False(t, ok, "plan.window_days row should be deleted")
}

func TestMigration00023_ChecksRejectInvalidValues(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	tests := []struct {
		name    string
		stmt    string
		wantErr bool
	}{
		{"valid diet", `UPDATE user_profile SET diet_type = 'vegan' WHERE id = 1`, false},
		{"invalid diet", `UPDATE user_profile SET diet_type = 'keto' WHERE id = 1`, true},
		{"meat days 0", `UPDATE user_profile SET meat_days_per_week_max = 0 WHERE id = 1`, false},
		{"meat days 7", `UPDATE user_profile SET meat_days_per_week_max = 7 WHERE id = 1`, false},
		{"meat days 8", `UPDATE user_profile SET meat_days_per_week_max = 8 WHERE id = 1`, true},
		{"meat days -1", `UPDATE user_profile SET meat_days_per_week_max = -1 WHERE id = 1`, true},
		{"household zero", `UPDATE user_profile SET household_portions = 0 WHERE id = 1`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := conn.Exec(tt.stmt)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}

	t.Run("insert with invalid diet", func(t *testing.T) {
		_, err := conn.Exec(`DELETE FROM user_profile`)
		require.NoError(t, err)
		_, err = conn.Exec(`INSERT INTO user_profile (id, diet_type) VALUES (1, 'keto')`)
		assert.Error(t, err)
	})
}

// TestMigration00023_UpDownUp verifies the migration is reversible.
func TestMigration00023_UpDownUp(t *testing.T) {
	conn := openDBAt00022(t)
	_, err := conn.Exec(`INSERT INTO time_slots (name_key, icon) VALUES ('slot.lunch', 'sun')`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(conn, "migrations", 23))
	require.NoError(t, goose.DownTo(conn, "migrations", 22))

	cols := profileColumns(t, conn)
	assert.False(t, cols["diet_type"])
	assert.False(t, cols["meat_days_per_week_max"])
	assert.False(t, cols["household_portions"])
	_, ok := settingRow(t, conn, "setup.completed")
	assert.False(t, ok, "setup.completed should be removed on down")

	require.NoError(t, goose.UpTo(conn, "migrations", 23))
	cols = profileColumns(t, conn)
	assert.True(t, cols["diet_type"])
	assert.True(t, cols["meat_days_per_week_max"])
	assert.True(t, cols["household_portions"])
}
