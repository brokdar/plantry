package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
)

// txKey carries the unit of work's *sql.Tx in a context.
type txKey struct{}

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
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// conn is embedded by every repository. It resolves the database handle for
// a call: the unit of work's transaction when ctx carries one, else the pool.
type conn struct {
	db *sql.DB
}

// dbtx returns the transaction in ctx, or the pool.
func (c conn) dbtx(ctx context.Context) sqlcgen.DBTX {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return c.db
}

// queriesFor returns sqlc queries bound to the transaction in ctx, or to the
// pool.
func (c conn) queriesFor(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(c.dbtx(ctx))
}

// inTx runs a multi-statement write atomically, joining the caller's unit of
// work when there is one.
func (c conn) inTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return inTx(ctx, c.db, fn)
}
