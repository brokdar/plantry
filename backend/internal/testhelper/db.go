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

// dsnPragmas are the app's pragmas (cmd/plantry openDB) plus synchronous(OFF):
// a test database needs no durability, and an fsync on every commit made the
// database-bound packages wait on the disk (planning 50 s → 8 s without it).
const dsnPragmas = "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(OFF)"

// DSN is the data source name a test opens a database file with: the app's
// pragmas, without fsync. Tests never build their own SQLite DSN
// (scripts/check-test-patterns.sh).
func DSN(path string) string { return "file:" + path + dsnPragmas }

// DSNWith opens path with the given pragmas (a query string without "?"),
// for tests that need other pragmas than the app's; fsync stays off.
func DSNWith(path, query string) string {
	return "file:" + path + "?" + query + "&_pragma=synchronous(OFF)"
}

var (
	templateOnce  sync.Once
	templateBytes []byte
	templateErr   error
)

// gooseMu serialises every migration of the test process: goose keeps its
// filesystem, dialect and logger in package variables, so two migrations
// (parallel tests, or the template next to a test's own) may not set them at once.
var gooseMu sync.Mutex

func migrate(conn *sql.DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(conn, "migrations")
}

// Migrate brings conn up to the latest schema. Tests migrate through it, never
// through goose directly, so the package-level goose state is set under one lock.
func Migrate(t testing.TB, conn *sql.DB) {
	t.Helper()
	require.NoError(t, migrate(conn))
}

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
		conn, err := sql.Open("sqlite", DSN(path))
		if err != nil {
			templateErr = err
			return
		}
		conn.SetMaxOpenConns(1)

		err = migrate(conn)
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
	return NewTestDBFrom(t, tmpl)
}

// NewTestDBFrom returns a database on its own copy of src, a SnapshotDB
// result: a fixture built once per test process and copied per test is far
// cheaper than building it again in every test.
func NewTestDBFrom(t testing.TB, src []byte) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, src, 0o600))

	conn, err := sql.Open("sqlite", DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)

	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// SnapshotDB returns a consistent copy of db's file (VACUUM INTO), to build a
// fixture once and hand it to NewTestDBFrom.
func SnapshotDB(t testing.TB, db *sql.DB) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.db")
	_, err := db.Exec(`VACUUM INTO ?`, path)
	require.NoError(t, err)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}
