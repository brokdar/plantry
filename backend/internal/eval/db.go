package eval

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
)

// The pragmas of the app's database (cmd/plantry openDB).
const dsnPragmas = "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

// dbs hands out fresh migrated databases in one scratch directory: the
// migrations run once, into a template file every database copies.
type dbs struct {
	dir  string
	keep bool // dir is the caller's: the run databases stay (-keep-db)
	once sync.Once
	tmpl []byte
	err  error
	mu   sync.Mutex
	n    int
}

// newDBs makes the scratch directory; with keepDir set the databases go
// there instead and survive close, named <scenario>-<run>.db, so a live
// run's steps can be audited afterwards.
func newDBs(keepDir string) (*dbs, error) {
	if keepDir != "" {
		if err := os.MkdirAll(keepDir, 0o750); err != nil {
			return nil, err
		}
		return &dbs{dir: keepDir, keep: true}, nil
	}
	dir, err := os.MkdirTemp("", "plantry-eval-")
	if err != nil {
		return nil, err
	}
	return &dbs{dir: dir}, nil
}

// close removes every database handed out, except in keep mode, which only
// drops the migrated template.
func (d *dbs) close() error {
	if d.keep {
		return os.RemoveAll(filepath.Join(d.dir, "template.db"))
	}
	return os.RemoveAll(d.dir)
}

func (d *dbs) template() ([]byte, error) {
	d.once.Do(func() {
		path := filepath.Join(d.dir, "template.db")
		conn, err := sql.Open("sqlite", "file:"+path+dsnPragmas)
		if err != nil {
			d.err = err
			return
		}
		conn.SetMaxOpenConns(1)
		goose.SetBaseFS(db.Migrations)
		goose.SetLogger(goose.NopLogger())
		if err = goose.SetDialect("sqlite3"); err == nil {
			err = goose.Up(conn, "migrations")
		}
		// Closing the last connection checkpoints the WAL into the file.
		if cerr := conn.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			d.err = fmt.Errorf("migrate: %w", err)
			return
		}
		d.tmpl, d.err = os.ReadFile(path)
	})
	return d.tmpl, d.err
}

// open returns a fresh migrated database; label names its file in keep mode.
func (d *dbs) open(label string) (*sql.DB, error) {
	tmpl, err := d.template()
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.n++
	path := filepath.Join(d.dir, fmt.Sprintf("run-%d.db", d.n))
	if d.keep {
		path = filepath.Join(d.dir, label+".db")
	}
	d.mu.Unlock()
	// A -wal/-shm left by an earlier run of the same label would be replayed
	// against the fresh file.
	for _, sfx := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + sfx); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	if err := os.WriteFile(path, tmpl, 0o600); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", "file:"+path+dsnPragmas)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	return conn, nil
}
