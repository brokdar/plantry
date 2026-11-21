package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
)

// txKey carries the unit of work (*unit) in a context.
type txKey struct{}

// unit is the unit of work a context carries: the transaction and the
// statements prepared on it. A chat tool call reads the plan state about ten
// times in one transaction; parsing the same SQL again each time was 17% of
// the agent's CPU. Statements prepared on a transaction close with it.
type unit struct {
	tx    *sql.Tx
	seq   uint64 // unique per unit of work for the life of the process
	mu    sync.Mutex
	stmts map[string]*sql.Stmt
}

// units numbers the units of work (unit.seq).
var units atomic.Uint64

// TxRunner is the unit of work (D21). InTx runs fn in one transaction whose
// *sql.Tx travels in fn's context; every repository resolves its queries
// through that context, so all repos called inside fn share the transaction.
// It satisfies domain.Transactor.
type TxRunner struct {
	db *sql.DB
}

// NewTxRunner creates a TxRunner bound to db.
func NewTxRunner(db *sql.DB) *TxRunner {
	return &TxRunner{db: db}
}

// InTx runs fn in a transaction: commit when fn returns nil, roll back
// otherwise. A nested call joins the outer transaction, so only the outermost
// InTx commits; there are no savepoints, so an outer fn must not swallow a
// nested error after partial writes.
func (t *TxRunner) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return inTx(ctx, t.db, fn)
}

func inTx(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*unit); ok {
		return fn(ctx)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(context.WithValue(ctx, txKey{}, &unit{tx: tx, seq: units.Add(1), stmts: map[string]*sql.Stmt{}})); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// conn is embedded by every repository. It resolves the database handle for
// a call: the unit of work's transaction when ctx carries one, else the pool.
// Generated queries reuse prepared statements (queriesFor); hand-written SQL
// does not (dbtx).
type conn struct {
	db    *sql.DB
	stmts *stmts
}

// newConn is the conn of a repository over db.
func newConn(db *sql.DB) conn { return conn{db: db, stmts: &stmts{db: db, m: map[string]*sql.Stmt{}}} }

// dbtx returns the transaction in ctx, or the pool, for hand-written SQL.
// It never reuses statements: a caller may run the same query again while
// its earlier rows are open, and two cursors on one SQLite statement corrupt
// each other (TestConn_SameRawQueryWhileItsRowsAreOpen).
func (c conn) dbtx(ctx context.Context) sqlcgen.DBTX {
	if u, ok := ctx.Value(txKey{}).(*unit); ok {
		return u.tx
	}
	return c.db
}

// queriesFor returns sqlc queries bound to the transaction in ctx, or to the
// pool, with prepared statements reused (see stmts and txDB). Safe because a
// generated query reads and closes its rows before it returns, and a unit of
// work is used by one goroutine at a time, so no statement is used again
// while its rows are open.
func (c conn) queriesFor(ctx context.Context) *sqlcgen.Queries {
	if u, ok := ctx.Value(txKey{}).(*unit); ok {
		return sqlcgen.New(txDB{u: u, pool: c.stmts})
	}
	if c.stmts == nil {
		return sqlcgen.New(c.db)
	}
	return sqlcgen.New(c.stmts)
}

// inTx runs a multi-statement write atomically, joining the caller's unit of
// work when there is one.
func (c conn) inTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return inTx(ctx, c.db, fn)
}

// maxStmts bounds a cache: queries built at runtime (filters, IN lists) have
// many texts; past the bound they run unprepared.
const maxStmts = 256

// cacheable reports whether q is one statement: Prepare compiles only the
// first statement of a multi-statement string, Exec runs them all.
func cacheable(q string) bool {
	return !strings.Contains(strings.TrimRight(q, "; \t\n"), ";")
}

// stmts is a repository's pool statements, prepared on first use.
type stmts struct {
	db *sql.DB
	mu sync.Mutex
	m  map[string]*sql.Stmt
}

// get returns the prepared statement for q, or nil to run q unprepared.
func (s *stmts) get(ctx context.Context, q string) (*sql.Stmt, error) {
	if !cacheable(q) {
		return nil, nil
	}
	s.mu.Lock()
	st, ok := s.m[q]
	full := len(s.m) >= maxStmts
	s.mu.Unlock()
	if ok {
		return st, nil
	}
	if full {
		return nil, nil
	}
	// Prepare without the lock: it waits for the pool's connection, which a
	// transaction may hold while it asks this cache (cached) for a statement.
	st, err := s.db.PrepareContext(ctx, q)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if prev, ok := s.m[q]; ok {
		_ = st.Close()
		return prev, nil
	}
	s.m[q] = st
	return st, nil
}

// cached returns the pool statement for q if one was prepared, never
// preparing: inside a transaction the pool's only connection is taken.
func (s *stmts) cached(q string) *sql.Stmt {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[q]
}

func (s *stmts) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	st, err := s.get(ctx, q)
	if err != nil || st == nil {
		return s.db.ExecContext(ctx, q, args...)
	}
	return st.ExecContext(ctx, args...)
}

func (s *stmts) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	return s.db.PrepareContext(ctx, q)
}

func (s *stmts) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	st, err := s.get(ctx, q)
	if err != nil || st == nil {
		return s.db.QueryContext(ctx, q, args...)
	}
	return st.QueryContext(ctx, args...)
}

func (s *stmts) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	st, err := s.get(ctx, q)
	if err != nil || st == nil {
		return s.db.QueryRowContext(ctx, q, args...)
	}
	return st.QueryRowContext(ctx, args...)
}

// txDB runs a repository's queries on the unit's transaction, each prepared
// once per transaction: from the pool statement when the repository has one
// (Tx.StmtContext reuses it on the same connection), else on the transaction.
type txDB struct {
	u    *unit
	pool *stmts
}

func (t txDB) stmt(ctx context.Context, q string) (*sql.Stmt, error) {
	if !cacheable(q) {
		return nil, nil
	}
	t.u.mu.Lock()
	defer t.u.mu.Unlock()
	if st, ok := t.u.stmts[q]; ok {
		return st, nil
	}
	if len(t.u.stmts) >= maxStmts {
		return nil, nil
	}
	var st *sql.Stmt
	if ps := t.pool.cached(q); ps != nil {
		st = t.u.tx.StmtContext(ctx, ps)
	} else {
		var err error
		if st, err = t.u.tx.PrepareContext(ctx, q); err != nil {
			return nil, err
		}
	}
	t.u.stmts[q] = st
	return st, nil
}

func (t txDB) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	st, err := t.stmt(ctx, q)
	if err != nil || st == nil {
		return t.u.tx.ExecContext(ctx, q, args...)
	}
	return st.ExecContext(ctx, args...)
}

func (t txDB) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	return t.u.tx.PrepareContext(ctx, q)
}

func (t txDB) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	st, err := t.stmt(ctx, q)
	if err != nil || st == nil {
		return t.u.tx.QueryContext(ctx, q, args...)
	}
	return st.QueryContext(ctx, args...)
}

func (t txDB) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	st, err := t.stmt(ctx, q)
	if err != nil || st == nil {
		return t.u.tx.QueryRowContext(ctx, q, args...)
	}
	return st.QueryRowContext(ctx, args...)
}
