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
	once sync.Once
	tmpl []byte
	err  error
	mu   sync.Mutex
	n    int
}

func newDBs() (*dbs, error) {
	dir, err := os.MkdirTemp("", "plantry-eval-")
	if err != nil {
		return nil, err
	}
	return &dbs{dir: dir}, nil
}

// close removes every database handed out.
func (d *dbs) close() error { return os.RemoveAll(d.dir) }

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

// open returns a fresh migrated database.
func (d *dbs) open() (*sql.DB, error) {
	tmpl, err := d.template()
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.n++
	path := filepath.Join(d.dir, fmt.Sprintf("run-%d.db", d.n))
	d.mu.Unlock()
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
