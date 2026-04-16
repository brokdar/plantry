package sqlite

import (
	"context"
	"database/sql"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// TxRunner implements planner.TxRunner using a SQLite *sql.DB.
type TxRunner struct {
	db *sql.DB
}

// NewTxRunner creates a TxRunner bound to db.
func NewTxRunner(db *sql.DB) *TxRunner {
	return &TxRunner{db: db}
}

// RunInTx wraps fn in a single transaction. The closure receives weeks + plate
// repositories bound to that transaction; both commit or both roll back.
func (t *TxRunner) RunInTx(ctx context.Context, fn func(planner.WeekRepository, plate.Repository) error) error {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	weeks := newWeekRepoTx(tx)
	plates := newPlateRepoTx(tx)

	if err := fn(weeks, plates); err != nil {
		return err
	}
	return tx.Commit()
}
