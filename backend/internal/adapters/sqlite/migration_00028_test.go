package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// openDBAt00027 opens a fresh SQLite file migrated to 00027 holding anchor
// and week-start rows.
func openDBAt00027(t *testing.T, anchor string) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration28.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 27))
	mustExec(t, conn, `INSERT INTO app_settings (key, value) VALUES ('plan.anchor', ?)`, anchor)
	mustExec(t, conn, `INSERT INTO app_settings (key, value) VALUES ('plan.week_starts_on', 'sunday')`)
	mustExec(t, conn, `INSERT INTO app_settings (key, value) VALUES ('plan.shopping_day', '5')`)
	return conn
}

// B4-AC10 (storage half)
func TestMigration00028_FixedWeekdayBecomesNextShoppingDay(t *testing.T) {
	conn := openDBAt00027(t, "fixed_weekday")
	require.NoError(t, goose.UpTo(conn, "migrations", 28))

	v, ok := settingRow(t, conn, "plan.anchor")
	require.True(t, ok)
	assert.Equal(t, "next_shopping_day", v)
	_, ok = settingRow(t, conn, "plan.week_starts_on")
	assert.False(t, ok, "the week-start row is deleted")
	v, _ = settingRow(t, conn, "plan.shopping_day")
	assert.Equal(t, "5", v, "other rows are untouched")

	got, err := testsvc.Settings(conn).Get(context.Background(), settings.KeyPlanAnchor)
	require.NoError(t, err)
	assert.Equal(t, "next_shopping_day", got.Raw)
	assert.Equal(t, settings.SourceDB, got.Source)
}

func TestMigration00028_KeepsTodayAnchor(t *testing.T) {
	conn := openDBAt00027(t, "today")
	require.NoError(t, goose.UpTo(conn, "migrations", 28))
	v, _ := settingRow(t, conn, "plan.anchor")
	assert.Equal(t, "today", v)
}

func TestMigration00028_DownIsANoOp(t *testing.T) {
	conn := openDBAt00027(t, "today")
	require.NoError(t, goose.UpTo(conn, "migrations", 28))
	require.NoError(t, goose.DownTo(conn, "migrations", 27))
	v, _ := settingRow(t, conn, "plan.anchor")
	assert.Equal(t, "today", v)
}
