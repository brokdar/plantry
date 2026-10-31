package testhelper

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
)

const dsnPragmas = "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

var (
	templateOnce  sync.Once
	templateBytes []byte
	templateErr   error
)

// migratedTemplate runs all migrations once per test process and returns the
// resulting database file bytes. The scratch directory is removed right away,
// so nothing is left behind in os.TempDir().
func migratedTemplate() ([]byte, error) {
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "plantry-template-")
		if err != nil {
			templateErr = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()

		path := filepath.Join(dir, "template.db")
		conn, err := sql.Open("sqlite", "file:"+path+dsnPragmas)
		if err != nil {
			templateErr = err
			return
		}
		conn.SetMaxOpenConns(1)

		goose.SetBaseFS(db.Migrations)
		if err = goose.SetDialect("sqlite3"); err == nil {
			err = goose.Up(conn, "migrations")
		}
		// Closing the last connection checkpoints and removes the WAL file.
		if cerr := conn.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			templateErr = err
			return
		}
		templateBytes, templateErr = os.ReadFile(path)
	})
	return templateBytes, templateErr
}

// NewTestDB returns a fresh SQLite database with all migrations applied. The
// migrations run once per test process; each call gets its own copy of the
// migrated file. The connection is closed automatically when the test ends.
func NewTestDB(t testing.TB) *sql.DB {
	t.Helper()

	tmpl, err := migratedTemplate()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, tmpl, 0o600))

	conn, err := sql.Open("sqlite", "file:"+path+dsnPragmas)
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)

	t.Cleanup(func() { _ = conn.Close() })

	return conn
}
